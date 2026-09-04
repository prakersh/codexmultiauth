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

`activate`, `delete`, and `rename` act on exactly one account and reject `all`
outright, even when only one account is saved, so `cma delete all` can never
silently remove it.

## Passphrase source syntax

`backup` and `restore` use this format for the passphrase argument:

- `prompt`: prompt for passphrase input
- `env:VAR`: read bytes from environment variable `VAR`
- `hash:<hex>`: decode bytes from a hex string (blocked unless `--allow-plain-pass-arg` is set)
- `pass:<literal>`: use literal text directly (blocked unless `--allow-plain-pass-arg` is set)
- `<literal>`: bare literal text, also blocked unless `--allow-plain-pass-arg` is set

`prompt` and `env:VAR` are the only forms that keep the passphrase out of the
process argument list. `hash:` is hex encoding, not hashing: it decodes to the
passphrase itself, so it is as exposed as `pass:` to anything that can read
`ps` output or shell history, and it is gated the same way.

A passphrase containing `:` cannot be given as a bare literal, because the
leading text would be read as a source name. Pass it as `pass:<literal>` or
through `env:VAR`.

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

Show limits for all saved accounts with account details, data provenance, and
reset windows.

Quota columns are discovered from the windows each account reports, so a vault
mixing free and paid plans shows a monthly pair and a weekly pair, with `-`
where an account has no window of that kind.

The `DATA` column says where the numbers came from, so an all-`-` row is never
ambiguous:

- `live`: fetched from the usage API this run
- `cached`: derived from the stored token, no API response
- `none`: no usage data at all
- `limited`: the API reports this account is currently rate limited

```bash
cma limits
cma limits --dull   # no colors, for piping to a file or another program
```

### `cma refresh`

Force a token-authority refresh for the selected accounts and persist the new
access and refresh tokens atomically. The selector argument is mandatory; pass
`all` to refresh every saved account, or a display name, alias, index, or ID to
refresh one.

Most commands refresh expiring tokens on their own, so this is for forcing a
rotation early or repairing an account whose tokens look stale.

```bash
cma refresh all
cma refresh work
```

An account whose refresh token has been revoked or has expired reports a
`status 401` failure. That account has to be logged in again with `cma login`
and re-saved; CMA cannot recover it.

### `cma doctor`

Verify that `state.json` and the vault agree, report any entry whose stored
fingerprint does not match its payload, and clear a torn-state marker left by a
failed rollback.

Mutating commands refuse to run while a torn-state marker is present, so this
is the command that clears the way after an interrupted write.

```bash
cma doctor
```

### `cma auto`

Choose and activate the best saved account automatically.

Selection uses an urgency-weighted quota score:

- more remaining quota on any reported window helps
- quota that resets sooner gets extra weight, scaled to that window's length
- ties fall back to raw remaining quota on the longest window, then earlier resets

Codex reports a monthly window on free plans and a weekly window on paid plans;
the 5-hour window is no longer issued. CMA reads the window length from the API,
so accounts on different plans are scored on whatever windows they actually have.

```bash
cma auto
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

Before overwriting the auth store, `activate` saves the credentials currently
in it back to the account that owns them. Codex rotates its refresh token as it
runs and invalidates the one it replaces, so without this the account being
switched away from would be left holding a spent token and coming back to it
would need a browser login.

The capture only happens when the live credentials clearly belong to the active
account, matched on the Codex account id. If a different account was logged in
manually, or the auth store holds an API key with no account id, the capture is
skipped rather than risk storing one account's tokens under another's name.

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

### `cma backup <passphrase-source> <name|abspath>`

Write an encrypted backup artifact.

Flags:

- `--allow-plain-pass-arg`: allow `hash:<hex>`, `pass:<literal>`, and bare literal arguments, all of which place the passphrase in argv

```bash
cma backup prompt nightly
cma backup env:CMA_PASS /absolute/path/snap.cma.bak
cma backup hash:736563726574 nightly --allow-plain-pass-arg
```

### `cma restore <passphrase-source> <pathtobackup|name>`

Restore accounts from an encrypted backup.

Flags:

- `--all`: restore all candidates atomically
- `--conflict ask|overwrite|skip|rename`: conflict policy (default `ask`)
- `--allow-plain-pass-arg`: allow `hash:<hex>`, `pass:<literal>`, and bare literal arguments, all of which place the passphrase in argv

Without `--all`, CMA prompts for account selection.

```bash
cma restore prompt nightly
cma restore env:CMA_PASS nightly --all --conflict overwrite
cma restore hash:736563726574 /abs/path/snap.cma.bak --conflict rename --allow-plain-pass-arg
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
