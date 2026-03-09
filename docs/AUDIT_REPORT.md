# CMA Audit Report

- Date: 2026-03-09
- Repo: `codexmultiauth`
- Branch: `main`
- Initial audit baseline: `246f438`
- Remediation commit reviewed: `cbecc11`
- Auditor mode: remediation + manual closure sprint
- Environment: macOS 25.1.0 (arm64), `codex` present on PATH

## A) Audit Summary

This report closes the findings and blocked cases from the 2026-03-08 No-Go audit.

The remediation sprint fixed the three open operator-facing issues:

- failing commands now print concise, sanitized diagnostics to stderr
- `cma list` now shows at most one active marker
- `cma new` no longer forces an aliases prompt when aliases are omitted

The previously blocked manual cases were re-run in isolated temp homes, with PTY-driven interaction where required. The live-account-dependent cases were also closed by cloning the current real Codex auth into an isolated environment and validating the post-activation status path there.

At this phase, the prior release blockers are closed. Final release signoff depends on the fresh full verification matrix in the next phase.

## B) Automated Checks

Baseline commands already executed and retained as evidence:

- `./app.sh --deps --lint --test --race --cover`
  - Evidence: `docs/audit_artifacts/app_deps_lint_test_race_cover.log`
- `./app.sh --verify`
  - Evidence: `docs/audit_artifacts/app_verify.log`
- `go vet ./...`
  - Evidence: `docs/audit_artifacts/go_vet.log`

Tool availability:

- `staticcheck ./...`
  - Not run in this phase because `staticcheck` is not installed.
- `gosec ./...`
  - Not run in this phase because `gosec` is not installed.

## C) Findings Status

| Finding | Old State | New State | Evidence |
|---|---|---|---|
| P2-001 Silent failures with no diagnostic text | Open | Closed | `docs/audit_artifacts/manual_fix_error_plain.log`, `docs/audit_artifacts/manual_fix_error_wrong_pass.log`, `docs/audit_artifacts/manual_fix_error_tampered.log`, `main_test.go` |
| P2-002 Multiple active markers in `cma list` | Open | Closed | `docs/audit_artifacts/manual_fix_list_singular.log`, `internal/app/services_test.go` |
| P3-001 Optional aliases prompt behaves as required | Open | Closed | `docs/audit_artifacts/manual_fix_new_no_alias_prompt.log`, `cmd/cmd_test.go` |

### P2-001: Failing commands can exit silently

- Severity before remediation: P2
- Status: Closed
- Fix summary:
  - `main.go` now prints `Error: ...` to stderr on failure.
  - error text is sanitized before printing
- Manual evidence:
  - `docs/audit_artifacts/manual_fix_error_plain.log`
  - `docs/audit_artifacts/manual_fix_error_wrong_pass.log`
  - `docs/audit_artifacts/manual_fix_error_tampered.log`
- Test evidence:
  - `main_test.go`

### P2-002: Multiple active markers can appear in `cma list`

- Severity before remediation: P2
- Status: Closed
- Fix summary:
  - active display now follows a single precedence rule:
    1. current auth fingerprint match
    2. fallback to state active account ID when no fingerprint match exists
    3. never mark more than one account active
- Manual evidence:
  - `docs/audit_artifacts/manual_fix_list_singular.log`
- Test evidence:
  - `internal/app/services_test.go`

### P3-001: `cma new` treated optional aliases as required friction

- Severity before remediation: P3
- Status: Closed
- Fix summary:
  - blank aliases now proceed without an interactive prompt
- Manual evidence:
  - `docs/audit_artifacts/manual_fix_new_no_alias_prompt.log`
- Test evidence:
  - `cmd/cmd_test.go`

## D) Manual Closure Highlights

Previously blocked or partial critical cases now closed:

- M4.3 real token status after activation: PASS
  - `codex login status` returned `Logged in using ChatGPT`
  - Evidence: `docs/audit_artifacts/manual_M4_3_codex_status_real.log`
