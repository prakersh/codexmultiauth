# Architecture

`cma` uses a layered design.

## Package Layout

- `cmd/`
  - Cobra CLI command wiring.
  - Prompt helpers and command argument handling.
- `internal/domain/`
  - Core domain models and selector resolution.
- `internal/app/`
  - Business services (`save`, `new`, `activate`, `delete`, `backup`, `restore`, `usage`).
  - Mutation orchestration with lock + commit + verification.
- `internal/infra/`
  - Filesystem safety (`fs`), path resolution (`paths`), repos (`store`), crypto (`crypto`), backup format (`backup`), usage client (`usage`), Codex CLI runner (`codexcli`).
- `internal/tui/`
  - Bubble Tea terminal UI on top of app services.

## Runtime Data

Resolved from `internal/infra/paths`:

- CMA config root: `${XDG_CONFIG_HOME:-~/.config}/cma`
- config: `config.json`
- account state: `state.json`
- encrypted vault: `vault.v1.json`
- fallback vault key file: `vault.key.v1`
- backups: `backups/*.cma.bak`
- locks: `locks/*.lock`
- Codex home: `${CODEX_HOME:-~/.codex}`
- Codex auth file: `${CODEX_HOME:-~/.codex}/auth.json`

## Key Flows

### Save

1. Load current Codex auth.
2. Normalize and fingerprint.
3. Deduplicate by fingerprint.
4. Append account metadata to state and payload to vault.
5. Commit atomically with verification.

### Activate

1. Resolve selector to account.
2. Load vault payload.
3. Save payload to Codex auth store.
4. Re-load and verify fingerprint.
5. Update active account in state.
6. Roll back auth on failure.

### Backup

1. Read state + vault.
2. Build backup account list from matching vault entries.
3. Encrypt plaintext backup with passphrase.
4. Atomic write backup artifact.

### Restore

1. Decrypt backup.
2. Analyze candidates and conflicts.
3. Apply selection (`--all` or selected subset).
4. Apply conflict policy (`ask|overwrite|skip|rename`).
5. Commit state + vault atomically.

### Usage

1. Resolve selector(s).
2. Parse auth payload from vault.
3. Try authenticated usage HTTP fetch.
4. Fall back to best-effort JWT plan parsing.
5. Return `confirmed`, `best_effort`, or `unknown`.
