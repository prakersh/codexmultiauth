# Backup and Restore

## Backup format

Backup artifacts are versioned JSON files with encrypted account payloads.

- backup version: `cma-backup-v1`
- envelope version: `cma-envelope-v1`
- KDF: `Argon2id`
- AEAD: `XChaCha20-Poly1305`

Each backup stores:

- manifest metadata (`version`, `created_at`, `account_ids`)
- encrypted account records (account metadata + auth payload bytes)

## Create backups

```bash
cma backup prompt nightly
cma backup env:CMA_PASS nightly
cma backup hash:736563726574 /abs/path/nightly.cma.bak
```

Target rules:

- absolute path: writes exactly to that path
- name only: writes under `${XDG_CONFIG_HOME:-~/.config}/cma/backups/`

## Restore modes

### Interactive subset restore (default)

When `--all` is not set, restore flow is:

1. decrypt backup
2. inspect candidates
3. prompt for selected accounts
4. apply restore to selected subset

### Atomic restore-all (`--all`)

`cma restore ... --all` restores all candidates in one mutation path.

## Conflict policies

If imported accounts conflict with existing entries:

- `ask`: require explicit decision per conflict
- `overwrite`: replace existing metadata and payload
- `skip`: keep existing account and ignore incoming record
- `rename`: import incoming account with generated display name suffix

Conflict detection order:

1. fingerprint
2. account ID
3. display name
4. alias overlap

## Restore command examples

```bash
cma restore prompt nightly
cma restore env:CMA_PASS nightly --all --conflict overwrite
cma restore hash:736563726574 /abs/path/nightly.cma.bak --conflict rename
```

## TUI restore flow

`cma tui` restore includes:

1. source and passphrase input
2. backup inspection
3. subset vs all toggle
4. conflict policy selection
5. per-conflict decisions for `ask`
6. restore execution through app service

## Safety guarantees

Backup and restore mutations use lock acquisition, atomic writes, verification, and rollback. Filesystem permission policy remains `0600` for files and `0700` for directories.