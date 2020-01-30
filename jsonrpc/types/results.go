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
	Repository      string `json:"repo"`
	CommitAdditions int    `json:"commitadditions"`
	CommitDeletions int    `json:"commitdeletions`
	MergeAdditions  int    `json:"mergeadditions"`
	MergeDeletions  int    `json:"mergedeletions"`
	ReviewAdditions int    `json:"reviewadditions"`
	ReviewDeletions int    `json:"reviewdeletions"`
}

type PullRequestInformation struct {
	Repoistory string `json:"repo"`
	Number     int    `json:"number"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Date       string `json:"date"`
}

type ReviewInformation struct {
	Repoistory string `json:"repo"`
	Number     int    `json:"number"`
	Additions  int    `json:"additions"`
	Deletions  int    `json:"deletions"`
	Date       string `json:"date"`
}
