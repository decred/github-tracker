package main

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	//"sort"
	//"strconv"
	"time"

	"github-tracker/api"
	bolt "go.etcd.io/bbolt"
)

type S struct {
	Additions int
	Deletions int
	Total     int
}

type server struct {
	tc *api.Client
	db *bolt.DB
}

func yearMonth(t time.Time) string {
	return fmt.Sprintf("%d%02d", t.Year(), t.Month())
}

var (
	ErrNotPR = errors.New("not requested pr")
)

func main() {
	cfg := loadConfig()
	token := "975b07d598ac18456f6657e6a6cdebbdd1d9ca35"

	s, err := NewServer(token)
	if err != nil {
		fmt.Printf("NewServer: %v\n", err)
		os.Exit(1)
	}
	defer s.db.Close()

	if cfg.Update {
		if err := s.Update(cfg.Org, cfg.Repo); err != nil {
			fmt.Fprintf(os.Stderr, "update failed: %v\n", err)
			os.Exit(1)
		}
	}

	// create
	var csvB bytes.Buffer
	csvWriter := csv.NewWriter(&csvB)
	var csvData [12]string
	csvData[0] = "Year"
	csvData[1] = "Month"
	csvData[2] = "Login"
	csvData[3] = "Organization"
	csvData[4] = "Repository"
	csvData[5] = "Commit Additions"
	csvData[6] = "Commit Deletions"
	csvData[7] = "Master Additions"
	csvData[8] = "Master Deletions"
	csvData[9] = "Review Additions"
	csvData[10] = "Review Deletions"
	csvData[11] = "Closed Unmerged PRs"
	err = csvWriter.Write(csvData[:])
	if err != nil {
		panic(err)
	}
	csvWriter.Flush()
	if err := csvWriter.Error(); err != nil {
		panic(err)
	}
	fmt.Printf("%s", csvB.Bytes())

	org := cfg.Org
	if err := s.db.View(func(tx *bolt.Tx) error {
		orgBucket := tx.Bucket([]byte(org))
		if orgBucket == nil {
			return fmt.Errorf("no organizations loaded -- try updating")
		}
		if err := orgBucket.ForEach(func(k, v []byte) error {
			if k == nil {
				return fmt.Errorf("orgBucket contains nil key")
			}
			repoName := string(k)

			// If the repository was specified, skip all others...
			if cfg.Repo != "" && cfg.Repo != repoName {
				return nil
			}
			repoBucket := orgBucket.Bucket(k)

			for year := 2016; year <= 2019; year++ {
				for month := 1; month <= 12; month++ {
					ymTime := time.Date(year, time.Month(month), 0, 0, 0, 0, 0, time.UTC)
					ym := fmt.Sprintf("%d%02d", year, month)
					nonMergeStats := make(map[string]uint)
					globalMergeStats := make(map[string]S)
					globalCommitStats := make(map[string]S)
					globalReviewStats := make(map[string]S)

					if err := repoBucket.ForEach(func(k, v []byte) error {
						if k == nil {
							return nil
						}
						prBucket := repoBucket.Bucket(k)
						prBytes := prBucket.Get([]byte("pullrequest"))
						if prBytes == nil {
							return fmt.Errorf("missing pull request json in db")
						}
						var pullRequest api.ApiPullRequest
						err := json.Unmarshal(prBytes, &pullRequest)
						if err != nil {
							log.Printf("%s", string(prBytes))
							return err
						}
						// PR number was specified -- skip everything else...
						if cfg.PRNum != 0 && cfg.PRNum != pullRequest.Number {
							return nil
						}
						updatedAt := parseTime(pullRequest.UpdatedAt)
						if !updatedAt.After(ymTime) {
							return nil
						}

						reviewMap := make(map[string]map[string]S)
						reviewBucket := prBucket.Bucket([]byte("reviews"))
						commitBucket := prBucket.Bucket([]byte("commits"))
						if err := commitBucket.ForEach(func(k, v []byte) error {
							if k == nil {
								return fmt.Errorf("commitBucket contains nil key")
							}
							var commit api.ApiPullRequestCommit
							err := json.Unmarshal(v, &commit)
							if err != nil {
								log.Printf("%s", string(v))

								return err
							}

							// Scan reviews that include this commit
							commitTime := parseTime(commit.Commit.Committer.Date)
							if err := reviewBucket.ForEach(func(k, v []byte) error {
								if k == nil {
									return fmt.Errorf("reviewBucket contains nil key")
								}
								var review api.ApiPullRequestReview
								err := json.Unmarshal(v, &review)
								if err != nil {
									log.Printf("%s", string(v))
									return err
								}

								// user was specified, skip others
								if cfg.User != "" && cfg.User != review.User.Login {
									return nil
								}
								if review.User.Login == pullRequest.User.Login {
									//log.Printf("skipping self-review '%s'", review.User.Login)
									// do not count review on own PR
									return nil
								}
								reviewSubmitTime := parseTime(review.SubmittedAt)
								if yearMonth(reviewSubmitTime) != ym {
									// review not submitted on this scan range
									return nil
								}
								if !reviewSubmitTime.After(commitTime) {
									// review submitted before commit, so skip.
									return nil
								}

								reviewSHAs, exists := reviewMap[review.User.Login]
								if !exists {
									s := make(map[string]S)
									s[commit.SHA] = S{
										Additions: commit.Stats.Additions,
										Deletions: commit.Stats.Deletions,
										Total:     commit.Stats.Total,
									}
									reviewMap[review.User.Login] = s
									// ACCEPTED
									//log.Printf("review,%s,%d,%s,%s,%d,%d", repoName, pullRequest.Number, review.User.Login, commit.SHA, commit.Stats.Additions, commit.Stats.Deletions)
									return nil
								}
								if _, exists = reviewSHAs[commit.SHA]; exists {
									// DUPLICATE review on SHA
									return nil
								}
								reviewMap[review.User.Login][commit.SHA] = S{
									Additions: commit.Stats.Additions,
									Deletions: commit.Stats.Deletions,
									Total:     commit.Stats.Total,
								}
								//log.Printf("review,%s,%s,%d,%d", review.User.Login, commit.SHA, commit.Stats.Additions, commit.Stats.Deletions)

								return nil
							}); err != nil { //reviewBucket.ForEach
								return err
							}

							if commit.Discarded {
								// no commit stats for discarded commits
								return nil
							}
							if yearMonth(commitTime) != ym {
								// commit did not occur during this scan range
								return nil
							}

							login := commit.Author.Login
							stats := commit.Stats

							// user was specified, so skip everything else...
							if cfg.User != "" && cfg.User != login {
								return nil
							}

							gstats, exists := globalCommitStats[login]
							if !exists {
								s := S{
									Additions: stats.Additions,
									Deletions: stats.Deletions,
									Total:     stats.Total,
								}
								//log.Printf("commit,%s,%d,%s,%s,%d,%d", repoName, pullRequest.Number, login, commit.SHA, stats.Additions, stats.Deletions)

								globalCommitStats[login] = s
								return nil
							}
							//log.Printf("commit,%s,%d,%s,%s,%d,%d", repoName, pullRequest.Number, login, commit.SHA, stats.Additions, stats.Deletions)

							gstats.Additions += stats.Additions
							gstats.Deletions += stats.Deletions
							gstats.Total += stats.Total
							globalCommitStats[login] = gstats
							return nil
						}); err != nil { // commitBucket.ForEach
							return err
						}

						// get approvals from the timeline
						timelineBytes := prBucket.Get([]byte("timeline"))
						if timelineBytes == nil {
							return fmt.Errorf("timeline missing")
						}
						var timelines []api.ApiTimeline
						err = json.Unmarshal(timelineBytes, &timelines)
						if err != nil {
							log.Printf("%s", string(timelineBytes))
							return err
						}
						userApprovals := make(map[string]struct{})
						var mergeUser string
						for _, timeline := range timelines {
							// Do not give credit if the person who opened the PR approved or
							// merged the PR.

							//              if timeline.User.Login == pullRequest.User.Login {
							//                      continue
							//              }

							switch timeline.Event {
							case "reviewed":
								if timeline.User.Login == pullRequest.User.Login {
									//log.Printf("skipping self-approval")
									continue
								}
								userApprovals[timeline.User.Login] = struct{}{}
							case "merged":
								mergeUser = timeline.User.Login
							default:
								continue
							}
						}

						// Gather merge statistics
						if pullRequest.State == "closed" {
							prTime := parseTime(pullRequest.ClosedAt)
							if yearMonth(prTime) == ym {
								login := pullRequest.User.Login
								if !pullRequest.Merged {
									if c, exists := nonMergeStats[login]; exists {
										nonMergeStats[login] = c + 1
									} else {
										nonMergeStats[login] = 1
									}
									//log.Printf("VERIFY: %d closed but not merged", pullRequest.Number)
								} else {
									selfMerge := login == pullRequest.MergedBy.Login
									if selfMerge {
										if len(userApprovals) == 0 {
											//	log.Printf("%v/%d self-merge by %s", repoName, pullRequest.Number, login)
										} else {
											if c, exists := globalMergeStats[login]; exists {
												c.Additions += pullRequest.Additions
												c.Deletions += pullRequest.Deletions
												globalMergeStats[login] = c
											} else {
												globalMergeStats[login] = S{
													Additions: pullRequest.Additions,
													Deletions: pullRequest.Deletions,
												}
											}
										}
									} else {
										//	log.Printf("merge,%s,%d,%d", pullRequest.MergedBy.Login, pullRequest.Additions, pullRequest.Deletions)
										if c, exists := globalMergeStats[pullRequest.MergedBy.Login]; exists {
											c.Additions += pullRequest.Additions
											c.Deletions += pullRequest.Deletions
											globalMergeStats[pullRequest.MergedBy.Login] = c
										} else {
											globalMergeStats[pullRequest.MergedBy.Login] = S{
												Additions: pullRequest.Additions,
												Deletions: pullRequest.Deletions,
											}
										}
										if c, exists := globalMergeStats[pullRequest.User.Login]; exists {
											c.Additions += pullRequest.Additions
											c.Deletions += pullRequest.Deletions
											globalMergeStats[pullRequest.User.Login] = c
										} else {
											globalMergeStats[pullRequest.User.Login] = S{
												Additions: pullRequest.Additions,
												Deletions: pullRequest.Deletions,
											}
										}
									}
								}
							}
						}

						// PR merged with approvals from the following.  give full pr credit.
						if pullRequest.Merged && yearMonth(parseTime(pullRequest.MergedAt)) == ym {
							for login := range userApprovals {
								// PR merged without approval
								// override current stats and give full pr credit.
								globalReviewStats[login] = S{
									Additions: pullRequest.Additions,
									Deletions: pullRequest.Deletions,
									Total:     pullRequest.Additions + pullRequest.Deletions,
								}
							}
							if len(userApprovals) == 0 && mergeUser != "" {
								// PR merged without approval
								// override current stats and give full pr credit, even to login
								// who opened the PR
								globalReviewStats[mergeUser] = S{
									Additions: pullRequest.Additions,
									Deletions: pullRequest.Deletions,
									Total:     pullRequest.Additions + pullRequest.Deletions,
								}
							}
						}

						// REVIEW
						for login, stats := range reviewMap {
							var a, d, t int
							for _, stat := range stats {
								a += stat.Additions
								d += stat.Deletions
								t += stat.Total
							}
							reviewStats, exists := globalReviewStats[login]
							if !exists {
								globalReviewStats[login] = S{
									Additions: a,
									Deletions: d,
									Total:     t,
								}
								continue
							}
							reviewStats.Additions += a
							reviewStats.Deletions += d
							reviewStats.Total += t
							globalReviewStats[login] = reviewStats
						}

						// end pr loop
						return nil
					}); err != nil {
						return err
					}

					// make distinct usernames
					logins := make(map[string]struct{})
					for k := range globalMergeStats {
						logins[k] = struct{}{}
					}
					for k := range globalReviewStats {
						logins[k] = struct{}{}
					}
					for k := range globalCommitStats {
						logins[k] = struct{}{}
					}

					if len(logins) > 0 {
						var csvB bytes.Buffer
						csvWriter := csv.NewWriter(&csvB)

						for login := range logins {
							var cAdd, cDel, mAdd, mDel, rAdd, rDel int
							var nonMerge uint
							if mergeStats, exists := globalMergeStats[login]; exists {
								mAdd = mergeStats.Additions
								mDel = mergeStats.Deletions
							}
							if commitStats, exists := globalCommitStats[login]; exists {
								cAdd = commitStats.Additions
								cDel = commitStats.Deletions
							}
							if reviewStats, exists := globalReviewStats[login]; exists {
								rAdd = reviewStats.Additions
								rDel = reviewStats.Deletions
							}
							if n, exists := nonMergeStats[login]; exists {
								nonMerge = n
							}
							csvData[0] = fmt.Sprintf("%d", year)
							csvData[1] = fmt.Sprintf("%d", month)
							csvData[2] = login
							csvData[3] = org
							csvData[4] = repoName
							csvData[5] = fmt.Sprintf("%d", cAdd)
							csvData[6] = fmt.Sprintf("%d", cDel)
							csvData[7] = fmt.Sprintf("%d", mAdd)
							csvData[8] = fmt.Sprintf("%d", mDel)
							csvData[9] = fmt.Sprintf("%d", rAdd)
							csvData[10] = fmt.Sprintf("%d", rDel)
							csvData[11] = fmt.Sprintf("%d", nonMerge)
							err := csvWriter.Write(csvData[:])
							if err != nil {
								panic(err)
							}
						}
						csvWriter.Flush()
						if err := csvWriter.Error(); err != nil {
							panic(err)
						}
						fmt.Printf("%s", csvB.Bytes())
					}
				}
			}
			// end repo loop
			return nil
		}); err != nil {
			return err
		}

		return nil
	}); err != nil {
		fmt.Printf("error: %v\n", err)
	}

	os.Exit(1)

	/*
				for mo := 1; mo < 13; mo++ {
					monthYear := time.Date(2017, time.Month(mo), 1, 0, 0, 0, 0, time.UTC)

					fmt.Fprintf(os.Stderr, "Processing %v-%v\n", monthYear.Year(), monthYear.Month())
					pullsRequests, err := fetchPullsRequest(org, repo, monthYear)
					if err != nil {
						fmt.Fprintf(os.Stderr, "FetchPullRequests failure: %v\n", err)
						os.Exit(1)
					}

					nonMergeStats := make(map[string]uint)
					globalMergeStats := make(map[string]S)
					globalCommitStats := make(map[string]S)
					globalUserStats := make(map[string]S)
					for _, pullsRequest := range pullsRequests {
						prNum := pullsRequest.Number
						shas := make(map[string]struct{})

						prCommits, err := s.tc.FetchPullRequestCommits(org, repo, prNum, monthYear)
						if err != nil {
							panic(err)
						}

						// active commits
						for _, commit := range prCommits {
							shas[commit.SHA] = struct{}{}
						}

						reviews, err := fetchPullRequestReviews(org, repo, prNum, monthYear)
						if err != nil {
							panic(err)
						}

						//fmt.Println(os.Stderr, "Inactive PR commits with reviews:")
						// skip - force push?
							//for _, review := range reviews {
								//if _, exists := shas[review.CommitID]; !exists {
								//	shas[review.CommitID] = struct{}{}
							//	}
						//	}

						//fmt.Fprintln(os.Stderr, "Fetching commits:")
						commits := make([]api.ApiPullRequestCommit, 0, len(shas))
						for sha := range shas {
							commit, err := s.tc.FetchCommit(org, repo, sha)
							if err != nil {
								panic(err)
							}
							commits = append(commits, *commit)
						}
						sort.Sort(sorter(commits))
						rr := make(map[string]S)

						//fmt.Fprintln(os.Stderr, "Processing commits...")
						for _, commit := range commits {
							login := commit.Author.Login
							stats := commit.Stats

							gstats, exists := globalCommitStats[login]
							if !exists {
								s := S{
									Additions: stats.Additions,
									Deletions: stats.Deletions,
									Total:     stats.Total,
								}
								globalCommitStats[login] = s
								continue
							}
							gstats.Additions += stats.Additions
							gstats.Deletions += stats.Deletions
							gstats.Total += stats.Total
							globalCommitStats[login] = gstats
						}

						//	fmt.Printf("Processing reviews...\n")
						for _, review := range reviews {
							login := review.User.Login
							if login == pullsRequest.User.Login {
								continue
							}
							submitTime := parseTime(review.SubmittedAt)
							//		fmt.Printf("%d: %v %v\n", i, login, submitTime)
							for _, commit := range commits {
								if commit.Commit.Committer.Date == "" {
									//spew.Dump(commit)
								}
								commitTime := parseTime(commit.Commit.Committer.Date)

								if !submitTime.After(commitTime) {
									//fmt.Printf("   IGNORING %v - %v\n", commit.SHA, commitTime)
									continue
								}

								stats := commit.Stats
								shaStats, exists := rr[login]
								if !exists {
									c := make(map[string]S)
									c[commit.SHA] = S{
										Additions: stats.Additions,
										Deletions: stats.Deletions,
										Total:     stats.Total,
									}
									rr[login] = c
									//fmt.Printf("  ACCEPTING %v - %v +%d -%d\n",
									//	commit.SHA, commitTime, stats.Additions, stats.Deletions)
									continue
								}
								if _, exists := shaStats[commit.SHA]; exists {
									//fmt.Printf("  DUPLICATE %v\n", commit.SHA)
									continue
								}
								s := S{
									Additions: stats.Additions,
									Deletions: stats.Deletions,
									Total:     stats.Total,
								}
								//fmt.Printf("  ACCEPTING %v - %v +%d -%d\n",
								//		commit.SHA, commitTime, s.Additions, s.Deletions)
								rr[login][commit.SHA] = s
							}
						}
						if pullsRequest.State == "closed" {
							pullRequest, err := fetchPullRequest(org, repo, prNum)
							if err != nil {
								fmt.Fprintf(os.Stderr, "fetchPullRequest failed: %v\n", err)
								os.Exit(1)
							}
							login := pullRequest.User.Login
							if !pullRequest.Merged {
								if c, exists := nonMergeStats[login]; exists {
									nonMergeStats[login] = c + 1
								} else {
									nonMergeStats[login] = 1
								}
								fmt.Fprintf(os.Stderr, "VERIFY: %d closed but not merged\n", prNum)
								continue
							}

							mergeTime := parseTime(pullRequest.MergedAt)
							good := monthYear.Month() == mergeTime.Month() &&
								monthYear.Year() == mergeTime.Year()

							if good {
								add := pullRequest.Additions
								del := pullRequest.Deletions
								gstats, exists := globalMergeStats[login]
								if !exists {
									s := S{
										Additions: add,
										Deletions: del,
									}
		//log.Printf("merge,%s,%s,%d,%d", review.User.Login, commit.SHA, commit.Stats.Additions, commit.Stats.Deletions)
									globalMergeStats[login] = s
								} else {
									gstats.Additions += add
									gstats.Deletions += del
									globalMergeStats[login] = gstats
								}
							}
						}

						// calc review stats
						for login, stats := range rr {
							var a, d, t int
							for _, stat := range stats {
								a += stat.Additions
								d += stat.Deletions
								t += stat.Total
							}

							gstats, exists := globalUserStats[login]
							if !exists {
								s := S{Additions: a, Deletions: d}
								globalUserStats[login] = s
								continue
							}
							gstats.Additions += a
							gstats.Deletions += d
							gstats.Total += t
							globalUserStats[login] = gstats
						}
					}

				}
	*/
	fmt.Fprintln(os.Stderr, "----- complete -----")
}

func parseTime(tstamp string) time.Time {
	t, err := time.Parse(time.RFC3339, tstamp)
	if err != nil {
		//err = fmt.Errorf("%s: %v", tstamp, err)
		//	panic(err)
		return time.Time{}
	}
	return t
}
