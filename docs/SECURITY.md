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
- envelope version: `cma-envelope-v1`
- key size: 32 bytes

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
