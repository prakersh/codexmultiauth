# Troubleshooting

## `cma new` fails with `codex CLI runner is not configured`

Cause:

- `codex` CLI is not available from runtime wiring.

Fix:

- Ensure `codex` is installed and on `PATH`.
- Run `codex --help` to validate.

## `load current codex auth` or auth not found

Cause:

- No valid auth in `${CODEX_HOME:-~/.codex}/auth.json`.

Fix:

- Run `codex login` first.
- Confirm `CODEX_HOME` points to expected profile directory.

## Keyring issues

Cause:

- OS keyring unavailable or blocked in environment.

Fix:

- Set `CMA_DISABLE_KEYRING=1` to force file-key mode.
- Or set `disable_keyring: true` in `~/.config/cma/config.json`.

## Backup or restore passphrase errors

Cause:

- Wrong passphrase, wrong source syntax, or malformed hash.

Fix:

- Use `prompt` for manual input.
- Verify `env:VAR` exists and is non-empty.
- Verify `hash:<hex>` is valid hex bytes.
- Use `pass:<literal>` only with `--allow-plain-pass-arg`.

## Selector is ambiguous or not found

Cause:

- Prefix matches multiple accounts, or no account matches selector.

Fix:

- Use exact selector (`index`, full `id`, exact `alias`, or exact display name).
- Run `cma list` and choose a unique selector.

## Restore conflict errors with `ask`

Cause:

- Non-interactive path reached with unresolved `ask` conflicts.

Fix:

- Use CLI interactive prompts, TUI restore flow, or choose explicit `--conflict` policy.

## Race test warnings on macOS linker

Cause:

- Toolchain linker warnings (for example `LC_DYSYMTAB`) may appear during `-race`.

Fix:

- If tests still report `ok`, treat as non-fatal warning.
- Keep Xcode command line tools and Go toolchain updated.
