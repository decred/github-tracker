// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2017-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"runtime/trace"
	"sync"
	"sync/atomic"
	"time"

	"github.com/decred/dcrd/dcrjson/v3"
	dcrdtypes "github.com/decred/dcrd/rpc/jsonrpc/types"
	"github.com/decred/dcrwallet/errors/v2"
	"github.com/decred/github-tracker/jsonrpc/types"
	"github.com/decred/github-tracker/server"
	"github.com/gorilla/websocket"
)

type websocketClient struct {
	conn          *websocket.Conn
	authenticated bool
	allRequests   chan []byte
	responses     chan []byte
	cancel        func()
	quit          chan struct{} // closed on disconnect
	wg            sync.WaitGroup
}

func newWebsocketClient(c *websocket.Conn, cancel func(), authenticated bool) *websocketClient {
	return &websocketClient{
		conn:          c,
		authenticated: authenticated,
		allRequests:   make(chan []byte),
		responses:     make(chan []byte),
		cancel:        cancel,
		quit:          make(chan struct{}),
	}
}

func (c *websocketClient) send(b []byte) error {
	select {
	case c.responses <- b:
		return nil
	case <-c.quit:
		return errors.New("websocket client disconnected")
	}
}

// Server holds the items the RPC server may need to access (auth,
// config, shutdown, etc.)
type Server struct {
	httpServer http.Server
	listeners  []net.Listener
	authsha    [sha256.Size]byte
	upgrader   websocket.Upgrader
	server     server.Server

	cfg Options

	wg      sync.WaitGroup
	quit    chan struct{}
	quitMtx sync.Mutex

	requestShutdownChan chan struct{}
}

type handler struct {
	fn     func(*Server, context.Context, interface{}) (interface{}, error)
	noHelp bool
}

// jsonAuthFail sends a message back to the client if the http auth is rejected.
func jsonAuthFail(w http.ResponseWriter) {
	w.Header().Add("WWW-Authenticate", `Basic realm="dcrwallet RPC"`)
	http.Error(w, "401 Unauthorized.", http.StatusUnauthorized)
}

// NewServer creates a new server for serving JSON-RPC client connections,
// both HTTP POST and websocket.
func NewServer(opts *Options, listeners []net.Listener, s server.Server) *Server {
	serveMux := http.NewServeMux()
	const rpcAuthTimeoutSeconds = 10
	server := &Server{
		httpServer: http.Server{
			Handler: serveMux,

			// Timeout connections which don't complete the initial
			// handshake within the allowed timeframe.
			ReadTimeout: time.Second * rpcAuthTimeoutSeconds,
		},
		cfg:       *opts,
		listeners: listeners,
		// A hash of the HTTP basic auth string is used for a constant
		// time comparison.
		authsha: sha256.Sum256(httpBasicAuth(opts.Username, opts.Password)),
		upgrader: websocket.Upgrader{
			// Allow all origins.
			CheckOrigin: func(r *http.Request) bool { return true },
		},
		quit:                make(chan struct{}),
		requestShutdownChan: make(chan struct{}, 1),
		server:              s,
	}

	serveMux.Handle("/", throttledFn(opts.MaxPOSTClients,
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Connection", "close")
			w.Header().Set("Content-Type", "application/json")
			r.Close = true

			if err := server.checkAuthHeader(r); err != nil {
				log.Warnf("Failed authentication attempt from client %s",
					r.RemoteAddr)
				jsonAuthFail(w)
				return
			}
			server.wg.Add(1)
			defer server.wg.Done()
			server.postClientRPC(w, r)
		}))

	for _, lis := range listeners {
		server.serve(lis)
	}

	return server
}

