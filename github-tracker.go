package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/ioutil"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/decred/github-tracker/database"
	db "github.com/decred/github-tracker/database/cockroachdb"
	"github.com/decred/github-tracker/jsonrpc"
	"github.com/decred/github-tracker/server"
)

// githubtracker application context.
type githubtracker struct {
	cfg    *config
	server *server.Server
}

func main() {
	// Create a context that is cancelled when a shutdown request is received
	// through an interrupt signal or an RPC request.
	ctx := withShutdownCancel(context.Background())
	go shutdownListener()

	// Run the wallet until permanent failure or shutdown is requested.
	if err := run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		os.Exit(1)
	}
}

// done returns whether the context's Done channel was closed due to
// cancellation or exceeded deadline.
func done(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

func run(ctx context.Context) error {
	cfg, err := loadConfig()
	if err != nil {
		log.Errorf("loadConfig failed: %v", err)
		return ctx.Err()
	}
	defer func() {
		if logRotator != nil {
			logRotator.Close()
		}
	}()

	s, err := server.NewServer(cfg.APIToken)
	if err != nil {
		log.Errorf("NewServer failed: %v\n", err)
		return ctx.Err()
	}

	s.DB, err = db.New(cfg.DBHost, cfg.DBRootCert, cfg.DBCert, cfg.DBKey)
	if err == database.ErrNoVersionRecord || err == database.ErrWrongVersion {
		log.Errorf("New DB failed no version, wrong version: %v\n", err)
		return err
	} else if err != nil {
		log.Errorf("New DB failed: %v\n", err)
		return err
	}
	err = s.DB.Setup()
	if err != nil {
		log.Errorf("DB Setup failed: %v\n", err)
		return fmt.Errorf("cmsdb setup: %v", err)
	}
	defer s.DB.Close()

	jsonRPCServer, err := startJSONRPCServer(cfg, s)
	if err != nil {
		log.Errorf("unable to create RPC servers: %v", err)
		return ctx.Err()
	}
	if jsonRPCServer != nil {
		go func() {
			for range jsonRPCServer.RequestProcessShutdown() {
				requestShutdown()
			}
		}()
		defer func() {
			log.Info("Stopping JSON-RPC server...")
			jsonRPCServer.Stop()
			log.Info("JSON-RPC server shutdown")
		}()
	}
	// Wait until shutdown is signaled before returning and running deferred
	// shutdown tasks.
	<-ctx.Done()
	return ctx.Err()
}

func startJSONRPCServer(cfg *config, s *server.Server) (*jsonrpc.Server, error) {
	var (
		jsonrpcServer *jsonrpc.Server
		jsonrpcListen = net.Listen
		keyPair       tls.Certificate
		err           error
	)

	keyPair, err = openRPCKeyPair(cfg)
	if err != nil {
		return nil, err
	}

	// Change the standard net.Listen function to the tls one.
	tlsConfig := &tls.Config{
		Certificates: []tls.Certificate{keyPair},
		MinVersion:   tls.VersionTLS12,
		NextProtos:   []string{"h2"}, // HTTP/2 over TLS
	}
	jsonrpcListen = func(net string, laddr string) (net.Listener, error) {
		return tls.Listen(net, laddr, tlsConfig)
	}

	if cfg.RPCUsername == "" || cfg.RPCPassword == "" {
		log.Info("JSON-RPC server disabled (requires username and password)")
	} else if len(cfg.LegacyRPCListeners) != 0 {
		listeners := makeListeners(cfg.LegacyRPCListeners, jsonrpcListen)
		if len(listeners) == 0 {
			err := errors.New("failed to create listeners for JSON-RPC server")
			return nil, err
		}
		opts := jsonrpc.Options{
			Username:       cfg.RPCUsername,
			Password:       cfg.RPCPassword,
			MaxPOSTClients: cfg.LegacyRPCMaxClients,
		}
		jsonrpcServer = jsonrpc.NewServer(&opts, listeners, *s)
	}

	// Error when neither the GRPC nor JSON-RPC servers can be started.
	if jsonrpcServer == nil {
		return nil, errors.New("no suitable RPC services can be started")
	}

	return jsonrpcServer, nil
}

// openRPCKeyPair creates or loads the RPC TLS keypair specified by the
// application config.
func openRPCKeyPair(cfg *config) (tls.Certificate, error) {
	// Check for existence of the TLS key file.  If one time TLS keys are
	// enabled but a key already exists, this function should error since
	// it's possible that a persistent certificate was copied to a remote
	// machine.  Otherwise, generate a new keypair when the key is missing.
	// When generating new persistent keys, overwriting an existing cert is
	// acceptable if the previous execution used a one time TLS key.
	// Otherwise, both the cert and key should be read from disk.  If the
	// cert is missing, the read error will occur in LoadX509KeyPair.
	_, e := os.Stat(cfg.RPCKey.Value)
	keyExists := !os.IsNotExist(e)
	switch {
	case !keyExists:
		return generateRPCKeyPair(true, cfg)
	default:
		return tls.LoadX509KeyPair(cfg.RPCCert.Value, cfg.RPCKey.Value)
	}
}

// generateRPCKeyPair generates a new RPC TLS keypair and writes the cert and
// possibly also the key in PEM format to the paths specified by the config.  If
// successful, the new keypair is returned.
func generateRPCKeyPair(writeKey bool, cfg *config) (tls.Certificate, error) {
	log.Infof("Generating TLS certificates...")

	// Create directories for cert and key files if they do not yet exist.
	certDir, _ := filepath.Split(cfg.RPCCert.Value)
	keyDir, _ := filepath.Split(cfg.RPCKey.Value)
	err := os.MkdirAll(certDir, 0700)
	if err != nil {
		return tls.Certificate{}, err
	}
	err = os.MkdirAll(keyDir, 0700)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Generate cert pair.
	org := "github-tracker autogenerated cert"
	validUntil := time.Now().Add(time.Hour * 24 * 365 * 10)
	cert, key, err := cfg.TLSCurve.CertGen(org, validUntil, nil)
	if err != nil {
		return tls.Certificate{}, err
	}
	keyPair, err := tls.X509KeyPair(cert, key)
	if err != nil {
		return tls.Certificate{}, err
	}

	// Write cert and (potentially) the key files.
	err = ioutil.WriteFile(cfg.RPCCert.Value, cert, 0600)
	if err != nil {
		return tls.Certificate{}, err
	}
	if writeKey {
		err = ioutil.WriteFile(cfg.RPCKey.Value, key, 0600)
		if err != nil {
			rmErr := os.Remove(cfg.RPCCert.Value)
			if rmErr != nil {
				log.Warnf("Cannot remove written certificates: %v",
					rmErr)
			}
			return tls.Certificate{}, err
		}
	}

	log.Info("Done generating TLS certificates")
	return keyPair, nil
}

type listenFunc func(net string, laddr string) (net.Listener, error)

// makeListeners splits the normalized listen addresses into IPv4 and IPv6
// addresses and creates new net.Listeners for each with the passed listen func.
// Invalid addresses are logged and skipped.
func makeListeners(normalizedListenAddrs []string, listen listenFunc) []net.Listener {
	ipv4Addrs := make([]string, 0, len(normalizedListenAddrs)*2)
	ipv6Addrs := make([]string, 0, len(normalizedListenAddrs)*2)
	for _, addr := range normalizedListenAddrs {
		host, _, err := net.SplitHostPort(addr)
		if err != nil {
			// Shouldn't happen due to already being normalized.
			log.Errorf("`%s` is not a normalized "+
				"listener address", addr)
			continue
		}

		// Empty host or host of * on plan9 is both IPv4 and IPv6.
		if host == "" || (host == "*" && runtime.GOOS == "plan9") {
			ipv4Addrs = append(ipv4Addrs, addr)
			ipv6Addrs = append(ipv6Addrs, addr)
			continue
		}

		// Remove the IPv6 zone from the host, if present.  The zone
		// prevents ParseIP from correctly parsing the IP address.
		// ResolveIPAddr is intentionally not used here due to the
		// possibility of leaking a DNS query over Tor if the host is a
		// hostname and not an IP address.
		zoneIndex := strings.Index(host, "%")
		if zoneIndex != -1 {
			host = host[:zoneIndex]
		}

		ip := net.ParseIP(host)
		switch {
		case ip == nil:
			log.Warnf("`%s` is not a valid IP address", host)
		case ip.To4() == nil:
			ipv6Addrs = append(ipv6Addrs, addr)
		default:
			ipv4Addrs = append(ipv4Addrs, addr)
		}
	}
	listeners := make([]net.Listener, 0, len(ipv6Addrs)+len(ipv4Addrs))
	for _, addr := range ipv4Addrs {
		listener, err := listen("tcp4", addr)
		if err != nil {
			log.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}
	for _, addr := range ipv6Addrs {
		listener, err := listen("tcp6", addr)
		if err != nil {
			log.Warnf("Can't listen on %s: %v", addr, err)
			continue
		}
		listeners = append(listeners, listener)
	}
	return listeners
}
