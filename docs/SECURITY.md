# Security Model

## Security goals

CMA is designed to:

- keep account credentials encrypted at rest
- avoid secret leakage in normal output
- apply strict filesystem permissions
- make every mutation crash-safe with rollback

## Encryption primitives

### Vault encryption

- AEAD: `XChaCha20-Poly1305`
- envelope version: `cma-envelope-v2` (reads `cma-envelope-v1`)
- key size: 32 bytes

### Envelope metadata binding

From `cma-envelope-v2`, the fields that travel in plaintext beside the
ciphertext are bound into the AEAD as additional authenticated data: envelope
version, creation time, AEAD name and nonce, the KDF block, and the metadata
map. For a vault entry that map carries the account ID, source, and
fingerprint.

Editing any of those, for example relabelling an entry to another account or
rewriting its fingerprint, now fails to decrypt. Under `cma-envelope-v1` the
ciphertext was authenticated but everything around it was freely rewritable.

`cma-envelope-v1` files are still read, sealed as they were with no additional
data, so existing vaults and backups keep working. Any write re-seals the
entry as v2, so a vault migrates the first time it is modified.

Migration is one way. Once entries have been written as v2, a CMA older than
the version that introduced it cannot decrypt them and reports `unsupported
envelope version`. Downgrading therefore needs a backup taken with the older
version, restored into a fresh vault by that older version.

### Key material lifetime

Derived keys, vault keys, and passphrase buffers are zeroed once the operation
that needed them completes, and the passphrase prompt reads into a byte slice
rather than a string so the bytes can be wiped at all.

This is best effort, not a guarantee. Go's garbage collector may copy or move a
buffer before it is zeroed, so a core dump or a `/proc/<pid>/mem` read may still
catch key material.

### Backup encryption

- KDF: `Argon2id`
- AEAD: `XChaCha20-Poly1305`
- backup format version: `cma-backup-v1`

## Key management

Vault key source order:

1. OS keyring (when available and not disabled)
2. local fallback key file (`vault.key.v1`) with strict permissions

## Filesystem permissions

CMA enforces:

- directories: `0700`
- files: `0600`

This includes state, vault, key, lock, and backup targets.

## Mutation safety

Mutating operations use:

1. lock acquisition (`gofrs/flock`)
2. in-memory mutation planning
3. atomic write (temp file, `fsync`, rename)
4. post-write verification
5. rollback on failure

State and vault are written in the order that makes a crash between them
survivable: vault first when adding an entry, state first when removing one, so
the residue is always an inert vault entry rather than a state pointer to a row
that no longer exists.

## Concurrency

Reads take a shared lock and mutations an exclusive one, so a reader can never
observe `state.json` and the vault from opposite sides of a commit.

Token refresh additionally takes a per-account lock held across the whole
read-refresh-persist sequence. A refresh token is single-use, so this stops two
processes from spending the same one, which would otherwise either fail a
healthy account or persist a superseded token. The lock is per-account, so
different accounts still refresh in parallel.

State and vault writes are validated after commit. Activation writes are also validated by auth fingerprint.

## Secret handling rules

Normal command output avoids printing:

- access tokens
- refresh tokens
- ID tokens
- passphrases
- decrypted auth payloads
- derived keys

Tests include leak checks for command output and plaintext scans for vault/backup artifacts.

## Auth store behavior

- primary auth store: `${CODEX_HOME:-~/.codex}/auth.json`
- optional keyring-backed auth path when configured and available
- activation verifies post-write fingerprint and restores prior auth on mismatch

## Token refresh handling

Usage checks may refresh tokens through OAuth. Refreshed values are persisted through the same lock and atomic commit path used by other mutations. Refresh failures do not print token material and do not block best-effort usage fallback.
