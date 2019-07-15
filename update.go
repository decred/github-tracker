package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github-tracker/api"
	"github.com/davecgh/go-spew/spew"
	bolt "go.etcd.io/bbolt"
)

func NewServer(token string) (*server, error) {
	tc := api.NewClient(token)

	db, err := NewDB()
	if err != nil {
		return nil, err
	}

	return &server{
		tc: tc,
		db: db,
	}, nil
}

func (s *server) Update(org, repository string) error {
	// Fetch the organization's repositories
	repos, err := s.tc.FetchOrgRepos(org)
	if err != nil {
		err = fmt.Errorf("FetchOrgRepos: %v", err)
		return err
	}

	// Create an organization bucket to hold repo buckets
	if err := s.db.Update(func(tx *bolt.Tx) error {
		orgBucket, err := tx.CreateBucketIfNotExists([]byte(org))
		if err != nil {
			return err
		}
		// create the repo buckets
		for _, repo := range repos {
			_, err := orgBucket.CreateBucketIfNotExists([]byte(repo.FullName))
			if err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}
	for _, repo := range repos {
		if repository != "" && repository != repo.FullName {
			continue
		}
		log.Printf("%s", repo.Name)
		log.Printf("Syncing %s", repo.FullName)

		// Grab latest sync time
		prs, err := s.tc.FetchPullsRequest(org, repo.Name)
		if err != nil {
			return err
		}

		for _, pr := range prs {
			var prNum [8]byte
			binary.LittleEndian.PutUint64(prNum[:], uint64(pr.Number))
			if err := s.db.Update(func(tx *bolt.Tx) error {
				orgBucket := tx.Bucket([]byte(org))
				repoBucket := orgBucket.Bucket([]byte(repo.FullName))

				prBucket, err := repoBucket.CreateBucketIfNotExists(prNum[:])
				if err != nil {
					return err
				}
				commitBucket, err := prBucket.CreateBucketIfNotExists([]byte("commits"))
				if err != nil {
					return err
				}
				reviewBucket, err := prBucket.CreateBucketIfNotExists([]byte("reviews"))
				if err != nil {
					return err
				}

				var prUpdatedAt time.Time
				var pullRequest *api.ApiPullRequest
				prBytes := prBucket.Get([]byte("pullrequest"))

				if prBytes != nil {
					err := json.Unmarshal(prBytes, &pullRequest)
					if err != nil {
						return err
					}
					dbUpdatedAt := parseTime(pullRequest.UpdatedAt)
					if dbUpdatedAt.IsZero() {
						prBytes, err = s.tc.FetchPullRequest(org, repo.Name, pr.Number)
						if err != nil {
							return err
						}
						if len(prBytes) == 0 {
							return fmt.Errorf("%s/%s: empty pull request", org, repo.Name)
						}
						err = json.Unmarshal(prBytes, &pullRequest)
						if err != nil {
							return err
						}
						err = prBucket.Put([]byte("pullrequest"), prBytes)
						if err != nil {
							return err
						}

					}
					prUpdatedAt = parseTime(pr.UpdatedAt)

					if prUpdatedAt.After(dbUpdatedAt) || prUpdatedAt.Equal(dbUpdatedAt) {
						log.Printf("\tPR %v: no update", pr.Number)
						return nil
					}
				}
				log.Printf("\tPR %d", pr.Number)
				prCommits, err := s.tc.FetchPullRequestCommits(org, repo.Name, pr.Number, prUpdatedAt)
				if err != nil {
					return err
				}

				shas := make(map[string]bool)
				//
				// This chunk looks for additional reviews on discarded commits
				//
				// active commits
				for _, commit := range prCommits {
					shas[commit.SHA] = false
				}

				prReviews, err := s.tc.FetchPullRequestReviews(org, repo.Name, pr.Number, prUpdatedAt)
				if err != nil {
					panic(err)
				}

				for _, prReview := range prReviews {
					if prReview.CommitID == "" {
						continue
					}
					if _, exists := shas[prReview.CommitID]; !exists {
						shas[prReview.CommitID] = true
					}
				}

				for sha, discarded := range shas {
					log.Printf("\t\tcommit %s - discarded:%v", sha, discarded)
					commit, err := s.tc.FetchCommit(org, repo.Name, sha)
					if err != nil {
						return err
					}
					commit.Discarded = discarded
					commitBytes, err := json.Marshal(commit)
					if err != nil {
						return err
					}
					err = commitBucket.Put([]byte(commit.SHA), commitBytes)
					if err != nil {
						return fmt.Errorf("commitBucket.Put: %v %v\n%v", commit.SHA, err, spew.Sdump(commit))
					}
					log.Printf("\t\tadded commit: %s", commit.SHA)
				}

				for _, prReview := range prReviews {
					prReviewBytes, err := json.Marshal(prReview)
					if err != nil {
						return err
					}
					id := fmt.Sprintf("%d", prReview.ID)
					err = reviewBucket.Put([]byte(id), prReviewBytes)
					if err != nil {
						return fmt.Errorf("reviewBucket.Put: %v %v\n%v", prReview.ID, err, spew.Sdump(prReview))
					}
					log.Printf("\t\tadded review: %d", prReview.ID)
				}
				timelineBytes, err := s.tc.Timeline(org, repo.Name, pr.Number)
				if err != nil {
					return err
				}
				err = prBucket.Put([]byte("timeline"), timelineBytes)
				if err != nil {
					return err
				}
				log.Printf("\t\tadded timeline")

				prBytes, err = s.tc.FetchPullRequest(org, repo.Name, pr.Number)
				if err != nil {
					return err
				}
				if len(prBytes) == 0 {
					return fmt.Errorf("%s/%s: empty pull request", org, repo.Name)
				}
				err = prBucket.Put([]byte("pullrequest"), prBytes)
				if err != nil {
					return err
				}
				log.Printf("\t\tadded pull request")

				return nil
			}); err != nil {
				return err
			}
		}
	}

	return nil
}
