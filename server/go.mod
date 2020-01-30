module github.com/decred/github-tracker/server

go 1.14

require (
	github.com/davecgh/go-spew v1.1.1
	github.com/decred/dcrd/dcrjson/v3 v3.0.1 // indirect
	github.com/decred/github-tracker/api v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/database v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/jsonrpc/types v0.0.0-00010101000000-000000000000
	github.com/decred/slog v1.0.0
	go.etcd.io/bbolt v1.3.3
)

replace (
	github.com/decred/github-tracker/api => ../api
	github.com/decred/github-tracker/database => ../database
	github.com/decred/github-tracker/jsonrpc/types => ../jsonrpc/types
)
