# Security Model

## Encryption at Rest

### Vault

- Vault entries are encrypted with `XChaCha20-Poly1305`.
- Envelope version: `cma-envelope-v1`.
- Vault key length: 32 bytes.
- Vault key source:
  - OS keyring when enabled and available.
  - Local key file fallback when keyring is disabled or unavailable.

### Backups

- Backup plaintext is encrypted with passphrase-derived key.
- KDF: `Argon2id`.
- AEAD: `XChaCha20-Poly1305`.
- Backup file version: `cma-backup-v1`.

## Lock, Atomic Write, and Rollback

Mutating operations use:

1. lock acquisition (`gofrs/flock`)
2. in-memory planning
3. atomic temp-write + `fsync` + rename
4. post-write verification when configured
5. rollback on failure

State and vault commits are verified by reloading and comparing expected state/vault shape.

## Permissions Model

- Directories are created and enforced as `0700`.
- Files are created and enforced as `0600`.
- Permission enforcement is applied to lock files and atomic-write targets.

## Secret Handling

CMA avoids printing sensitive values in normal command output:

- access tokens
- refresh tokens
- passphrases
- decrypted auth payloads
- derived keys

Integration tests include plaintext leak checks for vault and backup artifacts.

## Auth Store Behavior

- Primary auth store is Codex file auth (`$CODEX_HOME/auth.json`, default `~/.codex/auth.json`).
- Keyring auth is also supported when configured/available.
- Activation verifies written auth by fingerprint and rolls back on mismatch.
