# Backup and Restore

## Backup Format

Backup artifacts are JSON files with versioned structure.

- file version: `cma-backup-v1`
- envelope version: `cma-envelope-v1`
- KDF: `Argon2id`
- AEAD: `XChaCha20-Poly1305`

File stores:

- backup manifest (`version`, `created_at`, `account_ids`)
- encrypted account records (`account` metadata + auth payload bytes)

## Creating Backups

```bash
cma backup prompt nightly
cma backup env:CMA_PASS nightly
cma backup hash:736563726574 /abs/path/nightly.cma.bak
```

Relative names are saved under `~/.config/cma/backups/` (or `$XDG_CONFIG_HOME/cma/backups/`).

## Restore Modes

### Interactive Selection (default)

`cma restore ...` without `--all`:

1. decrypts backup
2. inspects account candidates
3. prompts for selected accounts
4. applies restore for chosen subset

### Atomic All (`--all`)

`cma restore ... --all` restores all candidates in one mutation path.

## Conflict Policies

When imported candidates conflict with existing accounts:

- `ask`  
  Requires explicit decision per conflict.
- `overwrite`  
  Replaces existing account metadata/payload with incoming data.
- `skip`  
  Keeps existing account and ignores incoming conflict record.
- `rename`  
  Imports incoming account with a generated display name suffix.

Conflict detection order:

1. fingerprint
2. account id
3. display name
4. alias overlap

## TUI Restore

`cma tui` restore flow includes:

1. source + passphrase input
2. backup inspection
3. selective vs all toggle
4. conflict policy selection
5. per-conflict decisions when policy is `ask`
6. restore execution through app service
