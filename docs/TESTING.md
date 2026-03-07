# Testing and Verification

## Canonical Workflow

`./app.sh` is the project entrypoint for checks:

```bash
./app.sh --test          # go test ./... -count=1
./app.sh --race          # go test -race ./... -count=1
./app.sh --cover         # coverage commands + gate status
./app.sh --verify        # full matrix (tests, race, coverage, cross-builds)
./app.sh --smoke         # vet + build + short tests
```

## Core Test Commands

```bash
go test ./... -count=1
go test -race ./... -count=1
```

## Coverage Commands

```bash
go test ./... -covermode=atomic -coverprofile=coverage.out
go tool cover -func=coverage.out

go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out
go tool cover -func=coverage_internal.out
```

## Current Coverage Gates

- Overall (`./...`): `>= 80%`
- Internal (`./internal/...`): `>= 80%`
- Package minimums:
  - `internal/app >= 85%`
  - `internal/infra/crypto >= 90%`
  - `internal/infra/fs >= 85%`
  - `internal/infra/store >= 80%`
  - `internal/infra/usage >= 80%`
  - `internal/tui >= 60%`

## Build Matrix

```bash
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=linux GOARCH=arm64 go build ./...
```

These commands are executed by `./app.sh --verify`.

## Test Areas Covered

- crypto envelope and passphrase error paths
- filesystem locking and atomic rollback paths
- app service flows (save/new/activate/delete/backup/restore/usage)
- selector and conflict policy behavior
- CLI command wiring and prompts
- TUI workflows including restore review/conflict decisions
- integration checks for atomicity and plaintext leak guards
