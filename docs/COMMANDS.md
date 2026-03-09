# Command Reference

## Project Entrypoint

Use `./app.sh` for local build, test, verification, and release workflows.  
Use `cma` commands for account operations.

Common orchestration examples:

```bash
./app.sh --smoke
./app.sh --verify
./app.sh --verify-sandbox
./app.sh --build
./app.sh --release
./app.sh --publish-release --draft --notes-file docs/release-notes.md
./app.sh --run -- version
```

Thin wrapper commands:

```bash
./test.sh quick
./test.sh full
./test.sh prerelease
./test.sh publish -- --notes-file docs/release-notes.md
```

Release-related `app.sh` flags:

- `--verify-sandbox` run the full verify matrix inside a temp HOME/XDG/CODEX sandbox
- `--release` build `dist/*` artifacts and `dist/sha256sums.txt`
- `--publish-release` verify in sandbox, build release artifacts, then create a GitHub release
- `--tag <tag>` override the default tag `v<cmd/VERSION>`
- `--draft` create the GitHub release as a draft
- `--notes-file <path>` supply release notes for GitHub release creation

Recommended draft-first flow:

```bash
./app.sh --verify-sandbox
./app.sh --release
./app.sh --publish-release --draft --notes-file docs/release-notes.md
gh release edit v$(cat cmd/VERSION) --draft=false
```

## Root

```bash
cma --help
```

## Selectors

Selectors are resolved in this order:

1. Exact `all` (only for commands that accept multiple accounts, such as `usage`).
2. 1-based list index (`1`, `2`, ...).
3. Exact account ID.
4. Exact alias.
5. Exact display name.
6. Unique prefix of ID, alias, or display name.

If no match exists, commands return a selector-not-found error. If multiple prefix matches exist, commands return an ambiguous-selector error.

## Passphrase Source Syntax

`backup` and `restore` use this source format:

- `prompt`  
  Prompts for a passphrase.
- `env:VAR`  
  Reads passphrase bytes from environment variable `VAR`.
- `hash:<hex>`  
  Decodes raw bytes from a hex string.
- `pass:<literal>`  
  Uses literal text directly. This is blocked unless `--allow-plain-pass-arg` is set.

## Commands

### `cma list`

List saved accounts, aliases, and active marker.

```bash
cma list
```

### `cma usage <selector|all>`

Fetch usage and print confidence tier.

```bash
cma usage all
cma usage work
```

### `cma version [--short]`

Show version and public project links.

Flags:

- `--short` print version string only

```bash
cma version
cma version --short
```

Default output:

```text
cma version: <version>
repository: https://github.com/prakersh/codexmultiauth
support: https://buymeacoffee.com/prakersh
```

Version source behavior:

- default value comes from `cmd/VERSION`
- build can override with ldflags (`cmd.Version`, `cmd.Commit`, `cmd.Date`)

### `cma save`

Save current Codex auth into encrypted vault.

Flags:

- `--name` display name
- `--aliases` comma-separated aliases

```bash
cma save
cma save --name work --aliases main,team
```

### `cma new [--device-auth]`

Run Codex login and save resulting auth.

Flags:

- `--name` display name
- `--aliases` comma-separated aliases
- `--device-auth` use device auth flow

```bash
cma new
cma new --device-auth --name personal
```

### `cma activate <selector>`

Activate selected saved account in Codex auth store.

```bash
cma activate 1
cma activate work
```

### `cma delete <selector>`

Delete a saved account. If it is active, CLI asks for confirmation.

```bash
cma delete personal
```

### `cma backup <encrypthash/pass> <name|abspath>`

Write encrypted backup artifact.

Flags:

- `--allow-plain-pass-arg` allows `pass:<literal>`

```bash
cma backup prompt nightly
cma backup env:CMA_PASS /absolute/path/snap.cma.bak
cma backup hash:736563726574 nightly
```

### `cma restore <encrypthash/pass> <pathtobackup|name>`

Restore accounts from encrypted backup.

Flags:

- `--all` restore all candidates atomically
- `--conflict ask|overwrite|skip|rename` conflict policy (default `ask`)
- `--allow-plain-pass-arg` allows `pass:<literal>`

Without `--all`, CLI prompts for account selection from backup candidates.

```bash
cma restore prompt nightly
cma restore env:CMA_PASS nightly --all --conflict overwrite
cma restore hash:736563726574 /abs/path/snap.cma.bak --conflict rename
```

### `cma tui`

Launch interactive terminal UI.

```bash
cma tui
```
