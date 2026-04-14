#!/usr/bin/env bash

set -euo pipefail

CMA_BIN="${CMA_BIN:-cma}"
SLEEP_SECONDS="${SLEEP_SECONDS:-300}"

capture_limits() {
  "${CMA_BIN}" limits --dull | sed -E 's/\[[^]]+\] Codex Limits/\[timestamp\] Codex Limits/'
}

echo "Running initial auto-selection..."
"${CMA_BIN}" auto

previous="$(capture_limits)"
printf '%s\n' "${previous}"

while true; do
  sleep "${SLEEP_SECONDS}"

  current="$(capture_limits)"
  printf '%s\n' "${current}"

  if [[ "${current}" != "${previous}" ]]; then
    echo "Limits changed. Running auto-selection..."
    "${CMA_BIN}" auto
    previous="$(capture_limits)"
    printf '%s\n' "${previous}"
    continue
  fi

  previous="${current}"
done
