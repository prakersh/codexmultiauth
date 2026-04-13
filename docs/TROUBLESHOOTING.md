# Troubleshooting

## `cma login` or `cma new` fails with `codex CLI runner is not configured`

Cause:

- `codex` CLI is unavailable from the CMA runtime environment

Fix:

- install `codex` and ensure it is on `PATH`
- run `codex --help` to confirm availability

## `load current codex auth` or auth not found

Cause:

- no valid auth exists at `${CODEX_HOME:-~/.codex}/auth.json`

Fix:

- run `codex login`
- verify `CODEX_HOME` points to the expected profile

## Keyring issues

Cause:

- OS keyring is unavailable or blocked

Fix:

- set `CMA_DISABLE_KEYRING=1` to force file-key mode
- or set `disable_keyring: true` in `${XDG_CONFIG_HOME:-~/.config}/cma/config.json`

## Backup or restore passphrase errors

Cause:

- wrong passphrase
- wrong passphrase source syntax
- malformed hash input

Fix:

- use `prompt` for manual entry
- verify `env:VAR` is set and non-empty
- verify `hash:<hex>` is valid hex
- use `pass:<literal>` only with `--allow-plain-pass-arg`

## Selector is ambiguous or not found

Cause:

- selector prefix matches multiple accounts
- selector does not match any account

Fix:

- use exact selector (index, full ID, exact alias, or exact display name)
- run `cma list` and choose a unique value

## Restore conflict errors with `ask`

Cause:

- non-interactive execution reached unresolved `ask` conflicts

Fix:

- run restore in interactive CLI/TUI mode
- or choose explicit `--conflict overwrite|skip|rename`

## Race warnings from macOS linker

Cause:

- toolchain-level linker warnings can appear during `go test -race`

Fix:

- if test output still reports `ok`, treat warnings as non-fatal
- keep Go toolchain and Xcode command line tools updated
