package main

import (
	bolt "go.etcd.io/bbolt"
)

func NewDB() (*bolt.DB, error) {
	db, err := bolt.Open("github.db", 0700, nil)
	if err != nil {
		return nil, err
	}

	return db, nil
}
