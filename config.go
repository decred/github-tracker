package main

import (
	"crypto/elliptic"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"io/ioutil"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/decred/dcrd/certgen"
	"github.com/decred/dcrd/dcrutil"
	"github.com/decred/slog"
	flags "github.com/jessevdk/go-flags"
	"github.com/pkg/errors"
)

const (
	defaultAPIToken       = ""
	defaultConfigFilename = "github-tracker.conf"
	defaultLogFilename    = "github-tracker.log"
	defaultRPCPort        = "8001"
	defaultLogDirname     = "logs"
)

var (
	defaultAppDataDir  = dcrutil.AppDataDir("github-tracker", false)
	defaultConfigFile  = filepath.Join(defaultAppDataDir, defaultConfigFilename)
	defaultRPCKeyFile  = filepath.Join(defaultAppDataDir, "rpc.key")
	defaultRPCCertFile = filepath.Join(defaultAppDataDir, "rpc.cert")
	defaultLogDir      = filepath.Join(defaultAppDataDir, defaultLogDirname)
	defaultDataDir     = filepath.Join(defaultAppDataDir, "data")
)

type config struct {
	ConfigFile          string          `short:"C" long:"configfile" description:"Path to configuration file"`
	DataDir             string          `short:"b" long:"datadir" description:"Directory to store data"`
	APIToken            string          `long:"apitoken" description:"github api token"`
	Update              bool            `long:"update" description:"fetch latest github data"`
	RPCCert             *ExplicitString `long:"rpccert" description:"RPC server TLS certificate"`
	RPCKey              *ExplicitString `long:"rpckey" description:"RPC server TLS key"`
	TLSCurve            *CurveFlag      `long:"tlscurve" description:"Curve to use when generating TLS keypairs"`
	LegacyRPCListeners  []string        `long:"rpclisten" description:"Listen for JSON-RPC connections on this interface"`
	LegacyRPCMaxClients int64           `long:"rpcmaxclients" description:"Max JSON-RPC HTTP POST clients"`
	RPCUsername         string          `long:"rpcuser" description:"JSON-RPC username"`
	RPCPassword         string          `long:"rpcpass" default-mask:"-" description:"JSON-RPC password"`
	LogDir              *ExplicitString `long:"logdir" description:"Directory to log output."`
	DebugLevel          string          `short:"d" long:"debuglevel" description:"Logging level {trace, debug, info, warn, error, critical}"`
	DBHost              string          `long:"dbhost" description:"Database ip:port"`
	DBRootCert          string          `long:"dbrootcert" description:"File containing the CA certificate for the database"`
	DBCert              string          `long:"dbcert" description:"File containing the politeiawww client certificate for the database"`
	DBKey               string          `long:"dbkey" description:"File containing the politeiawww client certificate key for the database"`
}

// validLogLevel returns whether or not logLevel is a valid debug log level.
func validLogLevel(logLevel string) bool {
	_, ok := slog.LevelFromString(logLevel)
	return ok
}

// supportedSubsystems returns a sorted slice of the supported subsystems for
// logging purposes.
func supportedSubsystems() []string {
	// Convert the subsystemLoggers map keys to a slice.
	subsystems := make([]string, 0, len(subsystemLoggers))
	for subsysID := range subsystemLoggers {
		subsystems = append(subsystems, subsysID)
	}

	// Sort the subsytems for stable display.
	sort.Strings(subsystems)
	return subsystems
}

