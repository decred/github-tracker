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
	startDate := time.Date(int(year), time.Month(month), 0, 0, 0, 0, 0, time.UTC).Unix()
	endDate := time.Date(int(year), time.Month(month+1), 0, 0, 0, 0, 0, time.UTC).Unix()
	dbUserPRs, err := s.DB.PullRequestsByUserDates(user, startDate, endDate)
	if err != nil {
		return nil, err
	}
	dbReviews, err := s.DB.ReviewsByUserDates(user, startDate, endDate)
	if err != nil {
		return nil, err
	}
	userInfo := convertPRsandReviewsToUserInformation(dbUserPRs, dbReviews)
	userInfo.User = user
	userInfo.Organization = org
	return userInfo, nil
}
