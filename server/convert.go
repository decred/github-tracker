// Copyright (c) 2017-2020 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package server

import (
	"time"

	"github.com/decred/github-tracker/api"
	"github.com/decred/github-tracker/database"
	"github.com/decred/github-tracker/jsonrpc/types"
)

func convertAPIPullRequestToDbPullRequest(apiPR *api.ApiPullRequest, repo api.ApiRepository, org string) (*database.PullRequest, error) {
	dbPR := &database.PullRequest{
		Repo:         repo.FullName,
		Organization: org,
		User:         apiPR.User.Login,
		URL:          apiPR.URL,
		Number:       apiPR.Number,
		State:        apiPR.State,
		Additions:    apiPR.Additions,
		Deletions:    apiPR.Deletions,
	}
	if apiPR.MergedAt != "" {
		mergedAt, err := time.Parse(time.RFC3339, apiPR.MergedAt)
		if err != nil {
			return nil, err
		}
		dbPR.MergedAt = mergedAt.Unix()
	}
	if apiPR.UpdatedAt != "" {
		updatedAt, err := time.Parse(time.RFC3339, apiPR.UpdatedAt)
		if err != nil {
			return nil, err
		}
		dbPR.UpdatedAt = updatedAt.Unix()
	}
	return dbPR, nil
}

func convertAPICommitsToDbCommits(apiCommits []api.ApiPullRequestCommit) []database.Commit {
	dbCommits := make([]database.Commit, 0, len(apiCommits))
	for _, commit := range apiCommits {
		dbCommit := convertAPICommitToDbCommit(commit)
		dbCommits = append(dbCommits, dbCommit)
	}
	return dbCommits
}

func convertAPICommitToDbCommit(apiCommit api.ApiPullRequestCommit) database.Commit {
	dbCommit := database.Commit{
		SHA:       apiCommit.SHA,
		URL:       apiCommit.URL,
		Message:   apiCommit.Commit.Message,
		Author:    apiCommit.Author.Login,
		Committer: apiCommit.Committer.Login,
		Additions: apiCommit.Stats.Additions,
		Deletions: apiCommit.Stats.Deletions,
	}
	return dbCommit
}

func convertAPIReviewsToDbReviews(apiReviews []api.ApiPullRequestReview) []database.PullRequestReview {
	dbReviews := make([]database.PullRequestReview, 0, len(apiReviews))
	for _, review := range apiReviews {
		dbReview := convertAPIReviewToDbReview(review)
		dbReviews = append(dbReviews, dbReview)
	}
	return dbReviews
}

func convertAPIReviewToDbReview(apiReview api.ApiPullRequestReview) database.PullRequestReview {
	dbReview := database.PullRequestReview{
		ID:          apiReview.ID,
		User:        apiReview.User.Login,
		State:       apiReview.State,
		SubmittedAt: parseTime(apiReview.SubmittedAt).Unix(),
		CommitID:    apiReview.CommitID,
	}
	return dbReview
}

func convertDBPullRequestsToPullRequests(dbPRs []*database.PullRequest) []types.PullRequestInformation {
	prInfo := make([]types.PullRequestInformation, 0, len(dbPRs))

	for _, dbPR := range dbPRs {
		pr := types.PullRequestInformation{
			Repoistory: dbPR.Repo,
			Additions:  dbPR.Additions,
			Deletions:  dbPR.Deletions,
			Date:       time.Unix(dbPR.MergedAt, 0).Format(time.RFC1123),
			Number:     dbPR.Number,
		}
		prInfo = append(prInfo, pr)
	}
	return prInfo
}
