# Verification and Release Checklist

## Verification Commands

Primary project checks:

```bash
./app.sh --smoke
./app.sh --verify
./app.sh --verify-sandbox
```

Explicit matrix:

```bash
go test ./... -count=1
go test -race ./... -count=1
go test ./... -covermode=atomic -coverprofile=coverage.out
go tool cover -func=coverage.out
go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out
go tool cover -func=coverage_internal.out
GOOS=darwin GOARCH=arm64 go build ./...
GOOS=darwin GOARCH=amd64 go build ./...
GOOS=linux GOARCH=amd64 go build ./...
GOOS=linux GOARCH=arm64 go build ./...
```

## Sandbox Gate

`./app.sh --verify-sandbox` is the release gate.

It runs verification in a temp sandbox with:

- `HOME=<temp>/home`
- `XDG_CONFIG_HOME=<temp>/xdg`
- `CODEX_HOME=<temp>/codex`
- `CMA_DISABLE_KEYRING=1`

It also compares host metadata before and after the run for:

- `~/.codex/auth.json` if present
- `~/.config/cma` if present

Pass criteria:

- full verify matrix passes
- coverage gates remain green
- host metadata is unchanged

## Release Artifacts

Build artifacts with:

```bash
./app.sh --release
```

Expected outputs:

- `dist/cma_<version>_darwin_amd64`
- `dist/cma_<version>_darwin_arm64`
- `dist/cma_<version>_linux_amd64`
- `dist/cma_<version>_linux_arm64`
- `dist/sha256sums.txt`

Pass criteria:

- all four binaries exist
- `sha256sums.txt` exists

## GitHub Release Flow

Recommended draft-first path:

```bash
./app.sh --publish-release --draft --notes-file docs/release-notes.md
```

If `--notes-file` is omitted, `app.sh` generates a short draft notes file automatically.

Publish checks:

- `gh` is installed
- `gh auth status` succeeds
- sandbox verification passes
- release artifacts and checksums exist
- remote tag does not already exist

To publish a reviewed draft release:

```bash
gh release edit v$(cat cmd/VERSION) --draft=false
```

## Current Targets

- overall coverage: `>= 80%`
- internal coverage: `>= 80%`
- `internal/app >= 85%`
- `internal/infra/crypto >= 90%`
- `internal/infra/fs >= 85%`
- `internal/infra/store >= 80%`
- `internal/infra/usage >= 80%`
- `internal/tui >= 60%`
