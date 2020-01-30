module github.com/decred/github-tracker/database/cockroachdb

go 1.14

require (
	github.com/decred/slog v1.0.0
	github.com/jinzhu/gorm v1.9.12
	github.com/decred/github-tracker/database v0.0.0-00010101000000-000000000000
)

replace (
	github.com/decred/github-tracker/database => ../../database
)