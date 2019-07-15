package api

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"time"
)

const (
	apiPullRequestReviewsURL = `https://api.github.com/repos/%s/%s/pulls/%d/reviews?per_page=250&page=%d`
)

func (a *Client) FetchPullRequestReviews(org, repo string, prNum int, lastUpdated time.Time) ([]ApiPullRequestReview, error) {
	var totalPullRequestReviews []ApiPullRequestReview
	page := 1
	for {
		url := fmt.Sprintf(apiPullRequestReviewsURL, org, repo, prNum, page)
		req, err := http.NewRequest("GET", url, nil)
		if err != nil {
			return nil, err
		}
		a.RateLimit()
		res, err := a.gh.Do(req)
		if err != nil {
			return nil, err
		}

		body, err := ioutil.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return totalPullRequestReviews, err
		}
		if res.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("http returned %v", res.StatusCode)
		}
		if len(body) == 0 {
			return totalPullRequestReviews, nil
		}

		var pullRequestReviews []ApiPullRequestReview
		err = json.Unmarshal(body, &pullRequestReviews)
		if err != nil {
			return totalPullRequestReviews, err
		}

		// no more left
		if len(pullRequestReviews) == 0 {
			break
		}

		for _, review := range pullRequestReviews {
			t, err := time.Parse(time.RFC3339, review.SubmittedAt)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Failed to parse time: %v\n", err)
				continue
			}
			if lastUpdated.After(t) || lastUpdated.Equal(t) {
				return totalPullRequestReviews, nil
			}
			totalPullRequestReviews = append(totalPullRequestReviews, review)
		}
		page++
	}
	return totalPullRequestReviews, nil
}
