#!/usr/bin/env bash

set -euo pipefail

CMA_BIN="${CMA_BIN:-cma}"
SLEEP_SECONDS="${SLEEP_SECONDS:-300}"
RULE_WIDTH="${RULE_WIDTH:-72}"

if [[ -t 1 && -z "${NO_COLOR:-}" ]]; then
  RESET=$'\033[0m'
  BOLD=$'\033[1m'
  DIM=$'\033[2m'
  BLUE=$'\033[34m'
  CYAN=$'\033[36m'
  GREEN=$'\033[32m'
  YELLOW=$'\033[33m'
  RED=$'\033[31m'
else
  RESET=""
  BOLD=""
  DIM=""
  BLUE=""
  CYAN=""
  GREEN=""
  YELLOW=""
  RED=""
fi

repeat_char() {
  local char="$1"
  local count="$2"
  local out=""

  while (( ${#out} < count )); do
    out+="${char}"
  done

  printf '%s\n' "${out:0:count}"
}

rule() {
  printf '%s%s%s\n' "${DIM}" "$(repeat_char "=" "${RULE_WIDTH}")" "${RESET}"
}

timestamp_now() {
  date '+%Y-%m-%d %I:%M:%S %p %Z'
}

format_time_short() {
  local epoch="$1"

  if date -r "${epoch}" '+%l:%M%p' >/dev/null 2>&1; then
    date -r "${epoch}" '+%l:%M%p' | sed 's/^ *//'
    return
  fi

  date -d "@${epoch}" '+%l:%M%p' | sed 's/^ *//'
}

next_check_time() {
  local next_epoch
  next_epoch=$(( $(date +%s) + SLEEP_SECONDS ))
  format_time_short "${next_epoch}"
}

log_line() {
  local color="$1"
  local label="$2"
  local message="$3"

  printf '%s[%s]%s %s\n' "${color}${BOLD}" "${label}" "${RESET}" "${message}"
}

print_block() {
  sed 's/^/  /'
}

cycle_header() {
  local cycle="$1"
  local title="$2"

  rule
  printf '%s[CYCLE %s]%s %s%s%s\n' "${BLUE}${BOLD}" "${cycle}" "${RESET}" "${CYAN}" "${title}" "${RESET}"
  printf '%s[WHEN ]%s %s\n' "${DIM}" "${RESET}" "$(timestamp_now)"
  rule
}

normalize_limits() {
  sed -E $'s/\x1B\\[[0-9;]*[[:alpha:]]//g' | sed -E 's/\[[^]]+\] Codex Limits/\[timestamp\] Codex Limits/'
}

run_auto() {
  local output

  log_line "${CYAN}" "AUTO" "Running ${CMA_BIN} auto..."
  if ! output="$("${CMA_BIN}" auto 2>&1)"; then
    log_line "${RED}" "ERR " "${CMA_BIN} auto failed."
    printf '%s\n' "${output}" | print_block >&2
    exit 1
  fi

  log_line "${GREEN}" "OK  " "Auto-selection finished."
  printf '%s\n' "${output}" | print_block
}

capture_limits() {
  local output

  if ! output="$("${CMA_BIN}" limits 2>&1)"; then
    log_line "${RED}" "ERR " "${CMA_BIN} limits failed."
    printf '%s\n' "${output}" | print_block >&2
    exit 1
  fi

  LAST_LIMITS_DISPLAY="${output}"
  LAST_LIMITS_COMPARE="$(printf '%s\n' "${output}" | normalize_limits)"
}

show_limits() {
  log_line "${CYAN}" "INFO" "$1"
  printf '%s\n' "${LAST_LIMITS_DISPLAY}"
}

show_sleep_notice() {
  log_line "${DIM}" "WAIT" "Sleeping for ${SLEEP_SECONDS} sec. Next check at $(next_check_time)."
}

on_interrupt() {
  printf '\n'
  log_line "${YELLOW}" "STOP" "Interrupted. Exiting autoloop."
  exit 130
}

trap on_interrupt INT TERM

cycle=1
cycle_header "${cycle}" "Initial auto-selection and baseline limits snapshot"
run_auto
capture_limits
show_limits "Baseline limits snapshot"
previous="${LAST_LIMITS_COMPARE}"
show_sleep_notice

while true; do
  sleep "${SLEEP_SECONDS}"
  cycle=$((cycle + 1))

  cycle_header "${cycle}" "Polling limits and checking for changes"
  capture_limits
  show_limits "Current limits snapshot"
  current="${LAST_LIMITS_COMPARE}"

  if [[ "${current}" != "${previous}" ]]; then
    log_line "${YELLOW}" "WARN" "Limits changed since the previous snapshot."
    run_auto
    capture_limits
    show_limits "Post-auto limits snapshot"
    previous="${LAST_LIMITS_COMPARE}"
  else
    log_line "${GREEN}" "OK  " "No limit change detected. Keeping the current active account."
    previous="${current}"
  fi

  show_sleep_notice
done
