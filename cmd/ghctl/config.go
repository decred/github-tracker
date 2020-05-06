// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/decred/dcrd/dcrjson/v3"
	dcrutil "github.com/decred/dcrd/dcrutil/v2"
	"github.com/decred/dcrwallet/version"

	ghtypes "github.com/decred/github-tracker/jsonrpc/types"

	flags "github.com/jessevdk/go-flags"
)

const (
	// unusableFlags are the command usage flags which this utility are not
	// able to use.  In particular it doesn't support websockets and
	// consequently notifications.
	unusableFlags = dcrjson.UFWebsocketOnly | dcrjson.UFNotification
)

var (
	ghctlHomeDir         = dcrutil.AppDataDir("ghctl", false)
	githubTrackerHomeDir = dcrutil.AppDataDir("github-tracker", false)
	defaultConfigFile    = filepath.Join(ghctlHomeDir, "ghctl.conf")
	defaultRPCServer     = "localhost"
	defaultRPCPort       = "8001"
	defaultRPCCertFile   = filepath.Join(githubTrackerHomeDir, "rpc.cert")
)

// listCommands categorizes and lists all of the usable commands along with
// their one-line usage.
func listCommands() {
	var categories = []struct {
		Header string
		Method interface{}
		Usages []string
	}{
		{
			Header: "GH Tracker Commands:",
			Method: ghtypes.Method(""),
		},
	}
	for i := range categories {
		method := categories[i].Method
		methods := dcrjson.RegisteredMethods(method)
		for _, methodStr := range methods {
			method = ghtypes.Method(methodStr)

			flags, err := dcrjson.MethodUsageFlags(method)
			if err != nil {
				// This should never happen since the method was just
				// returned from the package, but be safe.
				continue
			}

			// Skip the commands that aren't usable from this utility.
			if flags&unusableFlags != 0 {
				continue
			}

			usage, err := dcrjson.MethodUsageText(method)
			if err != nil {
				// This should never happen since the method was just
				// returned from the package, but be safe.
				continue
			}

			categories[i].Usages = append(categories[i].Usages, usage)
		}
	}

	// Display the command according to their categories.
	for i := range categories {
		fmt.Println(categories[i].Header)
		for _, usage := range categories[i].Usages {
			fmt.Println(usage)
		}
		fmt.Println()
	}
}

// config defines the configuration options for dcrctl.
//
// See loadConfig for details on the configuration load process.
type config struct {
	ShowVersion     bool   `short:"V" long:"version" description:"Display version information and exit"`
	ListCommands    bool   `short:"l" long:"listcommands" description:"List all of the supported commands and exit"`
	ConfigFile      string `short:"C" long:"configfile" description:"Path to configuration file"`
	RPCUser         string `short:"u" long:"rpcuser" description:"RPC username"`
	RPCPassword     string `short:"P" long:"rpcpass" default-mask:"-" description:"RPC password"`
	RPCServer       string `short:"s" long:"rpcserver" description:"RPC server to connect to"`
	WalletRPCServer string `short:"w" long:"walletrpcserver" description:"Wallet RPC server to connect to"`
	RPCCert         string `short:"c" long:"rpccert" description:"RPC server certificate chain for validation"`
	PrintJSON       bool   `short:"j" long:"json" description:"Print json messages sent and received"`
	NoTLS           bool   `long:"notls" description:"Disable TLS"`
	Proxy           string `long:"proxy" description:"Connect via SOCKS5 proxy (eg. 127.0.0.1:9050)"`
	ProxyUser       string `long:"proxyuser" description:"Username for proxy server"`
	ProxyPass       string `long:"proxypass" default-mask:"-" description:"Password for proxy server"`
	TLSSkipVerify   bool   `long:"skipverify" description:"Do not verify tls certificates (not recommended!)"`
}

