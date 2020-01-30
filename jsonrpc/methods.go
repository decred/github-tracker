// Copyright (c) 2013-2016 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/github-tracker/jsonrpc/types"
)

// API version constants
const (
	jsonrpcSemverString = "1.0.0"
	jsonrpcSemverMajor  = 1
	jsonrpcSemverMinor  = 0
	jsonrpcSemverPatch  = 0
)

// the registered rpc handlers
var handlers = map[string]handler{
	// Reference implementation wallet methods (implemented)
	"update":          {fn: (*Server).update},
	"userinformation": {fn: (*Server).userInformation},
}

// lazyHandler is a closure over a requestHandler or passthrough request with
// the RPC server's wallet and chain server variables as part of the closure
// context.
type lazyHandler func() (interface{}, *dcrjson.RPCError)

// lazyApplyHandler looks up the best request handler func for the method,
// returning a closure that will execute it with the (required) wallet and
// (optional) consensus RPC server.  If no handlers are found and the
// chainClient is not nil, the returned handler performs RPC passthrough.
func lazyApplyHandler(s *Server, ctx context.Context, request *dcrjson.Request) lazyHandler {
	handlerData, ok := handlers[request.Method]
	if !ok {
		return func() (interface{}, *dcrjson.RPCError) {
			return nil, dcrjson.ErrRPCInvalidRequest
		}
	}

	return func() (interface{}, *dcrjson.RPCError) {
		params, err := dcrjson.ParseParams(types.Method(request.Method), request.Params)
		if err != nil {
			fmt.Println(err)
			return nil, dcrjson.ErrRPCInvalidRequest
		}

		defer func() {
			if err := ctx.Err(); err != nil {
				log.Warnf("Canceled RPC method %v invoked by %v: %v", request.Method, remoteAddr(ctx), err)
			}
		}()
		resp, err := handlerData.fn(s, ctx, params)
		if err != nil {
			return nil, convertError(err)
		}
		return resp, nil
	}
}

// makeResponse makes the JSON-RPC response struct for the result and error
// returned by a requestHandler.  The returned response is not ready for
// marshaling and sending off to a client, but must be
func makeResponse(id, result interface{}, err error) dcrjson.Response {
	idPtr := idPointer(id)
	if err != nil {
		return dcrjson.Response{
			ID:    idPtr,
			Error: convertError(err),
		}
	}
	resultBytes, err := json.Marshal(result)
	if err != nil {
		return dcrjson.Response{
			ID: idPtr,
			Error: &dcrjson.RPCError{
				Code:    dcrjson.ErrRPCInternal.Code,
				Message: "Unexpected error marshalling result",
			},
		}
	}
	return dcrjson.Response{
		ID:     idPtr,
		Result: json.RawMessage(resultBytes),
	}
}

// update removes an unconfirmed transaction and all dependent
// transactions from the wallet.
func (s *Server) update(ctx context.Context, icmd interface{}) (interface{}, error) {
	cmd := icmd.(*types.UpdateCmd)

	err := s.server.Update(ctx, cmd.Organization)
	if err != nil {
		return nil, err
	}
	return nil, err
}

// update removes an unconfirmed transaction and all dependent
// transactions from the wallet.
func (s *Server) userInformation(ctx context.Context, icmd interface{}) (interface{}, error) {
	cmd := icmd.(*types.UserInformationCmd)

	userInfoResult, err := s.server.UserInformation(ctx, cmd.Org, cmd.User, cmd.Year, cmd.Month)
	if err != nil {
		return nil, err
	}
	return userInfoResult, err
}