func loadConfig() (*config, error) {
	cfg := config{
		ConfigFile:          defaultConfigFile,
		DataDir:             defaultAppDataDir,
		APIToken:            defaultAPIToken,
		RPCKey:              NewExplicitString(defaultRPCKeyFile),
		RPCCert:             NewExplicitString(defaultRPCCertFile),
		LogDir:              NewExplicitString(defaultLogDir),
		TLSCurve:            NewCurveFlag(PreferredCurve),
		LegacyRPCMaxClients: 5,
	}

	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show usage", appName)

	parser := flags.NewParser(&cfg, flags.HelpFlag)
	err := flags.NewIniParser(parser).ParseFile(cfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config "+
				"file: %v\n", err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, err
		}
	}

	_, err = parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		} else if ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			os.Exit(0)
		}
	}

	// Append the network type to the log directory so it is "namespaced"
	// per network.
	cfg.LogDir.Value = cleanAndExpandPath(cfg.LogDir.Value)

	// Special show command to list supported subsystems and exit.
	if cfg.DebugLevel == "show" {
		fmt.Println("Supported subsystems", supportedSubsystems())
		os.Exit(0)
	}

	// Initialize log rotation.  After log rotation has been initialized, the
	// logger variables may be used.
	initLogRotator(filepath.Join(cfg.LogDir.Value, defaultLogFilename))

	cfg.RPCCert.Value = cleanAndExpandPath(cfg.RPCCert.Value)
	cfg.RPCKey.Value = cleanAndExpandPath(cfg.RPCKey.Value)

	// Default to localhost listen addresses if no listeners were manually
	// specified.  When the RPC server is configured to be disabled, remove all
	// listeners so it is not started.
	localhostAddrs, err := net.LookupHost("localhost")
	if err != nil {
		return nil, err
	}

	if len(cfg.LegacyRPCListeners) == 0 {
		cfg.LegacyRPCListeners = make([]string, 0, len(localhostAddrs))
		for _, addr := range localhostAddrs {
			cfg.LegacyRPCListeners = append(cfg.LegacyRPCListeners,
				net.JoinHostPort(addr, defaultRPCPort))
		}
	}
	/*
		if len(cfg.Orgs) == 0 {
			fmt.Fprintln(os.Stderr, "please specify the organization")
			os.Exit(1)
		}
		if cfg.PRNum != 0 && cfg.Repo == "" {
			fmt.Fprintln(os.Stderr, "prnum requires specifying the repository")
			os.Exit(1)
		}
	*/
	// Validate cache options.

	switch {
	case cfg.DBHost == "":
		return nil, fmt.Errorf("dbhost param is required")
	case cfg.DBRootCert == "":
		return nil, fmt.Errorf("dbrootcert param is required")
	case cfg.DBCert == "":
		return nil, fmt.Errorf("dbcert param is required")
	case cfg.DBKey == "":
		return nil, fmt.Errorf("dbkey param is required")
	}

	cfg.DBRootCert = cleanAndExpandPath(cfg.DBRootCert)
	cfg.DBCert = cleanAndExpandPath(cfg.DBCert)
	cfg.DBKey = cleanAndExpandPath(cfg.DBKey)

	// Validate db host.
	_, err = url.Parse(cfg.DBHost)
	if err != nil {
		return nil, fmt.Errorf("parse dbhost: %v", err)
	}

	// Validate db root cert.
	b, err := ioutil.ReadFile(cfg.DBRootCert)
	if err != nil {
		return nil, fmt.Errorf("read dbrootcert: %v", err)
	}
	block, _ := pem.Decode(b)
	_, err = x509.ParseCertificate(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("parse dbrootcert: %v", err)
	}

	// Validate db key pair.
	_, err = tls.LoadX509KeyPair(cfg.DBCert, cfg.DBKey)
	if err != nil {
		return nil, fmt.Errorf("load key pair dbcert "+
			"and dbkey: %v", err)
	}

	cfg.DataDir = cleanAndExpandPath(cfg.DataDir)
	return &cfg, nil
}

// cleanAndExpandPath expands environement variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Do not try to clean the empty string
	if path == "" {
		return ""
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but they variables can still be expanded via POSIX-style
	// $VARIABLE.
	path = os.ExpandEnv(path)

	if !strings.HasPrefix(path, "~") {
		return filepath.Clean(path)
	}

	// Expand initial ~ to the current user's home directory, or ~otheruser
	// to otheruser's home directory.  On Windows, both forward and backward
	// slashes can be used.
	path = path[1:]

	var pathSeparators string
	if runtime.GOOS == "windows" {
		pathSeparators = string(os.PathSeparator) + "/"
	} else {
		pathSeparators = string(os.PathSeparator)
	}

	userName := ""
	if i := strings.IndexAny(path, pathSeparators); i != -1 {
		userName = path[:i]
		path = path[i:]
	}

	homeDir := ""
	var u *user.User
	var err error
	if userName == "" {
		u, err = user.Current()
	} else {
		u, err = user.Lookup(userName)
	}
	if err == nil {
		homeDir = u.HomeDir
	}
	// Fallback to CWD if user lookup fails or user has no home directory.
	if homeDir == "" {
		homeDir = "."
	}

	return filepath.Join(homeDir, path)
}

