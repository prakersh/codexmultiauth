# CMA Manual Test Report

- Date: 2026-03-09
- Scope baseline: remediation closure for audit suites M1-M12
- Environment used:
  - OS: macOS 25.1.0 (arm64)
  - Auth mode: isolated temp HOME environments
  - Keyring mode: `CMA_DISABLE_KEYRING=1`
  - Live-account closure:
    - current real Codex auth was cloned into an isolated temp HOME for M4.3 and M9.1
  - Interactive closure:
    - PTY-driven survey sessions were used for prompt and TUI cases

## Case Matrix

| Case ID | Result | Expected | Actual | Evidence |
|---|---|---|---|---|
| M1.1 `./app.sh --help` | PASS | Help prints flags and usage | Help output printed | `docs/audit_artifacts/manual_M1_1_app_help.log` |
| M1.2 `./app.sh --build` | PASS | Build succeeds | Binary built in `./bin/cma` | `docs/audit_artifacts/manual_M1_2_app_build.log` |
| M1.3 `./app.sh --run -- version` | PASS | Version and links printed | Output matched | `docs/audit_artifacts/manual_M1_3_app_run_version.log` |
| M1.4 `./app.sh --verify` | PASS | Full verify matrix passes | Verify completed with coverage gates passing | `docs/audit_artifacts/manual_M1_4_app_verify.log` |
| M2.1 `cma version` | PASS | Version, repo, and support links printed | Output matched | `docs/audit_artifacts/manual_M2_1_version.log` |
| M2.2 `cma version --short` | PASS | Version only | Output: `0.0.1` | `docs/audit_artifacts/manual_M2_2_version_short.log` |
| M2.3 ldflags override | PASS | Overridden version visible | Output: `9.9.9-audit` | `docs/audit_artifacts/manual_M2_3_ldflags.log` |
| M3.1 `save` account A | PASS | Account saved | `Saved acct-a` | `docs/audit_artifacts/manual_M3_1_save_A.log` |
| M3.2 dedupe save | PASS | Existing account reused | `Already saved as acct-a` | `docs/audit_artifacts/manual_M3_2_save_dedupe.log` |
| M3.3 `list` active marker | PASS | Single active account shown | One active entry shown in this setup | `docs/audit_artifacts/manual_M3_3_list.log` |
| M4.1 save account B | PASS | Second account saved | `Saved acct-b` | `docs/audit_artifacts/manual_M4_1_save_B.log` |
| M4.2 rotate activate A/B | PASS | Activations switch auth atomically | Activation commands succeeded and auth payload switched between A and B | `docs/audit_artifacts/manual_M4_2_activate_*.log`, `docs/audit_artifacts/manual_M4_2_auth_after_*.json` |
| M4.3 `codex login status` after switch | PASS | Real status reflects active account | `Logged in using ChatGPT` after activating isolated real auth | `docs/audit_artifacts/manual_M4_3_activate_real.log`, `docs/audit_artifacts/manual_M4_3_codex_status_real.log` |
| M4.4 no manual auth file edits during switching | PASS | `cma activate` handles writes | Switching completed through CLI only | M4 evidence files |
| M5.1 `cma new` success | PASS | Login and save succeeds | `Saved acct-new` with codex stub | `docs/audit_artifacts/manual_M5_1_new_success.log` |
| M5.2 `cma new --device-auth` success | PASS | Device auth path succeeds | `Saved acct-device` with codex stub | `docs/audit_artifacts/manual_M5_2_new_device_auth.log` |
| M5.3 `cma new` failure rollback | PASS | Original auth restored on failure | Failure reproduced and original auth preserved | `docs/audit_artifacts/manual_M5_3_new_fail.log`, `docs/audit_artifacts/manual_M5_3_auth_after_fail.json` |
| M6.1 delete non-active | PASS | Non-active delete succeeds | `Deleted acct-a` | `docs/audit_artifacts/manual_M6_1_delete_non_active.log` |
| M6.2 delete active and reject | PASS | Reject keeps account | Prompt answered `No`; account remained listed | `docs/audit_artifacts/manual_M6_2_delete_cancel.log`, `docs/audit_artifacts/manual_M6_2_delete_cancel_list.log` |
| M6.3 delete active and confirm | PASS | Confirm deletes account | Prompt answered `Yes`; `Deleted acct-del` and list became empty | `docs/audit_artifacts/manual_M6_3_delete_confirm.log`, `docs/audit_artifacts/manual_M6_3_delete_confirm_list.log` |
| M7.1 backup `prompt` | PASS | Prompt passphrase flow writes encrypted backup | Backup file written through prompt flow | `docs/audit_artifacts/manual_M7_1_backup_prompt_script.log` |
| M7.2 backup `env:VAR` | PASS | Encrypted backup written | Backup file created | `docs/audit_artifacts/manual_M7_2_backup_env.log` |
| M7.3 backup `hash:<hex>` | PASS | Encrypted backup written | Backup file created | `docs/audit_artifacts/manual_M7_3_backup_hash.log` |
| M7.4 reject `pass:<literal>` without unsafe flag | PASS | Command rejected with clear error | Exit `1` with actionable stderr text | `docs/audit_artifacts/manual_fix_error_plain.log` |
| M7.5 restore selective interactive | PASS | Selected accounts restored only | Selected `acct-a`; imported `1` account | `docs/audit_artifacts/manual_M7_5_restore_selective.log`, `docs/audit_artifacts/manual_M7_5_restore_selective_list.log` |
| M7.6 restore `--all` atomic | PASS | All imported atomically | `Imported 4 account(s)` | `docs/audit_artifacts/manual_M7_6_restore_all.log` |
| M7.7 wrong passphrase path | PASS | Failure returns clear error | Exit `1` with `wrong passphrase or corrupted ciphertext` | `docs/audit_artifacts/manual_fix_error_wrong_pass.log` |
| M7.8 tampered backup path | PASS | Failure returns clear error | Exit `1` with parse error text | `docs/audit_artifacts/manual_fix_error_tampered.log` |
| M8.1 conflict policy `ask` | PASS | Interactive decision completes restore | Selected conflicting account and chose `rename`; imported `1` account | `docs/audit_artifacts/manual_M8_1_ask_complete.log`, `docs/audit_artifacts/manual_M8_1_ask_list.log` |
| M8.2 conflict policy `overwrite` | PASS | Conflicts overwritten | `Imported 4 account(s)`; stable list | `docs/audit_artifacts/manual_M8_2_overwrite*.log` |
| M8.3 conflict policy `skip` | PASS | Conflicts skipped | `Imported 0 account(s)` | `docs/audit_artifacts/manual_M8_3_skip*.log` |
| M8.4 conflict policy `rename` | PASS | Conflicts renamed | Restored names suffixed `-restored-2` | `docs/audit_artifacts/manual_M8_4_rename*.log` |
| M8 conflict reasons: fingerprint | PASS | Fingerprint conflicts detected | Policy behaviors validated on repeated restore | M8 logs |
| M8 conflict reasons: account_id | PASS | Account ID conflict decision path works | Prompt showed `[conflict:account_id]`; `skip` imported `0` | `docs/audit_artifacts/manual_M8_account_id.log` |
| M8 conflict reasons: display_name | PASS | Display name conflict decision path works | Prompt showed `[conflict:display_name]`; `rename` imported `1` | `docs/audit_artifacts/manual_M8_1_ask_complete.log` |
| M8 conflict reasons: alias | PASS | Alias conflict decision path works | Prompt showed `[conflict:alias]`; `skip` imported `0` | `docs/audit_artifacts/manual_M8_alias.log` |
| M9.1 confidence `confirmed` | PASS | API-confirmed usage confidence | Output included `confidence: confirmed` and quota categories | `docs/audit_artifacts/manual_M9_1_confirmed.log` |
| M9.2 confidence `best_effort` | PASS | Best-effort from token metadata | `confidence: best_effort`, `plan: plus` | `docs/audit_artifacts/manual_M9_2_best_effort.log` |
| M9.3 confidence `unknown` | PASS | Unknown when metadata absent | `confidence: unknown` | `docs/audit_artifacts/manual_M9_3_unknown.log` |
| M9.4 no fake precision | PASS | No fabricated precision in fallback output | Fallback output stayed within confidence semantics | `docs/audit_artifacts/manual_M9_usage_all.log` |
| M10.1-M10.4 TUI end-to-end | PASS | Save, activate, delete, backup, restore, and usage refresh all work interactively | One PTY session completed save, usage refresh, activate, backup, delete, and selective restore | `docs/audit_artifacts/manual_M10_tui_full.log`, `docs/audit_artifacts/manual_M10_tui_final_list.log` |
| M11.1 permissions | PASS | Files `0600`, dirs `0700` | Modes matched policy | `docs/audit_artifacts/manual_M11_1_permissions.log` |
| M11.2 no secret leak in logs/output | PASS | No token or passphrase in command logs | Leak scan empty | `docs/audit_artifacts/manual_M11_2_leak_scan_logs.log` |
| M11.3 no plaintext token in vault/backup | PASS | Encrypted artifacts contain no plaintext tokens | Plaintext scan empty | `docs/audit_artifacts/manual_M11_3_plaintext_scan.log` |
| M12.1 lock contention | PASS | Contention handled safely | Targeted fs contention tests passed | `docs/audit_artifacts/manual_M12_fs_faults.log` |
| M12.2 corrupted state/vault handling | PASS | Corruption handled with errors | Targeted store tests passed | `docs/audit_artifacts/manual_M12_store_faults.log` |
| M12.3 rollback on injected failures | PASS | Rollback protections hold | Targeted app and fs rollback tests passed | `docs/audit_artifacts/manual_M12_app_faults.log`, `docs/audit_artifacts/manual_M12_fs_faults.log` |

