module github-tracker

go 1.13

require (
	github.com/decred/dcrd/certgen v1.1.0
	github.com/decred/dcrd/chaincfg v1.5.2 // indirect
	github.com/decred/dcrd/dcrec/secp256k1 v1.0.3 // indirect
	github.com/decred/dcrd/dcrjson/v3 v3.0.1
	github.com/decred/dcrd/dcrutil v1.4.0
	github.com/decred/dcrd/dcrutil/v2 v2.0.1
	github.com/decred/dcrwallet/version v1.0.5
	github.com/decred/github-tracker/api v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/database v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/database/cockroachdb v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/jsonrpc v0.0.0-00010101000000-000000000000
	github.com/decred/github-tracker/jsonrpc/types v0.0.0-24e5946600460fc6cf063593a9503baaf49aa650
	github.com/decred/github-tracker/server v0.0.0-00010101000000-000000000000
	github.com/decred/go-socks v1.1.0
	github.com/decred/slog v1.0.0
	github.com/golang/protobuf v1.3.2 // indirect
	github.com/jessevdk/go-flags v1.4.0
	github.com/jrick/logrotate v1.0.0
	github.com/lib/pq v1.2.0 // indirect
	github.com/pkg/errors v0.8.1
	golang.org/x/net v0.0.0-20191028085509-fe3aa8a45271 // indirect
	golang.org/x/xerrors v0.0.0-20191011141410-1b5146add898 // indirect
	google.golang.org/appengine v1.6.1 // indirect
)

replace (
	github.com/decred/github-tracker/api => ./api
	github.com/decred/github-tracker/database => ./database
	github.com/decred/github-tracker/database/cockroachdb => ./database/cockroachdb
	github.com/decred/github-tracker/jsonrpc => ./jsonrpc
	github.com/decred/github-tracker/jsonrpc/types => ./jsonrpc/types
	github.com/decred/github-tracker/server => ./server
)
