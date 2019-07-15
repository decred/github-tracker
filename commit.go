package main

import "github-tracker/api"

type sorter []api.ApiPullRequestCommit

func (s sorter) Len() int {
	return len(s)
}

func (s sorter) Less(i, j int) bool {
	a := parseTime(s[i].Commit.Committer.Date)
	b := parseTime(s[j].Commit.Committer.Date)

	return !a.After(b)
}

func (s sorter) Swap(i, j int) {
	s[i], s[j] = s[j], s[i]
}
