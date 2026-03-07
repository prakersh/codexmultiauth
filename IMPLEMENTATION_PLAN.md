# CodexMultiAuth (CMA) - Comprehensive Implementation Plan (v2)

> Planning-only document. **Do not implement in this phase.**
> This document is for the executor agent to implement.

## 0) Decision updates from product/architecture review

1. **Yes - `cma activate` is mandatory.**
   - It is the safe, explicit command that writes the selected account auth into `~/.codex/auth.json` so Codex uses it.
2. **Library-first policy is mandatory.**
   - Use mature Go libraries where they fit.
   - Only write custom code for CMA-specific logic (selector resolution, conflict planner, Codex auth validation, restore policy engine, etc.).
3. **No security/feature compromises allowed.**
   - If a library cannot satisfy required guarantees, use a thin custom wrapper around a stable primitive.
4. **Codex behavior references must come from:**
   - https://github.com/openai/codex
   - https://developers.openai.com/codex/auth

---

## 1) Product goals and non-goals

## Goals
- Manage multiple Codex accounts safely on macOS/Linux.
- Keep credentials encrypted at rest at all times.
- Provide safe account switching (`cma activate`).
- Provide encrypted backup/restore with selective or all import.
- Provide usage visibility with confidence labels.
- Provide both CLI and TUI workflows.

## Non-goals (v1)
- No remote API dependency for account management.
- No fake precision for quotas where no stable machine-readable source exists.
- No plaintext credential persistence outside tightly controlled transient memory.

---

## 2) Command contract (v2)

## Required commands
- `cma list`
- `cma usage <selector|all>`
- `cma save`
- `cma new [--device-auth]`
- `cma activate <selector>` **(new mandatory command)**
- `cma delete <selector>`
- `cma backup <encrypthash/pass> <name|abspath>`
- `cma restore <encrypthash/pass> <pathtobackup|name> [--all] [--conflict ask|overwrite|skip|rename]`
- `cma tui`

## Behavioral contract highlights
- `save`: encrypt and dedupe by fingerprint.
- `new`: optional pre-save of current account, login flow, rollback on failure.
- `activate`: atomic write to `~/.codex/auth.json` with rollback and verification.
- `backup`: always encrypted output.
- `restore`: decrypt + analyze + interactive or `--all` import.

---

## 3) Mature library strategy (do not reinvent wheel)

## Core dependencies (recommended)
- CLI: `github.com/spf13/cobra`
- TUI: `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/bubbles`, `github.com/charmbracelet/lipgloss`
- File locking: `github.com/gofrs/flock`
- KDF + AEAD primitives: `golang.org/x/crypto/argon2`, `golang.org/x/crypto/chacha20poly1305`
- Interactive prompts (non-TUI): `github.com/AlecAivazis/survey/v2`
- IDs: `github.com/google/uuid`
- Tests/assertions: `github.com/stretchr/testify`, `github.com/google/go-cmp/cmp`

## Atomic write strategy
- Prefer a mature atomic-write helper (e.g. `github.com/google/renameio/v2`) **if** it supports required semantics.
- If not sufficient for strict CMA durability/permission policy, implement a small, auditable wrapper:
  - temp file in same dir, `0600`
  - write + `fsync(file)`
  - `rename` atomically
  - `fsync(parent dir)`

## Principle
- Library-first by default.
- Custom code only for CMA-specific behavior or security guarantees unavailable in mature libs.

---

## 4) Security architecture

## Encryption at rest (mandatory)
- Canonical credential store is encrypted vault (`vault.v1.json`), never plaintext.
- KDF: Argon2id
  - salt: 16 random bytes
  - key length: 32 bytes
  - baseline: m=64MiB, t=3, p=min(4, CPU)
- AEAD: XChaCha20-Poly1305
  - nonce: 24 random bytes per encryption

## Filesystem controls
- Directories: `0700`
- Files: `0600`
- Strict no-secrets logging policy.

## Passphrase source contract
`<encrypthash/pass>` supports:
- `prompt` (recommended)
- `env:VAR_NAME`
- `hash:<hex>` (advanced)
- `pass:<literal>` only with `--allow-plain-pass-arg` (explicit unsafe opt-in)

## Secret safety
- Never print/pass through:
  - access tokens
  - refresh tokens
  - decrypted auth payloads
  - derived keys
  - passphrases

---

## 5) Data model (high level)

## Metadata state file (plaintext-safe metadata only)
Path: `~/.config/cma/state.json`
- accounts[]
  - id (uuid)
  - display_name
  - aliases[]
  - fingerprint
  - created_at
  - last_used_at
- active_account_id (optional)
- usage cache summary (non-secret)

