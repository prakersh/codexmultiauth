# CodexMultiAuth (`cma`)

Switch Codex accounts quickly when limits hit.

Codex is effectively single-active-auth on one machine. When one account hits its window, users often repeat logout and login loops across accounts. `cma` removes that friction with safe account switching, encrypted storage, and encrypted backup and restore.

Repository: https://github.com/prakersh/codexmultiauth

## Why `cma`

If you use multiple Codex accounts, setup is usually easy. Repeated switching is not. Manual auth-file handling is slow and error-prone.

`cma` provides:

- encrypted credential storage at rest
- atomic account activation with rollback
- encrypted backups with guided restore
- confidence-tiered usage reporting
- CLI and TUI workflows

## Who this is for

`cma` is built for users who need fast account rotation:

- power users handling multiple accounts
- consultants or agencies switching client identities
- teams that want fewer manual auth mistakes

## Core capabilities

- Save current Codex auth into an encrypted vault: `cma save`
- Switch active account safely: `cma activate <selector>`
- Auto-activate the best account by remaining quota and reset urgency: `cma auto`
- Create encrypted backups: `cma backup <encrypthash/pass> <name|abspath>`
- Restore selectively or all-at-once: `cma restore ... [--all]`
- View account usage with confidence labels: `cma usage <selector|all>`
- Show limits with account details, confidence, and reset windows: `cma limits`
- Run interactive terminal UI: `cma tui`

## Requirements

- Go `1.24.2`
- `codex` CLI on `PATH` (required for `cma login`)
- Optional OS keyring support (CMA falls back to file key storage when needed)

## Quick start

### 1) Build

```bash
go build -o cma .
./cma --help
```

### 2) Save and switch accounts

```bash
# Save current Codex account (encrypted)
./cma save

# List saved accounts
./cma list

# Activate an account by selector
./cma activate 1

# Auto-pick and activate the best account
./cma auto
```

### 3) Check usage and limits

```bash
# Usage for one account
./cma usage work

# Usage for all accounts
./cma usage all

# Limits view for all accounts
./cma limits

# Auto-pick the account with the best urgency-weighted remaining quota
./cma auto
```

### 4) Backup and restore

```bash
# Encrypted backup (interactive passphrase prompt)
./cma backup prompt weekly-backup

# Restore with interactive selection
./cma restore prompt weekly-backup

# Restore all entries atomically with conflict policy
./cma restore prompt weekly-backup --all --conflict overwrite
```

### 5) Launch the TUI

```bash
./cma tui
```

## `app.sh`: single contributor entrypoint

Use `./app.sh` for build, test, verification, and release workflows.

```bash
# Quick pre-commit checks
./app.sh --smoke

# Full host-shell verification matrix
./app.sh --verify

# Full verification in isolated temp HOME/XDG/CODEX
./app.sh --verify-sandbox

# Build local binary (./bin/cma)
./app.sh --build

# Run cma through orchestrator
./app.sh --run -- version
./app.sh --run -- tui

# Cross-platform release artifacts (./dist)
./app.sh --release

# Draft-first GitHub release publish
./app.sh --publish-release --draft --notes-file docs/release-notes.md
```

Execution order for combined flags:

`deps -> clean -> fmt -> lint -> test -> race -> cover -> build -> smoke -> verify -> verify-sandbox -> release -> publish-release -> run`

## `test.sh`: quick wrapper commands

`./test.sh` is a thin wrapper over `./app.sh`.

```bash
./test.sh quick
./test.sh full
./test.sh prerelease
./test.sh publish -- --notes-file docs/release-notes.md
```

Mappings:

- `./test.sh quick` -> `./app.sh --smoke`
- `./test.sh full` -> `./app.sh --verify-sandbox`
- `./test.sh prerelease` -> `./app.sh --verify-sandbox --release`
- `./test.sh publish` -> `./app.sh --publish-release --draft`

## Release workflow

Recommended path:

```bash
# 1. Verify in isolated temp HOME/XDG/CODEX
./app.sh --verify-sandbox

# 2. Build dist artifacts and checksums
./app.sh --release

# 3. Create GitHub draft release with assets
./app.sh --publish-release --draft --notes-file docs/release-notes.md
```

To publish a draft release after review:

```bash
gh release edit v$(cat cmd/VERSION) --draft=false
```

## Command overview

- `cma list`
- `cma usage <selector|all>`
- `cma limits`
- `cma auto`
- `cma save`
- `cma login [--device-auth|--with-api-key]`
- `cma new [--device-auth|--with-api-key]` alias for `cma login`
- `cma activate <selector>`
- `cma delete <selector>`
- `cma rename <selector> <new-name>`
- `cma backup <encrypthash/pass> <name|abspath> [--allow-plain-pass-arg]`
- `cma restore <encrypthash/pass> <pathtobackup|name> [--all] [--conflict ask|overwrite|skip|rename] [--allow-plain-pass-arg]`
- `cma version [--short]`
- `cma tui`

Full syntax and examples: [docs/COMMANDS.md](docs/COMMANDS.md)

## Versioning

`cma version` prints app version and public project links.

```bash
./cma version
./cma version --short
```

Version resolution order:

1. `cmd.Version` (build-time ldflags override)
2. `cmd/VERSION`
3. fallback `dev`

Example release build metadata injection:

```bash
VERSION=$(cat cmd/VERSION)
COMMIT=$(git rev-parse --short HEAD)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
go build -ldflags "-X github.com/prakersh/codexmultiauth/cmd.Version=${VERSION} -X github.com/prakersh/codexmultiauth/cmd.Commit=${COMMIT} -X github.com/prakersh/codexmultiauth/cmd.Date=${DATE}" -o cma .
```

## Security model at a glance

- Vault and backup encryption: `XChaCha20-Poly1305`
- Backup key derivation: `Argon2id`
- Strict filesystem permissions: files `0600`, dirs `0700`
- Mutations use lock + atomic write + verification + rollback
- Normal command output avoids secret values

Details: [docs/SECURITY.md](docs/SECURITY.md)

## Usage confidence tiers

`cma usage` and `cma limits` report:

- `confirmed`
- `best_effort`
- `unknown`

This prevents false precision when no stable machine-readable quota source is available.

## Auto selection

`cma auto` scores each saved account from the remaining 5-hour and weekly quota headroom, then increases the weight of quota that resets sooner. This lets a weekly bucket that resets tomorrow beat a small 5-hour advantage on another account when that is the better quota to burn next.

## Documentation map

- [docs/COMMANDS.md](docs/COMMANDS.md)
- [docs/SECURITY.md](docs/SECURITY.md)
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/BACKUP_RESTORE.md](docs/BACKUP_RESTORE.md)
- [docs/TESTING.md](docs/TESTING.md)
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)
- [docs/VERIFICATION.md](docs/VERIFICATION.md)

## Support

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/prakersh)
