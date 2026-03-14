# Testing and Verification

## Canonical workflow

Use `./app.sh` as the source of truth for checks.

```bash
./app.sh --test           # go test ./... -count=1
./app.sh --race           # go test -race ./... -count=1
./app.sh --cover          # coverage commands + gate status
./app.sh --verify         # tests + race + coverage + cross-build matrix
./app.sh --verify-sandbox # full matrix in isolated temp HOME/XDG/CODEX
./app.sh --smoke          # vet + build + short tests
```

Shortcut wrapper:

```bash
./test.sh quick
./test.sh full
./test.sh prerelease
./test.sh publish -- --notes-file docs/release-notes.md
```

## Core test commands

```bash
go test ./... -count=1
go test -race ./... -count=1
```

## Coverage commands

```bash
go test ./... -covermode=atomic -coverprofile=coverage.out
go tool cover -func=coverage.out

go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out
go tool cover -func=coverage_internal.out
```

## Coverage gates

Current thresholds:

- overall (`./...`): `>= 80%`
- internal (`./internal/...`): `>= 80%`
- package minimums:
  - `internal/app >= 85%`
  - `internal/infra/crypto >= 90%`
  - `internal/infra/fs >= 85%`
  - `internal/infra/store >= 80%`
  - `internal/infra/usage >= 80%`
  - `internal/tui >= 60%`

## Build matrix

```bash
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=linux GOARCH=arm64 go build ./...
```

`./app.sh --verify` runs this matrix.

## Sandbox verification gate

Run `./app.sh --verify-sandbox` before release work.

It executes verification inside an isolated temporary environment:

- `HOME=<temp>/home`
- `XDG_CONFIG_HOME=<temp>/xdg`
- `CODEX_HOME=<temp>/codex`
- `CMA_DISABLE_KEYRING=1`

After verification, it compares host metadata to confirm real host Codex/CMA paths were not changed.

## Release automation checks

`./app.sh --release` builds dist artifacts and `dist/sha256sums.txt`:

- `cma_<version>_darwin_amd64`
- `cma_<version>_darwin_arm64`
- `cma_<version>_linux_amd64`
- `cma_<version>_linux_arm64`

`./app.sh --publish-release --draft` also requires:

- sandbox verification pass
- release artifacts and checksums present
- `gh auth status` pass
- remote tag does not already exist
- release notes from `--notes-file` or generated draft notes

## Coverage areas

Tests cover:

- crypto envelope behavior and passphrase error paths
- filesystem lock, atomic write, and rollback paths
- app service flows (`save`, `new`, `activate`, `delete`, `backup`, `restore`, `usage`)
- selector resolution and conflict policy behavior
- CLI command wiring and interactive prompts
- TUI workflows, including restore decisions
- integration checks for plaintext leak guards
