package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"time"
)

const (
	apiPullsRequestURL       = `https://api.github.com/repos/%s/%s/pulls?per_page=250&page=%d&state=all&sort=updated&direction=desc`
	apiPullRequestURL        = `https://api.github.com/repos/%s/%s/pulls/%d`
	apiPullRequestCommitsURL = `https://api.github.com/repos/%s/%s/pulls/%d/commits?per_page=250&page=%d&sort=updated&direction=desc`
)

func (a *Client) FetchPullRequest(org, repo string, prNum int) ([]byte, error) {
	url := fmt.Sprintf(apiPullRequestURL, org, repo, prNum)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	a.RateLimit()
	res, err := a.gh.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http returned %v", res.StatusCode)
	}

	return ioutil.ReadAll(res.Body)
}

// FetchPullsRequest
func (a *Client) FetchPullsRequest(org, repo string) ([]ApiPullsRequest, error) {
	var totalPullsRequests []ApiPullsRequest
	page := 1
	for {
		url := fmt.Sprintf(apiPullsRequestURL, org, repo, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return totalPullsRequests, err
		}
		a.RateLimit()
		res, err := a.gh.Do(req)
		if err != nil {
			return totalPullsRequests, err
		}

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return totalPullsRequests, err
		}

		var pullsRequests []ApiPullsRequest
		err = json.Unmarshal(body, &pullsRequests)
		if err != nil {
			return totalPullsRequests, err
		}

		// no more left
		if len(pullsRequests) == 0 {
			break
		}

		for _, pullsRequest := range pullsRequests {
			/*
				t, err := time.Parse(time.RFC3339, pullsRequest.UpdatedAt)
				if err != nil {
					fmt.Fprintf(os.Stderr, "Failed to parse time: %v\n", err)
					continue
				}
				if lastUpdated.After(t) || lastUpdated.Equal(t) {
					return totalPullsRequests, nil
				}
			*/
			totalPullsRequests = append(totalPullsRequests, pullsRequest)
		}
		page++
	}
	return totalPullsRequests, nil
}

func (a *Client) FetchPullRequestCommits(org, repo string, prNum int, monthYear time.Time) ([]ApiPullRequestCommit, error) {
	var totalPullRequestCommits []ApiPullRequestCommit
	page := 1
	for {
		url := fmt.Sprintf(apiPullRequestCommitsURL, org, repo, prNum, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return totalPullRequestCommits, err
		}
		a.RateLimit()
		res, err := a.gh.Do(req)
		if err != nil {
			return totalPullRequestCommits, err
		}

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return totalPullRequestCommits, err
		}

		var pullRequestCommits []ApiPullRequestCommit
		err = json.Unmarshal(body, &pullRequestCommits)
		if err != nil {
			return totalPullRequestCommits, err
		}

		// no more left
		if len(pullRequestCommits) == 0 {
			break
		}

		for _, commit := range pullRequestCommits {
			/*
			           t, err := time.Parse(time.RFC3339, commit.Commit.Committer.Date)
			           if err != nil {
			                   fmt.Fprintf(os.Stderr, "Failed to parse time: %v\n", err)
			                   continue
			           }
			           if monthYear.After(t) {
			                   return totalPullRequestCommits, nil
			           }
			           if monthYear.Month() != t.Month() || monthYear.Year() != t.Year() {
			                   // skip
			   continue
			   }
			*/
			totalPullRequestCommits = append(totalPullRequestCommits, commit)
		}
		page++
	}
	return totalPullRequestCommits, nil

}
