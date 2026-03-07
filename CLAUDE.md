# CodexMultiAuth Rules

- Use Go with a layered architecture: `cmd`, `internal/domain`, `internal/app`, `internal/infra`, `internal/tui`.
- Keep credentials encrypted at rest and never log access tokens, refresh tokens, passphrases, decrypted payloads, or derived keys.
- Enforce `0600` permissions for files and `0700` permissions for directories created by CMA.
- Use lock acquisition, in-memory planning, verified atomic writes, and rollback for all mutating operations.
- Prefer mature libraries over custom implementations unless CMA-specific behavior requires custom code.
- Vault and backup encryption uses Argon2id + XChaCha20-Poly1305 with versioned metadata.
- Support keyring-backed vault keys with file-key fallback when keyring is disabled or unavailable.
- Keep CLI/TUI command behavior aligned with implemented selectors, conflict policies, and usage confidence tiers.