// httpBasicAuth returns the UTF-8 bytes of the HTTP Basic authentication
// string:
//
//   "Basic " + base64(username + ":" + password)
func httpBasicAuth(username, password string) []byte {
	const header = "Basic "
	base64 := base64.StdEncoding

	b64InputLen := len(username) + len(":") + len(password)
	b64Input := make([]byte, 0, b64InputLen)
	b64Input = append(b64Input, username...)
	b64Input = append(b64Input, ':')
	b64Input = append(b64Input, password...)

	output := make([]byte, len(header)+base64.EncodedLen(b64InputLen))
	copy(output, header)
	base64.Encode(output[len(header):], b64Input)
	return output
}

// serve serves HTTP POST and websocket RPC for the JSON-RPC RPC server.
// This function does not block on lis.Accept.
func (s *Server) serve(lis net.Listener) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		log.Infof("Listening on %s", lis.Addr())
		err := s.httpServer.Serve(lis)
		log.Tracef("Finished serving RPC: %v", err)
	}()
}

// Stop gracefully shuts down the rpc server by stopping and disconnecting all
// clients.  This blocks until shutdown completes.
func (s *Server) Stop() {
	s.quitMtx.Lock()
	select {
	case <-s.quit:
		s.quitMtx.Unlock()
		return
	default:
	}

	// Stop all the listeners.
	for _, listener := range s.listeners {
		err := listener.Close()
		if err != nil {
			log.Errorf("Cannot close listener `%s`: %v",
				listener.Addr(), err)
		}
	}

	// Signal the remaining goroutines to stop.
	close(s.quit)
	s.quitMtx.Unlock()

	// Wait for all remaining goroutines to exit.
	s.wg.Wait()
}

// handlerClosure creates a closure function for handling requests of the given
// method.  This may be a request that is handled directly by dcrwallet, or
// a chain server request that is handled by passing the request down to dcrd.
//
// NOTE: These handlers do not handle special cases, such as the authenticate
// method.  Each of these must be checked beforehand (the method is already
// known) and handled accordingly.
func (s *Server) handlerClosure(ctx context.Context, request *dcrjson.Request) lazyHandler {
	log.Infof("RPC method %v invoked by %v", request.Method, remoteAddr(ctx))
	return lazyApplyHandler(s, ctx, request)
}

// errNoAuth represents an error where authentication could not succeed
// due to a missing Authorization HTTP header.
var errNoAuth = errors.E("missing Authorization header")

// checkAuthHeader checks the HTTP Basic authentication supplied by a client
// in the HTTP request r.
//
// The authentication comparison is time constant.
func (s *Server) checkAuthHeader(r *http.Request) error {
	authhdr := r.Header["Authorization"]
	if len(authhdr) == 0 {
		return errNoAuth
	}

	authsha := sha256.Sum256([]byte(authhdr[0]))
	cmp := subtle.ConstantTimeCompare(authsha[:], s.authsha[:])
	if cmp != 1 {
		return errors.New("invalid Authorization header")
	}
	return nil
}

// throttledFn wraps an http.HandlerFunc with throttling of concurrent active
// clients by responding with an HTTP 429 when the threshold is crossed.
func throttledFn(threshold int64, f http.HandlerFunc) http.Handler {
	return throttled(threshold, f)
}

// throttled wraps an http.Handler with throttling of concurrent active
// clients by responding with an HTTP 429 when the threshold is crossed.
func throttled(threshold int64, h http.Handler) http.Handler {
	var active int64

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		current := atomic.AddInt64(&active, 1)
		defer atomic.AddInt64(&active, -1)

		if current-1 >= threshold {
			log.Warnf("Reached threshold of %d concurrent active clients", threshold)
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		h.ServeHTTP(w, r)
	})
}

// idPointer returns a pointer to the passed ID, or nil if the interface is nil.
// Interface pointers are usually a red flag of doing something incorrectly,
// but this is only implemented here to work around an oddity with dcrjson,
// which uses empty interface pointers for response IDs.
func idPointer(id interface{}) (p *interface{}) {
	if id != nil {
		p = &id
	}
	return
}

