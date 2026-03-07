# CodexMultiAuth (`cma`)

CodexMultiAuth manages multiple Codex accounts from one CLI/TUI. It saves Codex auth credentials in an encrypted vault, switches active accounts safely, and supports encrypted backup and restore.

Repository: https://github.com/prakersh/codexmultiauth

## Project Purpose

- Manage multiple Codex identities in one place.
- Keep saved credentials encrypted at rest.
- Provide atomic and rollback-safe account mutation flows.
- Offer both CLI commands and a terminal UI.

## Requirements

- Go `1.24.2` (from `go.mod`)
- A working `codex` CLI on `PATH` for `cma new`
- Optional OS keyring support (CMA falls back to a local key file when keyring is disabled or unavailable)

## Build and Install

```bash
go build -o cma .
./cma --help
```

## `app.sh` Workflow (Canonical Entrypoint)

Use `./app.sh` for contributor and maintainer workflows:

```bash
# quick smoke checks
./app.sh --smoke

# full verification matrix
./app.sh --verify

# build binary at ./bin/cma with ldflags metadata
./app.sh --build

# run CLI through orchestrator
./app.sh --run -- version
./app.sh --run -- tui

# release binaries to ./dist (darwin/linux, amd64/arm64)
./app.sh --release
```

`app.sh` reads the default app version from `cmd/VERSION` and injects build metadata (`cmd.Version`, `cmd.Commit`, `cmd.Date`) via ldflags.

## Quick Start

```bash
# 1) Save current Codex auth
./cma save

# 2) List saved accounts
./cma list

# 3) Activate one account by selector
./cma activate 1

# 4) Create encrypted backup (interactive passphrase prompt)
./cma backup prompt my-snapshot

# 5) Restore from backup (interactive selection by default)
./cma restore prompt my-snapshot

# 6) Launch TUI
./cma tui

# 7) Show version and public links
./cma version
```

## Command Overview

- `cma list`
- `cma usage <selector|all>`
- `cma version [--short]`
- `cma save`
- `cma new [--device-auth]`
- `cma activate <selector>`
- `cma delete <selector>`
- `cma backup <encrypthash/pass> <name|abspath> [--allow-plain-pass-arg]`
- `cma restore <encrypthash/pass> <pathtobackup|name> [--all] [--conflict ask|overwrite|skip|rename] [--allow-plain-pass-arg]`
- `cma tui`

See [docs/COMMANDS.md](docs/COMMANDS.md) for full syntax and examples.

## Versioning Process

- Source of default version: `cmd/VERSION`.
- `cma version --short` prints only the resolved version.
- Build-time override is supported through ldflags.

Example build with explicit version, commit, and date:

```bash
VERSION=$(cat cmd/VERSION)
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags "-X github.com/prakersh/codexmultiauth/cmd.Version=${VERSION} -X github.com/prakersh/codexmultiauth/cmd.Commit=${COMMIT} -X github.com/prakersh/codexmultiauth/cmd.Date=${DATE}" -o cma .
```

For tagged releases, set `cmd/VERSION` to the release version before build/tag.

## Security Model Summary

- CMA vault and backups are encrypted with `XChaCha20-Poly1305`.
- Backup passphrases use `Argon2id` for key derivation.
- Files are written with `0600`; directories with `0700`.
- Mutating flows use lock + atomic write + verification + rollback.
- Secrets are not printed in normal output.

See [docs/SECURITY.md](docs/SECURITY.md) for details.

## TUI Overview

`cma tui` supports:

- account list and active marker
- usage refresh for selected account
- save / activate / delete actions
- backup flow (name + passphrase)
- restore flow with inspect, selective/all toggle, and conflict policy handling

See [docs/BACKUP_RESTORE.md](docs/BACKUP_RESTORE.md) for restore behavior.

## Support

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/prakersh)
