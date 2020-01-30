package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"time"

	"github.com/decred/github-tracker/database"
)

func (s *Server) Update(ctx context.Context, org string) error {
	// Fetch the organization's repositories
	repos, err := s.tc.FetchOrgRepos(org)
	if err != nil {
		err = fmt.Errorf("FetchOrgRepos: %v", err)
		return err
	}

	for _, repo := range repos {
		log.Infof("%s", repo.Name)
		log.Infof("Syncing %s", repo.FullName)

		// Let the current repo finish before exiting on cancel.
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Grab latest sync time
		prs, err := s.tc.FetchPullsRequest(org, repo.Name)
		if err != nil {
			return err
		}

		for _, pr := range prs {
			var prNum [8]byte
			binary.LittleEndian.PutUint64(prNum[:], uint64(pr.Number))

			apiPR, err := s.tc.FetchPullRequest(org, repo.Name, pr.Number)
			if err != nil {
				return err
			}
			dbPullRequest, err := convertAPIPullRequestToDbPullRequest(apiPR, *repo, org)
			if err != nil {
				log.Errorf("error converting api PR to database: %v", err)
				continue
			}
			dbPR, err := s.DB.PullRequestByURL(pr.URL)
			if err != nil {
				if err == database.ErrNoPullRequestFound {
					prCommits, err := s.tc.FetchPullRequestCommits(org, repo.Name, pr.Number, parseTime(pr.UpdatedAt))
					if err != nil {
						return err
					}

					commits := convertAPICommitsToDbCommits(prCommits)
					dbPullRequest.Commits = commits

					prReviews, err := s.tc.FetchPullRequestReviews(org, repo.Name, pr.Number, parseTime(pr.UpdatedAt))
					if err != nil {
						panic(err)
					}

					reviews := convertAPIReviewsToDbReviews(prReviews)
					dbPullRequest.Reviews = reviews

					err = s.DB.NewPullRequest(dbPullRequest)
					if err != nil {
						log.Errorf("error adding new pull request: %v", err)
						continue
					}
				} else {
					log.Errorf("error locating pull request: %v", err)
					continue
				}
			}
			// Only update if dbPR is found and Uqpdated is more recent than what is currently stored.
			if dbPR != nil && time.Unix(dbPR.UpdatedAt, 0).After(parseTime(pr.UpdatedAt)) {
				log.Infof("\tUpdate PR %d", pr.Number)
				prCommits, err := s.tc.FetchPullRequestCommits(org, repo.Name, pr.Number, parseTime(pr.UpdatedAt))
				if err != nil {
					return err
				}

				commits := convertAPICommitsToDbCommits(prCommits)
				dbPullRequest.Commits = commits

				prReviews, err := s.tc.FetchPullRequestReviews(org, repo.Name, pr.Number, parseTime(pr.UpdatedAt))
				if err != nil {
					panic(err)
				}

				reviews := convertAPIReviewsToDbReviews(prReviews)
				dbPullRequest.Reviews = reviews

				err = s.DB.UpdatePullRequest(dbPullRequest)
				if err != nil {
					log.Errorf("error updating new pull request: %v", err)
					continue
				}

			}
		}
	}

	return nil
}
