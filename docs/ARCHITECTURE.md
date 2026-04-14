# Architecture

CodexMultiAuth (`cma`) uses a layered Go architecture.

## Package layout

- `cmd/`
  - Cobra command wiring
  - Prompt handling and CLI output
- `internal/domain/`
  - core models
  - selector and conflict policy logic
- `internal/app/`
  - application services (`save`, `new`, `activate`, `delete`, `backup`, `restore`, `usage`)
  - lock + commit + rollback orchestration for mutations
- `internal/infra/`
  - filesystem and locking helpers (`fs`)
  - runtime path resolution (`paths`)
  - repositories and auth storage (`store`)
  - encryption and backup format (`crypto`, `backup`)
  - Codex CLI adapter (`codexcli`)
  - usage API and token refresh clients (`usage`)
- `internal/tui/`
  - Bubble Tea UI over app services

## Runtime data layout

Resolved by `internal/infra/paths`:

- config root: `${XDG_CONFIG_HOME:-~/.config}/cma`
- config file: `config.json`
- account state: `state.json`
- encrypted vault: `vault.v1.json`
- fallback vault key: `vault.key.v1`
- backups: `backups/*.cma.bak`
- lock files: `locks/*.lock`
- Codex home: `${CODEX_HOME:-~/.codex}`
- Codex auth file: `${CODEX_HOME:-~/.codex}/auth.json`

## Core mutation model

All mutating flows use the same safety model:

1. acquire lock
2. load and validate current state
3. build in-memory mutation plan
4. write atomically (temp file + fsync + rename)
5. verify committed shape
6. roll back on failure

## Key service flows

### Save

1. load current Codex auth
2. normalize and fingerprint payload
3. deduplicate by fingerprint
4. append account metadata to state
5. append encrypted payload to vault
6. commit atomically

### Activate

1. resolve selector to account
2. load payload from vault
3. write payload to active Codex auth store
4. reload and verify fingerprint
5. set active account in state
6. roll back auth file/keyring entry if activation verification fails

### Backup

1. load state and vault
2. match state accounts to vault entries
3. build versioned plaintext backup structure
4. encrypt with passphrase-derived key
5. write backup artifact atomically

### Restore

1. decrypt and parse backup artifact
2. detect conflicts by fingerprint, account ID, display name, and alias overlap
3. select candidate set (`--all` or interactive subset)
4. apply conflict policy (`ask|overwrite|skip|rename`)
5. commit state and vault atomically

### Usage and limits

1. resolve one or many accounts
2. load auth payload from vault
3. attempt proactive token refresh when token is near expiry
4. persist refreshed auth payload and fingerprint to vault/state
5. update active Codex auth store if the active account was refreshed
6. fetch usage from authenticated API
7. fall back to JWT-based best effort summary when API fetch fails
8. return confidence tier (`confirmed`, `best_effort`, `unknown`)

### Auto activation

1. fetch usage for all saved accounts
2. identify 5-hour and weekly quota entries
3. score each account from remaining quota and reset urgency
4. prefer the highest combined score, then higher raw headroom, then earlier resets
5. activate the winning account through the normal activation flow