// cleanAndExpandPath expands environment variables and leading ~ in the
// passed path, cleans the result, and returns it.
func cleanAndExpandPath(path string) string {
	// Nothing to do when no path is given.
	if path == "" {
		return path
	}

	// NOTE: The os.ExpandEnv doesn't work with Windows cmd.exe-style
	// %VARIABLE%, but the variables can still be expanded via POSIX-style
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

// fileExists reports whether the named file or directory exists.
func fileExists(name string) bool {
	if _, err := os.Stat(name); err != nil {
		if os.IsNotExist(err) {
			return false
		}
	}
	return true
}

// loadConfig initializes and parses the config using a config file and command
// line options.
//
// The configuration proceeds as follows:
// 	1) Start with a default config with sane settings
// 	2) Pre-parse the command line to check for an alternative config file
// 	3) Load configuration file overwriting defaults with any specified options
// 	4) Parse CLI options and overwrite/add any specified options
//
// The above results in functioning properly without any config settings
// while still allowing the user to override settings with config files and
// command line options.  Command line options always take precedence.
func loadConfig() (*config, []string, error) {
	// Default config.
	cfg := config{
		ConfigFile: defaultConfigFile,
		RPCServer:  defaultRPCServer,
		RPCCert:    defaultRPCCertFile,
	}

	// Pre-parse the command line options to see if an alternative config
	// file, the version flag, or the list commands flag was specified.  Any
	// errors aside from the help message error can be ignored here since
	// they will be caught by the final parse below.
	preCfg := cfg
	preParser := flags.NewParser(&preCfg, flags.HelpFlag)
	_, err := preParser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); ok && e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, err)
			fmt.Fprintln(os.Stderr, "")
			fmt.Fprintln(os.Stderr, "The special parameter `-` "+
				"indicates that a parameter should be read "+
				"from the\nnext unread line from standard input.")
			os.Exit(1)
		} else if ok && e.Type == flags.ErrHelp {
			fmt.Fprintln(os.Stdout, err)
			fmt.Fprintln(os.Stdout, "")
			fmt.Fprintln(os.Stdout, "The special parameter `-` "+
				"indicates that a parameter should be read "+
				"from the\nnext unread line from standard input.")
			os.Exit(0)
		}
	}

	// Show the version and exit if the version flag was specified.
	appName := filepath.Base(os.Args[0])
	appName = strings.TrimSuffix(appName, filepath.Ext(appName))
	usageMessage := fmt.Sprintf("Use %s -h to show options", appName)
	if preCfg.ShowVersion {
		fmt.Printf("%s version %s (Go version %s %s/%s)\n", appName,
			version.String(), runtime.Version(), runtime.GOOS, runtime.GOARCH)
		os.Exit(0)
	}

	// Show the available commands and exit if the associated flag was
	// specified.
	if preCfg.ListCommands {
		listCommands()
		os.Exit(0)
	}

	if !fileExists(preCfg.ConfigFile) {
		err := createDefaultConfigFile(preCfg.ConfigFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating a default config file: %v\n", err)
		}
	}

	// Load additional config from file.
	parser := flags.NewParser(&cfg, flags.Default)
	err = flags.NewIniParser(parser).ParseFile(preCfg.ConfigFile)
	if err != nil {
		if _, ok := err.(*os.PathError); !ok {
			fmt.Fprintf(os.Stderr, "Error parsing config file: %v\n",
				err)
			fmt.Fprintln(os.Stderr, usageMessage)
			return nil, nil, err
		}
	}

	// Parse command line options again to ensure they take precedence.
	remainingArgs, err := parser.Parse()
	if err != nil {
		if e, ok := err.(*flags.Error); !ok || e.Type != flags.ErrHelp {
			fmt.Fprintln(os.Stderr, usageMessage)
		}
		return nil, nil, err
	}

	// Handle environment variable expansion in the RPC certificate path.
	cfg.RPCCert = cleanAndExpandPath(cfg.RPCCert)
	cfg.RPCServer = net.JoinHostPort(cfg.RPCServer, defaultRPCPort)
	return &cfg, remainingArgs, nil
}

// createDefaultConfig creates a basic config file at the given destination path.
// For this it tries to read the dcrd config file at its default path, and extract
// the RPC user and password from it.
func createDefaultConfigFile(destinationPath string) error {
	// Nothing to do when there is no existing dcrd conf file at the default
	// path to extract the details from.
	ghConfigPath := filepath.Join(githubTrackerHomeDir, "dcrd.conf")
	if !fileExists(ghConfigPath) {
		return nil
	}

	// Read dcrd.conf from its default path
	ghConfigFile, err := os.Open(ghConfigPath)
	if err != nil {
		return err
	}
	defer ghConfigFile.Close()
	content, err := ioutil.ReadAll(ghConfigFile)
	if err != nil {
		return err
	}

	// Extract the rpcuser
	rpcUserRegexp, err := regexp.Compile(`(?m)^\s*rpcuser=([^\s]+)`)
	if err != nil {
		return err
	}
	userSubmatches := rpcUserRegexp.FindSubmatch(content)
	if userSubmatches == nil {
		// No user found, nothing to do
		return nil
	}

	// Extract the rpcpass
	rpcPassRegexp, err := regexp.Compile(`(?m)^\s*rpcpass=([^\s]+)`)
	if err != nil {
		return err
	}
	passSubmatches := rpcPassRegexp.FindSubmatch(content)
	if passSubmatches == nil {
		// No password found, nothing to do
		return nil
	}

	// Create the destination directory if it does not exists
	err = os.MkdirAll(filepath.Dir(destinationPath), 0700)
	if err != nil {
		return err
	}

	// Create the destination file and write the rpcuser and rpcpass to it
	dest, err := os.OpenFile(destinationPath,
		os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer dest.Close()

	dest.WriteString(fmt.Sprintf("rpcuser=%s\nrpcpass=%s",
		string(userSubmatches[1]), string(passSubmatches[1])))

	return nil
}