// invalidAuth checks whether a websocket request is a valid (parsable)
// authenticate request and checks the supplied username and passphrase
// against the server auth.
func (s *Server) invalidAuth(req *dcrjson.Request) bool {
	cmd, err := dcrjson.ParseParams(types.Method(req.Method), req.Params)
	if err != nil {
		return false
	}
	authCmd, ok := cmd.(*dcrdtypes.AuthenticateCmd)
	if !ok {
		return false
	}
	// Check credentials.
	login := authCmd.Username + ":" + authCmd.Passphrase
	auth := "Basic " + base64.StdEncoding.EncodeToString([]byte(login))
	authSha := sha256.Sum256([]byte(auth))
	return subtle.ConstantTimeCompare(authSha[:], s.authsha[:]) != 1
}

func (s *Server) websocketClientRead(ctx context.Context, wsc *websocketClient) {
	for {
		_, request, err := wsc.conn.ReadMessage()
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
				log.Warnf("Websocket receive failed from client %s: %v",
					remoteAddr(ctx), err)
			}
			close(wsc.allRequests)
			wsc.cancel()
			break
		}
		wsc.allRequests <- request
	}
}

// maxRequestSize specifies the maximum number of bytes in the request body
// that may be read from a client.  This is currently limited to 4MB.
const maxRequestSize = 1024 * 1024 * 4

// postClientRPC processes and replies to a JSON-RPC client request.
func (s *Server) postClientRPC(w http.ResponseWriter, r *http.Request) {
	ctx := withRemoteAddr(r.Context(), r.RemoteAddr)

	body := http.MaxBytesReader(w, r.Body, maxRequestSize)
	rpcRequest, err := ioutil.ReadAll(body)
	if err != nil {
		// TODO: what if the underlying reader errored?
		log.Warnf("Request from client %v exceeds maximum size", r.RemoteAddr)
		http.Error(w, "413 Request Too Large.",
			http.StatusRequestEntityTooLarge)
		return
	}

	// First check whether wallet has a handler for this request's method.
	// If unfound, the request is sent to the chain server for further
	// processing.  While checking the methods, disallow authenticate
	// requests, as they are invalid for HTTP POST clients.
	var req dcrjson.Request
	err = json.Unmarshal(rpcRequest, &req)
	if err != nil {
		resp, err := dcrjson.MarshalResponse(req.Jsonrpc, req.ID, nil, dcrjson.ErrRPCInvalidRequest)
		if err != nil {
			log.Errorf("Unable to marshal response to client %s: %v",
				r.RemoteAddr, err)
			http.Error(w, "500 Internal Server Error",
				http.StatusInternalServerError)
			return
		}
		_, err = w.Write(resp)
		if err != nil {
			log.Warnf("Cannot write invalid request request to "+
				"client %s: %v", r.RemoteAddr, err)
		}
		return
	}

	ctx, task := trace.NewTask(ctx, req.Method)
	defer task.End()

	// Create the response and error from the request.  Two special cases
	// are handled for the authenticate and stop request methods.
	var res interface{}
	var jsonErr *dcrjson.RPCError
	var stop bool
	switch req.Method {
	default:
		res, jsonErr = s.handlerClosure(ctx, &req)()
	}

	// Marshal and send.
	mresp, err := dcrjson.MarshalResponse(req.Jsonrpc, req.ID, res, jsonErr)
	if err != nil {
		log.Errorf("Unable to marshal response to client %s: %v",
			r.RemoteAddr, err)
		http.Error(w, "500 Internal Server Error", http.StatusInternalServerError)
		return
	}
	_, err = w.Write(mresp)
	if err != nil {
		log.Warnf("Failed to write response to client %s: %v",
			r.RemoteAddr, err)
	}

	if stop {
		s.requestProcessShutdown()
	}
}

func (s *Server) requestProcessShutdown() {
	s.requestShutdownChan <- struct{}{}
}

// RequestProcessShutdown returns a channel that is sent to when an authorized
// client requests remote shutdown.
func (s *Server) RequestProcessShutdown() <-chan struct{} {
	return s.requestShutdownChan
}
