// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package server

import (
	"time"

	"github.com/decred/github-tracker/api"
	"github.com/decred/github-tracker/database"
)

type Server struct {
	tc *api.Client
	// Following entries are use only during cmswww mode
	DB database.Database
}

type S struct {
	Additions int
	Deletions int
	Total     int
}

func NewServer(token string) (*Server, error) {
	tc := api.NewClient(token)

	return &Server{
		tc: tc,
	}, nil
}

func parseTime(tstamp string) time.Time {
	t, err := time.Parse(time.RFC3339, tstamp)
	if err != nil {
		return time.Time{}
	}
	return t
}
