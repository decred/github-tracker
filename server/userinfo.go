// Copyright (c) 2018-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package server

import (
	"context"
	"fmt"
	"time"

	"github.com/decred/github-tracker/jsonrpc/types"
)

func yearMonth(t time.Time) string {
	return fmt.Sprintf("%d%02d", t.Year(), t.Month())
}

func (s *Server) UserInformation(ctx context.Context, org string, user string, year, month int) (*types.UserInformationResult, error) {
	userInfo := &types.UserInformationResult{}
	userInfo.RepoDetails = make([]types.RepositoryInformation, 0, 1024)
	userInfo.Reviews = make([]types.ReviewInformation, 0, 1024)

	dbUserPRs, err := s.DB.PullRequestsByUserDates(user, int64(year), int64(month))
	if err != nil {
		return nil, err
	}
	userInfoPrs := convertDBPullRequestsToPullRequests(dbUserPRs)
	userInfo.PRs = userInfoPrs
	return userInfo, nil
}