- M6.2 active delete cancel: PASS
  - account remained present after rejecting confirmation
  - Evidence: `docs/audit_artifacts/manual_M6_2_delete_cancel.log`, `docs/audit_artifacts/manual_M6_2_delete_cancel_list.log`
- M6.3 active delete confirm: PASS
  - prompt accepted and account deleted
  - Evidence: `docs/audit_artifacts/manual_M6_3_delete_confirm.log`, `docs/audit_artifacts/manual_M6_3_delete_confirm_list.log`
- M7.1 prompt passphrase backup: PASS
  - encrypted backup written via prompt flow
  - Evidence: `docs/audit_artifacts/manual_M7_1_backup_prompt_script.log`
- M7.5 selective interactive restore: PASS
  - single selected account restored
  - Evidence: `docs/audit_artifacts/manual_M7_5_restore_selective.log`, `docs/audit_artifacts/manual_M7_5_restore_selective_list.log`
- M8.1 ask-policy restore: PASS
  - interactive restore completed with `rename`
  - Evidence: `docs/audit_artifacts/manual_M8_1_ask_complete.log`
- M8 conflict subtype coverage: PASS
  - `account_id`: `docs/audit_artifacts/manual_M8_account_id.log`
  - `display_name`: `docs/audit_artifacts/manual_M8_1_ask_complete.log`
  - `alias`: `docs/audit_artifacts/manual_M8_alias.log`
- M9.1 confirmed usage path: PASS
  - output included `confidence: confirmed`
  - Evidence: `docs/audit_artifacts/manual_M9_1_confirmed.log`
- M10 full interactive TUI workflow: PASS
  - save, usage refresh, activate, backup, delete, and restore all completed in one session
  - Evidence: `docs/audit_artifacts/manual_M10_tui_full.log`, `docs/audit_artifacts/manual_M10_tui_final_list.log`

## E) Security Checklist Results

- No secrets printed in command output scanned during this audit: PASS
  - Evidence: `docs/audit_artifacts/manual_M11_2_leak_scan_logs.log`
- Vault and backup plaintext token scan: PASS
  - Evidence: `docs/audit_artifacts/manual_M11_3_plaintext_scan.log`
- Files and directories created with policy modes: PASS
  - Evidence: `docs/audit_artifacts/manual_M11_1_permissions.log`
- Locking, atomicity, and rollback tests: PASS
  - Evidence:
    - `docs/audit_artifacts/manual_M12_fs_faults.log`
    - `docs/audit_artifacts/manual_M12_app_faults.log`
    - `docs/audit_artifacts/manual_M12_store_faults.log`

## F) Coverage and Build Gates

Latest accepted baseline before the final rerun:

- Overall coverage: `85.4%`
  - Evidence: `docs/audit_artifacts/verify_cover_func.log`
- Internal coverage: `85.1%`
  - Evidence: `docs/audit_artifacts/verify_internal_cover_func.log`

Package baselines:

- `internal/app`: `87.7%`
- `internal/infra/crypto`: `93.4%`
- `internal/infra/fs`: `87.7%`
- `internal/infra/store`: `82.3%`
- `internal/infra/usage`: `91.2%`
- `internal/tui`: `80.4%`

These gates remain above the required thresholds before the final rerun.

## G) Open Risks

- `staticcheck` is not installed on this machine.
  - Install recommendation: `go install honnef.co/go/tools/cmd/staticcheck@latest`
- `gosec` is not installed on this machine.
  - Install recommendation: `go install github.com/securego/gosec/v2/cmd/gosec@latest`

No open P0 or P1 issues remain from the prior audit. No open P2 or P3 findings remain from the prior report either.

## H) Interim Verdict

**Release blockers from the 2026-03-08 audit are closed.**

The project moves from **No-Go** to **pending final verification**. If the fresh test, race, coverage, and cross-build matrix passes without regression, the release verdict can move to **Go**.
