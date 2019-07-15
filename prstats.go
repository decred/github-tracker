package main

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"

	"github-tracker/api"
	"github.com/davecgh/go-spew/spew"
	bolt "go.etcd.io/bbolt"
)

func (s *server) PRStats(org, repo string, prNumber int) error {
	var prNum [8]byte
	binary.LittleEndian.PutUint64(prNum[:], uint64(prNumber))

	fullRepo := fmt.Sprintf("%s/%s", org, repo)
	if err := s.db.View(func(tx *bolt.Tx) error {
		orgBucket := tx.Bucket([]byte(org))
		if orgBucket == nil {
			return fmt.Errorf("invalid organization")
		}
		repoBucket := orgBucket.Bucket([]byte(fullRepo))
		if repoBucket == nil {
			return fmt.Errorf("invalid repo: %s", repo)
		}
		prBucket := repoBucket.Bucket(prNum[:])
		if prBucket == nil {
			return fmt.Errorf("invalid PR: %s", prNumber)
		}
		reviewBucket := prBucket.Bucket([]byte("reviews"))

		if err := reviewBucket.ForEach(func(k, v []byte) error {
			if k == nil {
				return fmt.Errorf("reviewBucket contains nil key")
			}
			var review api.ApiPullRequestReview
			err := json.Unmarshal(v, &review)
			if err != nil {
				return err
			}
			spew.Dump(review)
			return nil
		}); err != nil {
			return err
		}
		return nil
	}); err != nil {
		log.Printf("ERROR: %v", err)
		return err
	}
	return nil
}
