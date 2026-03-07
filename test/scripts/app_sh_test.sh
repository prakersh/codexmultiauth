#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_DIR="$(cd "${SCRIPT_DIR}/../.." && pwd)"
APP_SH="${REPO_DIR}/app.sh"

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "${TMP_DIR}"' EXIT

FAKE_BIN="${TMP_DIR}/bin"
LOG_FILE="${TMP_DIR}/calls.log"
TRACE_FILE="${TMP_DIR}/trace.log"
mkdir -p "${FAKE_BIN}"

cat > "${FAKE_BIN}/go" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "go $*" >> "${APP_SH_TEST_LOG}"
cmd="${1:-}"
if [[ "${cmd}" == "version" ]]; then
  echo "go version go1.24.2"
  exit 0
fi
if [[ "${cmd}" == "tool" && "${2:-}" == "cover" ]]; then
  echo "total: (statements) 85.1%"
  exit 0
fi
if [[ "${cmd}" == "build" ]]; then
  out=""
  prev=""
  for arg in "$@"; do
    if [[ "${prev}" == "-o" ]]; then
      out="${arg}"
    fi
    prev="${arg}"
  done
  if [[ -n "${out}" ]]; then
    mkdir -p "$(dirname "${out}")"
    cat > "${out}" <<'INNER'
#!/usr/bin/env bash
set -euo pipefail
echo "BIN_RUN $*" >> "${APP_SH_TEST_LOG}"
if [[ "${1:-}" == "--help" ]]; then
  echo "cma help"
fi
INNER
    chmod +x "${out}"
  fi
fi
exit 0
EOF

cat > "${FAKE_BIN}/git" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "git $*" >> "${APP_SH_TEST_LOG}"
if [[ "${1:-}" == "rev-parse" && "${2:-}" == "--short" && "${3:-}" == "HEAD" ]]; then
  echo "deadbee"
  exit 0
fi
if [[ "${1:-}" == "--version" ]]; then
  echo "git version 2.39.0"
  exit 0
fi
exit 0
EOF

cat > "${FAKE_BIN}/gofmt" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "gofmt $*" >> "${APP_SH_TEST_LOG}"
exit 0
EOF

chmod +x "${FAKE_BIN}/go" "${FAKE_BIN}/git" "${FAKE_BIN}/gofmt"

assert_contains() {
  local haystack="$1"
  local needle="$2"
  if [[ "${haystack}" != *"${needle}"* ]]; then
    echo "ASSERTION FAILED: expected to find '${needle}'" >&2
    exit 1
  fi
}

assert_file_contains() {
  local file="$1"
  local needle="$2"
  if ! grep -Fq "${needle}" "${file}"; then
    echo "ASSERTION FAILED: expected '${needle}' in ${file}" >&2
    exit 1
  fi
}

assert_order() {
  local file="$1"
  shift
  local prev=0
  for token in "$@"; do
    local idx
    idx="$(grep -n -F "${token}" "${file}" | head -n1 | cut -d: -f1 || true)"
    if [[ -z "${idx}" ]]; then
      echo "ASSERTION FAILED: missing token '${token}' in ${file}" >&2
      exit 1
    fi
    if (( idx <= prev )); then
      echo "ASSERTION FAILED: token '${token}' not in expected order" >&2
      exit 1
    fi
    prev="${idx}"
  done
}

run_with_env() {
  env PATH="${FAKE_BIN}:$PATH" APP_SH_TEST_LOG="${LOG_FILE}" APP_SH_TRACE_FILE="${TRACE_FILE}" "$@"
}

echo "test: help output"
help_out="$(run_with_env "${APP_SH}" --help)"
assert_contains "${help_out}" "USAGE"
assert_contains "${help_out}" "--verify"

echo "test: unknown flag fails"
set +e
unknown_out="$(run_with_env "${APP_SH}" --unknown 2>&1)"
unknown_rc=$?
set -e
if [[ ${unknown_rc} -eq 0 ]]; then
  echo "ASSERTION FAILED: unknown flag should fail" >&2
  exit 1
fi
assert_contains "${unknown_out}" "Unknown flag"

echo "test: combined flag execution order"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
run_with_env "${APP_SH}" --deps --clean --fmt --lint --test --race --cover --build --smoke --verify --release >/dev/null
assert_order "${TRACE_FILE}" deps clean fmt lint test race cover build smoke verify release

echo "test: run arg forwarding"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
run_with_env "${APP_SH}" --run -- version >/dev/null
assert_file_contains "${LOG_FILE}" "BIN_RUN version"

echo "test: run defaults to help without args"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
run_with_env "${APP_SH}" --run >/dev/null
assert_file_contains "${LOG_FILE}" "BIN_RUN --help"

echo "test: verify command path"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
run_with_env "${APP_SH}" --verify >/dev/null
assert_file_contains "${LOG_FILE}" "go test ./... -count=1"
assert_file_contains "${LOG_FILE}" "go test -race ./... -count=1"
assert_file_contains "${LOG_FILE}" "go test ./... -covermode=atomic -coverprofile=coverage.out"
assert_file_contains "${LOG_FILE}" "go tool cover -func=coverage.out"
assert_file_contains "${LOG_FILE}" "go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out"
assert_file_contains "${LOG_FILE}" "go tool cover -func=coverage_internal.out"
assert_file_contains "${LOG_FILE}" "go build ./..."

echo "all app.sh tests passed"
