# CodexMultiAuth (`cma`)

**The fastest way to switch Codex accounts when limits hit.**

Codex is effectively single-active-auth on a local machine. When one account hits its 5-hour usage window, many users end up in repeated logout/login cycles across accounts. `cma` removes that overhead with safe account switching, encrypted storage, and encrypted backup/restore.

Repository: https://github.com/prakersh/codexmultiauth

## Why `cma`

If you run multiple Codex accounts, the pain is usually not setup - it is repeated switching after a usage window is consumed. Manual logout/login loops and auth-file handling are slow, error-prone, and distracting.

`cma` solves that with:

- encrypted credential storage at rest
- atomic account activation with rollback
- encrypted backups and guided restore
- confidence-tiered usage reporting
- both CLI and TUI workflows

## Who This Is For

`cma` is built for users who need fast account rotation with less overhead:

- **power Codex users** juggling multiple accounts to avoid repeated auth friction
- **consultants and agencies** switching between client identities during tight delivery windows
- **teams with strict security posture** that want fewer manual auth-file mistakes

## Core Capabilities

- Save current Codex auth into an encrypted vault: `cma save`
- Switch active account safely: `cma activate <selector>`
- Create encrypted backups: `cma backup <encrypthash/pass> <name|abspath>`
- Restore selectively or all-at-once: `cma restore ... [--all]`
- View account usage with confidence labels: `cma usage <selector|all>`
- Run interactive terminal UI: `cma tui`
- Print release info and links: `cma version`

## Requirements

- Go `1.24.2`
- `codex` CLI on `PATH` (required for `cma new` login flow)
- Optional OS keyring support (CMA falls back to file key storage when needed)

## Quick Start

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
```

### 3) Backup and restore

```bash
# Encrypted backup (interactive passphrase prompt)
./cma backup prompt weekly-backup

# Restore with interactive selection
./cma restore prompt weekly-backup

# Restore all entries atomically with conflict policy
./cma restore prompt weekly-backup --all --conflict overwrite
```

### 4) Launch the TUI

```bash
./cma tui
```

## `app.sh` - Single Entrypoint for Contributors

Use `./app.sh` as the standard project workflow entrypoint for building, testing, verification, and release artifacts.

```bash
# Quick pre-commit checks
./app.sh --smoke

# Full verification matrix
./app.sh --verify

# Build local binary (./bin/cma)
./app.sh --build

# Run cma through orchestrator
./app.sh --run -- version
./app.sh --run -- tui

# Cross-platform release artifacts (./dist)
./app.sh --release
```

Execution order for combined flags:

`deps -> clean -> fmt -> lint -> test -> race -> cover -> build -> smoke -> verify -> release -> run`

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

Full syntax and examples: [docs/COMMANDS.md](docs/COMMANDS.md)

## Versioning

`cma version` prints the app version, repository URL, and support URL.

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

## Security Model at a Glance

- Vault and backup encryption: `XChaCha20-Poly1305`
- Backup key derivation: `Argon2id`
- Strict filesystem permissions: files `0600`, dirs `0700`
- Mutations use lock + atomic write + verification + rollback
- Secrets are not printed in normal command output

Details: [docs/SECURITY.md](docs/SECURITY.md)

## Usage Data Confidence

`cma usage` labels each result as:

- `confirmed`
- `best_effort`
- `unknown`

This prevents false precision when no stable machine-readable quota source is available. It is especially useful for users trying to decide when to rotate to another account without guessing.

## Documentation Map

- [docs/COMMANDS.md](docs/COMMANDS.md)
- [docs/SECURITY.md](docs/SECURITY.md)
- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)
- [docs/BACKUP_RESTORE.md](docs/BACKUP_RESTORE.md)
- [docs/TESTING.md](docs/TESTING.md)
- [docs/TROUBLESHOOTING.md](docs/TROUBLESHOOTING.md)
- [docs/VERIFICATION.md](docs/VERIFICATION.md)

## Support

[![Buy Me a Coffee](https://img.shields.io/badge/Buy%20Me%20a%20Coffee-FFDD00?style=for-the-badge&logo=buymeacoffee&logoColor=black)](https://buymeacoffee.com/prakersh)