## Closure Notes for Previously Blocked Cases

### M4.3 real token status path

- Method:
  - cloned the current real Codex auth into an isolated temp HOME
  - activated that saved account in isolation
  - ran `codex login status`
- Result:
  - PASS
- Evidence:
  - `docs/audit_artifacts/manual_M4_3_codex_status_real.log`

### M6.2 and M6.3 active delete confirmation flows

- Method:
  - drove the survey confirmation prompt through a PTY session
- Results:
  - `No` kept the active account
  - `Yes` deleted the active account
- Evidence:
  - `docs/audit_artifacts/manual_M6_2_delete_cancel.log`
  - `docs/audit_artifacts/manual_M6_3_delete_confirm.log`

### M7.1 prompt backup and M7.5 selective restore

- Method:
  - used PTY input for prompt and multi-select interactions
- Results:
  - prompt passphrase backup succeeded
  - selective restore imported only the selected account
- Evidence:
  - `docs/audit_artifacts/manual_M7_1_backup_prompt_script.log`
  - `docs/audit_artifacts/manual_M7_5_restore_selective.log`

### M8.1 ask-policy completion and conflict subtype coverage

- Method:
  - created controlled restore conflicts and exercised interactive decisions
- Results:
  - ask-policy flow completed
  - conflict subtype coverage now includes `account_id`, `display_name`, and `alias`
- Evidence:
  - `docs/audit_artifacts/manual_M8_1_ask_complete.log`
  - `docs/audit_artifacts/manual_M8_account_id.log`
  - `docs/audit_artifacts/manual_M8_alias.log`

### M9.1 confirmed usage path

- Method:
  - ran `cma usage` against the isolated real current auth
- Result:
  - output reported `confidence: confirmed`
- Evidence:
  - `docs/audit_artifacts/manual_M9_1_confirmed.log`

### M10 full interactive TUI workflow

- Method:
  - drove a single TUI session through save, usage refresh, activate, backup, delete, and restore
- Result:
  - PASS
- Evidence:
  - `docs/audit_artifacts/manual_M10_tui_full.log`

## Manual Campaign Conclusion

- All critical suites requested for release signoff are covered in this report.
- The previously blocked interactive and live-account cases are closed with evidence.
- Security-focused manual checks still pass:
  - no secret leakage detected in logs
  - encrypted artifacts do not expose plaintext token values
  - permissions match the documented policy
- The final automated rerun also passed after this manual campaign.

The manual campaign does not block release.
