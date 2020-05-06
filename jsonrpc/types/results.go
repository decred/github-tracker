// Copyright (c) 2014 The btcsuite developers
// Copyright (c) 2015-2019 The Decred developers
// Use of this source code is governed by an ISC
// license that can be found in the LICENSE file.

package types

// UpdateResult models the data from the update command.
type UpdateResult struct {
}

// UserInformationResult models the data from the userinformation command.
type UserInformationResult struct {
	User         string                   `json:"user"`
	Organization string                   `json:"organization"`
	PRs          []PullRequestInformation `json:"prs"`
	RepoDetails  []RepositoryInformation  `json:"repodetails"`
	Reviews      []ReviewInformation      `json:"reviews"`
}

type RepositoryInformation struct {
	PRs             []string `json:"prs"`
	Repository      string   `json:"repo"`
	CommitAdditions int64    `json:"commitadditions"`
	CommitDeletions int64    `json:"commitdeletions"`
	MergeAdditions  int64    `json:"mergeadditions"`
	MergeDeletions  int64    `json:"mergedeletions"`
	ReviewAdditions int64    `json:"reviewadditions"`
	ReviewDeletions int64    `json:"reviewdeletions"`
}

type PullRequestInformation struct {
	Repository string `json:"repo"`
	URL        string `json:"url"`
	Number     int    `json:"number"`
	Additions  int64  `json:"additions"`
	Deletions  int64  `json:"deletions"`
	Date       string `json:"date"`
	State      string `json:"state"`
}

type ReviewInformation struct {
	Repository string `json:"repo"`
	URL        string `json:"url"`
	Number     int    `json:"number"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Date       string `json:"date"`
	State      string `json:"state"`
}
