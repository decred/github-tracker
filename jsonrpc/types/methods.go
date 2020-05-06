// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

// NOTE: This file is intended to house the RPC commands that are supported by
// a wallet server.

package types

import (
	"github.com/decred/dcrd/dcrjson/v3"
)

// Method describes the exact type used when registering methods with dcrjson.
type Method string

// UpdateCmd describes the command and parameters for performing the
// update method.
type UpdateCmd struct {
	Organization string `json:"orgnaization"`
}

// UserInformationCmd describes the command and parameters for performing the
// userinformation method.
type UserInformationCmd struct {
	User  string `json:"user"`
	Org   string `json:"org"`
	Year  int    `json:"year"`
	Month int    `json:"month"`
}

type registeredMethod struct {
	method string
	cmd    interface{}
}

func init() {
	flags := dcrjson.UsageFlag(0)
	dcrjson.MustRegister(Method("update"), (*UpdateCmd)(nil), flags)
	dcrjson.MustRegister(Method("userinformation"), (*UserInformationCmd)(nil), flags)
}