## Encrypted vault file
Path: `~/.config/cma/vault.v1.json`
- version
- kdf params
- vault-wide metadata
- entries[]
  - account_id
  - fingerprint
  - encrypted payload blob
  - payload metadata (created_at, source)

## Backup artifact
Default path for name input: `~/.config/cma/backups/<name>.cma.bak`
- versioned envelope
- KDF metadata + salt
- AEAD metadata + nonce(s)
- encrypted manifest
- encrypted entry payloads

---

## 6) Selector and conflict resolution model

## Selector resolution order
1. exact index (1-based)
2. exact id
3. exact alias
4. exact display name
5. unique prefix
6. `all` keyword (where supported)

## Conflict precedence (restore/import)
1. fingerprint
2. account id
3. alias/name

## Conflict actions
- `ask` (interactive default)
- `overwrite`
- `skip`
- `rename` (deterministic suffix strategy)

---

## 7) Required command behavior details

## `cma save`
- Read current `~/.codex/auth.json`.
- Validate minimal schema expected by Codex.
- Fingerprint credential payload.
- If existing fingerprint already saved -> no duplicate.
- Else encrypt and store payload in vault.
- Update metadata state.

## `cma new [--device-auth]`
- Optionally auto-save current auth first.
- Execute login:
  - default: `codex login`
  - device auth: `codex login --device-auth`
- Validate resulting auth file.
- Encrypt-save new credentials.
- Rollback previous auth on failed/canceled flow.

## `cma activate <selector>` (new mandatory)
- Resolve selected account from state.
- Decrypt selected vault entry.
- Validate auth JSON payload before write.
- Lock + atomic write to `~/.codex/auth.json`.
- Verify written file fingerprint matches selected account.
- Update active metadata marker.
- Optional best-effort `codex login status` check for UX messaging.

## `cma list`
- Show accounts + aliases.
- Active marker by fingerprint match against current auth.json and/or active metadata.

## `cma delete <selector>`
- Resolve account.
- Confirm deletion if currently active.
- Remove encrypted entry + metadata.
- Keep rollback safety for state mutations.

## `cma usage <selector|all>`
- Return usage values with confidence:
  - `confirmed`
  - `best_effort`
  - `unknown`
- Never claim precise quota without confirmed source.

## `cma backup <encrypthash/pass> <name|abspath>`
- Validate passphrase source.
- Resolve output path:
  - absolute path -> exact target
  - name -> backups dir + `.cma.bak`
- Export selected/available encrypted credentials + metadata into encrypted backup envelope.
- Strictly fail if encryption source invalid.

## `cma restore <encrypthash/pass> <pathtobackup|name> [--all]`
- Resolve backup path.
- Decrypt and validate envelope.
- Analyze contained accounts and display summary.
- Default interactive: per-item import decisions.
- `--all`: pre-validate all, then single atomic commit.
- Apply explicit conflict policy.
- Emit import summary (counts, conflicts handled, skipped, renamed).

---

## 8) TUI scope (`cma tui`)

## Screens
- Accounts list + active indicator.
- Account details pane.
- Usage pane with confidence labels.
- Backup/restore workflow pane.

## TUI actions
- Save current account
- Activate selected account
- Delete selected account
- Trigger backup flow
- Trigger restore flow (interactive decisions)

## UX constraints
- No secret values rendered.
- Clear conflict and policy messaging.
- Strong keyboard navigation hints.

---

## 9) Repository structure (updated)

```text
/Users/prakersh/projects/codexmultiauth/
  go.mod
  go.sum
  main.go

  cmd/
    root.go
    list.go
    usage.go
    save.go
    new.go
    activate.go
    delete.go
    backup.go
    restore.go
    tui.go

  internal/
    domain/
      account.go
      usage.go
      backup.go
      selector.go
      errors.go

    app/
      list_service.go
      usage_service.go
      save_service.go
      new_service.go
      activate_service.go
      delete_service.go
      backup_service.go
      restore_service.go

    infra/
      paths/
        codex_paths.go
      fs/
        atomic_write.go
        lock.go
        perms.go
      codexcli/
        client.go
        login.go
        status.go
      store/
        state_repo.go
        vault_repo.go
      crypto/
        argon2id.go
        envelope.go
      backup/
        format_v1.go
        reader.go
        writer.go
      usage/
        status_parser.go
        session_parser.go

    tui/
      model.go
      view.go
      update.go
      styles.go
      screens/
        accounts.go
        usage.go
        backup_restore.go

  test/
    integration/
      save_new_delete_test.go
      activate_atomicity_test.go
      backup_restore_test.go
      usage_confidence_test.go

  CLAUDE.md
  AGENTS.md -> CLAUDE.md
```

---

## 10) Delivery phases for executor

1. **Bootstrap**
   - module init, command tree, package skeleton
2. **Infra primitives**
   - path resolver, perms, lock, atomic write
