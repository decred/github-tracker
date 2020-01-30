// Copyright (c) 2015-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package jsonrpc

// UpdateCmd describes the command and parameters for performing the
// update command
type UpdateCmd struct {
	Organization string `json:"organization"`
}

// UserInformationCmd describes the command and parameters for perfoming the
// userinformation command
type UserInformationCmd struct {
	User  string `json:"user"`
	Org   string `json:"org"`
	Year  int    `json:"year"`
	Month int    `json:"month"`
}
