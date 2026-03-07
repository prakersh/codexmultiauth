# Verification Snapshot

Date: 2026-03-08

## Commands Run

- `go test ./... -count=1`
- `go test -race ./... -count=1`
- `go test ./... -covermode=atomic -coverprofile=coverage.out`
- `go tool cover -func=coverage.out`
- `go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out`
- `go tool cover -func=coverage_internal.out`
- `GOOS=darwin GOARCH=arm64 go build ./...`
- `GOOS=darwin GOARCH=amd64 go build ./...`
- `GOOS=linux GOARCH=amd64 go build ./...`
- `GOOS=linux GOARCH=arm64 go build ./...`

## Result Summary

- Test matrix: pass
- Race tests: pass
- Build matrix: pass
- Overall coverage: `85.3%`
- Internal coverage: `85.1%`

## Package Coverage Targets

- `internal/app`: `87.7%`
- `internal/infra/crypto`: `93.4%`
- `internal/infra/fs`: `87.7%`
- `internal/infra/store`: `82.3%`
- `internal/infra/usage`: `91.2%`
- `internal/tui`: `80.4%`
