# Testing and Verification

## Canonical Workflow

`./app.sh` is the project entrypoint for checks:

```bash
./app.sh --test          # go test ./... -count=1
./app.sh --race          # go test -race ./... -count=1
./app.sh --cover         # coverage commands + gate status
./app.sh --verify        # full matrix (tests, race, coverage, cross-builds)
./app.sh --verify-sandbox # full matrix in isolated temp HOME/XDG/CODEX
./app.sh --smoke         # vet + build + short tests
```

Fast wrapper commands:

```bash
./test.sh quick
./test.sh full
./test.sh prerelease
./test.sh publish -- --notes-file docs/release-notes.md
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

## Sandbox Verification Gate

Use `./app.sh --verify-sandbox` before release work.

It creates a temp sandbox and runs verification with:

- `HOME=<temp>/home`
- `XDG_CONFIG_HOME=<temp>/xdg`
- `CODEX_HOME=<temp>/codex`
- `CMA_DISABLE_KEYRING=1`

The stage also compares host metadata before and after the run to confirm that the caller's real Codex and CMA paths were not mutated.

## Release Automation Checks

`./app.sh --release` builds these artifacts and writes `dist/sha256sums.txt`:

- `cma_<version>_darwin_amd64`
- `cma_<version>_darwin_arm64`
- `cma_<version>_linux_amd64`
- `cma_<version>_linux_arm64`

`./app.sh --publish-release --draft` adds these gates:

- sandbox verification must pass
- release artifacts and checksums must exist
- `gh auth status` must pass
- remote tag must not already exist
- release notes must come from `--notes-file` or generated draft notes

## Test Areas Covered

- crypto envelope and passphrase error paths
- filesystem locking and atomic rollback paths
- app service flows (save/new/activate/delete/backup/restore/usage)
- selector and conflict policy behavior
- CLI command wiring and prompts
- TUI workflows including restore review/conflict decisions
- integration checks for atomicity and plaintext leak guards
