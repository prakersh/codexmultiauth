# Command Reference

## Project entrypoints

Use these scripts for local workflows:

- `./app.sh`: canonical build, test, verification, release, and publish orchestration
- `./test.sh`: thin wrapper for common verification and publish shortcuts

Examples:

```bash
./app.sh --smoke
./app.sh --verify
./app.sh --verify-sandbox
./app.sh --build
./app.sh --release
./app.sh --publish-release --draft --notes-file docs/release-notes.md
./app.sh --run -- version
```

`test.sh` mappings:

- `./test.sh quick` -> `./app.sh --smoke`
- `./test.sh full` -> `./app.sh --verify-sandbox`
- `./test.sh prerelease` -> `./app.sh --verify-sandbox --release`
- `./test.sh publish` -> `./app.sh --publish-release --draft`

## Selector resolution

When a command accepts `<selector>`, CMA resolves in this order:

1. exact `all` (only for commands that support multiple accounts)
2. 1-based list index (`1`, `2`, ...)
3. exact account ID
4. exact alias
5. exact display name
6. unique prefix of ID, alias, or display name

If there is no match, CMA returns selector not found. If a prefix matches multiple accounts, CMA returns ambiguous selector.

## Passphrase source syntax

`backup` and `restore` use this format for the passphrase argument:

- `prompt`: prompt for passphrase input
- `env:VAR`: read bytes from environment variable `VAR`
- `hash:<hex>`: decode bytes from a hex string
- `pass:<literal>`: use literal text directly (blocked unless `--allow-plain-pass-arg` is set)

## cma commands

### `cma list`

List saved accounts and show the active marker.

```bash
cma list
```

### `cma usage <selector|all>`

Fetch usage and print confidence labels, account details, and quota reset windows.

```bash
cma usage work
cma usage all
```

### `cma limits`

Show limits for all saved accounts with account details, confidence, and reset windows.

```bash
cma limits
```

### `cma save`

Save the current Codex auth into the encrypted vault.

Flags:

- `--name`: display name
- `--aliases`: comma-separated aliases

```bash
cma save
cma save --name work --aliases main,team
```

### `cma login [--device-auth|--with-api-key]`

Run Codex login and save the resulting account.

Flags:

- `--name`: display name
- `--aliases`: comma-separated aliases
- `--device-auth`: use device auth flow
- `--with-api-key`: read the API key from stdin through `codex login`

```bash
cma login
cma login --device-auth --name personal
printenv OPENAI_API_KEY | cma login --with-api-key --name api
```

### `cma new [--device-auth|--with-api-key]`

Compatibility alias for `cma login`.

### `cma activate <selector>`

Activate a saved account in the Codex auth store.

```bash
cma activate 1
cma activate work
```

### `cma delete <selector>`

Delete a saved account. If the account is active, CMA asks for confirmation.

```bash
cma delete personal
```

### `cma rename <selector> <new-name>`

Rename a saved account.

```bash
cma rename personal work
```

### `cma backup <encrypthash/pass> <name|abspath>`

Write an encrypted backup artifact.

Flags:

- `--allow-plain-pass-arg`: allow `pass:<literal>`

```bash
cma backup prompt nightly
cma backup env:CMA_PASS /absolute/path/snap.cma.bak
cma backup hash:736563726574 nightly
```

### `cma restore <encrypthash/pass> <pathtobackup|name>`

Restore accounts from an encrypted backup.

Flags:

- `--all`: restore all candidates atomically
- `--conflict ask|overwrite|skip|rename`: conflict policy (default `ask`)
- `--allow-plain-pass-arg`: allow `pass:<literal>`

Without `--all`, CMA prompts for account selection.

```bash
cma restore prompt nightly
cma restore env:CMA_PASS nightly --all --conflict overwrite
cma restore hash:736563726574 /abs/path/snap.cma.bak --conflict rename
```

### `cma version [--short]`

Print version information.

Flags:

- `--short`: print version only

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

Version resolution order:

1. `cmd.Version` from build-time ldflags
2. embedded `cmd/VERSION`
3. fallback `dev`

### `cma tui`

Launch the interactive terminal UI.

```bash
cma tui
```
