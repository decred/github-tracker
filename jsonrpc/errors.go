// Copyright (c) 2013-2015 The btcsuite developers
// Copyright (c) 2016-2018 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

import (
	"fmt"

	"github.com/decred/dcrd/dcrjson/v3"
	"github.com/decred/dcrwallet/errors/v2"
)

func convertError(err error) *dcrjson.RPCError {
	if err, ok := err.(*dcrjson.RPCError); ok {
		return err
	}

	code := dcrjson.ErrRPCWallet
	var kind errors.Kind
	if errors.As(err, &kind) {
		switch kind {
		case errors.Bug:
			code = dcrjson.ErrRPCInternal.Code
		case errors.Encoding:
			code = dcrjson.ErrRPCInvalidParameter
		}
	}
	return &dcrjson.RPCError{
		Code:    code,
		Message: err.Error(),
	}
}

func rpcError(code dcrjson.RPCErrorCode, err error) *dcrjson.RPCError {
	return &dcrjson.RPCError{
		Code:    code,
		Message: err.Error(),
	}
}

func rpcErrorf(code dcrjson.RPCErrorCode, format string, args ...interface{}) *dcrjson.RPCError {
	return &dcrjson.RPCError{
		Code:    code,
		Message: fmt.Sprintf(format, args...),
	}
}