// ExplicitString is a string value implementing the flags.Marshaler and
// flags.Unmarshaler interfaces so it may be used as a config struct field.  It
// records whether the value was explicitly set by the flags package.  This is
// useful when behavior must be modified depending on whether a flag was set by
// the user or left as a default.  Without recording this, it would be
// impossible to determine whether flag with a default value was unmodified or
// explicitly set to the default.
type ExplicitString struct {
	Value         string
	explicitlySet bool
}

// NewExplicitString creates a string flag with the provided default value.
func NewExplicitString(defaultValue string) *ExplicitString {
	return &ExplicitString{Value: defaultValue, explicitlySet: false}
}

// ExplicitlySet returns whether the flag was explicitly set through the
// flags.Unmarshaler interface.
func (e *ExplicitString) ExplicitlySet() bool { return e.explicitlySet }

// MarshalFlag implements the flags.Marshaler interface.
func (e *ExplicitString) MarshalFlag() (string, error) { return e.Value, nil }

// UnmarshalFlag implements the flags.Unmarshaler interface.
func (e *ExplicitString) UnmarshalFlag(value string) error {
	e.Value = value
	e.explicitlySet = true
	return nil
}

// String implements the fmt.Stringer interface.
func (e *ExplicitString) String() string { return e.Value }

// CurveID specifies a recognized curve through a constant value.
type CurveID int

// Recognized curve IDs.
const (
	CurveP224 CurveID = iota
	CurveP256
	CurveP384
	CurveP521
	afterECDSA
	Ed25519 CurveID = afterECDSA + iota
)

// CurveFlag describes a curve and implements the flags.Marshaler and
// Unmarshaler interfaces so it can be used as a config struct field.
type CurveFlag struct {
	curveID CurveID
}

// NewCurveFlag creates a CurveFlag with a default curve.
func NewCurveFlag(defaultValue CurveID) *CurveFlag {
	return &CurveFlag{defaultValue}
}

// ECDSACurve returns the elliptic curve described by f, or (nil, false) if the
// curve is not one of the elliptic curves suitable for ECDSA.
func (f *CurveFlag) ECDSACurve() (elliptic.Curve, bool) {
	switch f.curveID {
	case CurveP224:
		return elliptic.P224(), true
	case CurveP256:
		return elliptic.P256(), true
	case CurveP384:
		return elliptic.P384(), true
	case CurveP521:
		return elliptic.P521(), true
	default:
		return nil, false
	}
}

// PreferredCurve is the curve that should be used as the application default.
const PreferredCurve = Ed25519

// MarshalFlag satisfies the flags.Marshaler interface.
func (f *CurveFlag) MarshalFlag() (name string, err error) {
	switch f.curveID {
	case CurveP224:
		name = "P-224"
	case CurveP256:
		name = "P-256"
	case CurveP384:
		name = "P-384"
	case CurveP521:
		name = "P-521"
	case Ed25519:
		name = "Ed25519"
	default:
		err = errors.Errorf("unknown curve ID %v", int(f.curveID))
	}
	return
}

// UnmarshalFlag satisfies the flags.Unmarshaler interface.
func (f *CurveFlag) UnmarshalFlag(value string) error {
	switch value {
	case "P-224":
		f.curveID = CurveP224
	case "P-256":
		f.curveID = CurveP256
	case "P-384":
		f.curveID = CurveP384
	case "P-521":
		f.curveID = CurveP521
	case "Ed25519":
		f.curveID = Ed25519
	default:
		return errors.Errorf("unrecognized curve %v", value)
	}
	return nil
}

func (f *CurveFlag) CertGen(org string, validUntil time.Time, extraHosts []string) (cert, key []byte, err error) {
	if ec, ok := f.ECDSACurve(); ok {
		return certgen.NewTLSCertPair(ec, org, validUntil, extraHosts)
	}
	if f.curveID == Ed25519 {
		return certgen.NewEd25519TLSCertPair(org, validUntil, extraHosts)
	}
	return nil, nil, errors.New("unknown curve ID")
}