3. **Crypto engine**
   - Argon2id + XChaCha envelope v1
4. **Vault + metadata repos**
   - encrypted entry CRUD + state CRUD
5. **Core account flows**
   - `save`, `new`, `list`, `delete`
6. **Activation flow**
   - `activate` + rollback/verify
7. **Backup/restore**
   - writer/reader + conflict engine + all mode
8. **Usage**
   - confidence-tiered parser behavior
9. **TUI**
   - account/usage/backup-restore UI
10. **Docs + symlink**
   - `CLAUDE.md`, `AGENTS.md -> CLAUDE.md`
11. **Verification + hardening**
   - full tests and build matrix

---

## 11) Quality gates (must pass)

## Automated
- `go test ./...`
- `go test -race ./...`
- `GOOS=darwin GOARCH=arm64 go build ./...`
- `GOOS=darwin GOARCH=amd64 go build ./...`
- `GOOS=linux GOARCH=amd64 go build ./...`
- `GOOS=linux GOARCH=arm64 go build ./...`

## Security/correctness
- Crypto round-trip tests (vault + backup)
- Wrong-passphrase failures
- Tampered-ciphertext failures
- No plaintext persistence tests
- Restore conflict policy tests (`ask/overwrite/skip/rename`)
- `activate` atomicity + rollback tests
- File mode assertions (`0600`/`0700`)

---

## 12) Architect/tech-lead review checklist (for post-implementation review)

- [ ] Command surface matches contract (including `cma activate`)
- [ ] Library-first approach followed and justified
- [ ] No secrets logged or printed
- [ ] Encrypted-at-rest guaranteed for saved credentials
- [ ] Backup artifacts are always encrypted
- [ ] Restore supports selective + `--all` modes
- [ ] Atomic write + lock + rollback paths are validated
- [ ] Usage confidence labels correctly applied
- [ ] macOS/Linux build matrix passes
- [ ] CLAUDE.md + AGENTS symlink present
- [ ] Known limitations documented clearly

---

## 13) Risk register and mitigations

1. **Passphrase UX friction**
   - Mitigation: support `prompt` and `env:VAR`; keep unsafe literal behind explicit opt-in.
2. **Atomic write edge-cases on FS**
   - Mitigation: file + parent dir fsync, integration tests with injected failure points.
3. **Ambiguous restore conflicts**
   - Mitigation: deterministic precedence + explicit policy + clear prompts.
4. **Quota source instability**
   - Mitigation: confidence labeling, no fabricated precision.

---

## 14) Executor prompt (copy/paste)

```text
Implement codexmultiauth (`cma`) in Go from scratch in /Users/prakersh/projects/codexmultiauth using /Users/prakersh/projects/codexmultiauth/IMPLEMENTATION_PLAN.md as the source of truth.

IMPORTANT MODE:
- Execute implementation only (this planning phase is complete).
- Use a library-first strategy: prefer mature Go libraries and avoid reinventing solved primitives.
- Do not compromise required functionality or security guarantees.

Mandatory command scope:
1) cma list
2) cma usage <selector|all>
3) cma save
4) cma new [--device-auth]
5) cma activate <selector>
6) cma delete <selector>
7) cma backup <encrypthash/pass> <name|abspath>
8) cma restore <encrypthash/pass> <pathtobackup|name> [--all] [--conflict ask|overwrite|skip|rename]
9) cma tui

Security-critical requirements:
- Saved credentials must be encrypted at rest.
- Backup output must always be encrypted.
- Use Argon2id + XChaCha20-Poly1305 with versioned envelope metadata.
- Never print/log secrets (tokens/passphrases/decrypted payloads).
- Enforce strict file perms (0600 files, 0700 dirs).
- Use lock + atomic write + rollback for all mutating operations.

Behavior-critical requirements:
- `cma activate` must atomically write selected credentials into ~/.codex/auth.json and verify success.
- `cma restore` must decrypt, analyze, and support selective import and --all atomic import.
- Usage output must include confidence tiers (confirmed/best_effort/unknown); no fake precision.

External references required for Codex behavior:
- https://github.com/openai/codex
- https://developers.openai.com/codex/auth

Documentation/setup requirements:
- Create CLAUDE.md with project policies.
- Create symlink AGENTS.md -> CLAUDE.md.

Verification requirements (must run and report outputs):
- go test ./...
- go test -race ./...
- GOOS=darwin GOARCH=arm64 go build ./...
- GOOS=darwin GOARCH=amd64 go build ./...
- GOOS=linux GOARCH=amd64 go build ./...
- GOOS=linux GOARCH=arm64 go build ./...

Final report must include:
- what was implemented per command
- security guarantees achieved
- test/build outputs
- known limitations and follow-up suggestions
```
