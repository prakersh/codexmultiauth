# CodexMultiAuth Rules

- Use Go with a layered architecture: `cmd`, `internal/domain`, `internal/app`, `internal/infra`, `internal/tui`.
- Keep credentials encrypted at rest and never log access tokens, refresh tokens, passphrases, decrypted payloads, or derived keys.
- Enforce `0600` permissions for files and `0700` permissions for directories created by CMA.
- Use lock acquisition, in-memory planning, verified atomic writes, and rollback for all mutating operations.
- Prefer mature libraries over custom implementations unless CMA-specific behavior requires custom code.
