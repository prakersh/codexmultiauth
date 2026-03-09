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
echo "env HOME=${HOME} XDG_CONFIG_HOME=${XDG_CONFIG_HOME:-} CODEX_HOME=${CODEX_HOME:-} CMA_DISABLE_KEYRING=${CMA_DISABLE_KEYRING:-}" >> "${APP_SH_TEST_LOG}"
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

cat > "${FAKE_BIN}/gh" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "gh $*" >> "${APP_SH_TEST_LOG}"
if [[ "${1:-}" == "auth" && "${2:-}" == "status" ]]; then
  echo "github.com"
  exit 0
fi
if [[ "${1:-}" == "release" && "${2:-}" == "create" ]]; then
  echo "https://github.com/prakersh/codexmultiauth/releases/tag/${3:-unknown}"
  exit 0
fi
exit 0
EOF

cat > "${FAKE_BIN}/shasum" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
echo "shasum $*" >> "${APP_SH_TEST_LOG}"
if [[ "${1:-}" == "-a" ]]; then
  shift 2
fi
for file in "$@"; do
  echo "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef  ${file}"
done
EOF

cat > "${FAKE_BIN}/mktemp" <<'EOF'
#!/usr/bin/env bash
set -euo pipefail
dir="${TMP_DIR_FOR_FAKE_MKTEMP}/sandbox"
mkdir -p "${dir}"
echo "${dir}"
EOF

chmod +x "${FAKE_BIN}/go" "${FAKE_BIN}/git" "${FAKE_BIN}/gofmt" "${FAKE_BIN}/gh" "${FAKE_BIN}/shasum" "${FAKE_BIN}/mktemp"

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
    idx="$(grep -n -x -F "${token}" "${file}" | head -n1 | cut -d: -f1 || true)"
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
  env PATH="${FAKE_BIN}:$PATH" APP_SH_TEST_LOG="${LOG_FILE}" APP_SH_TRACE_FILE="${TRACE_FILE}" TMP_DIR_FOR_FAKE_MKTEMP="${TMP_DIR}" "$@"
}

echo "test: help output"
help_out="$(run_with_env "${APP_SH}" --help)"
assert_contains "${help_out}" "USAGE"
assert_contains "${help_out}" "--verify"
assert_contains "${help_out}" "--verify-sandbox"
assert_contains "${help_out}" "--publish-release"

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
cat > "${TMP_DIR}/notes.md" <<'EOF'
release notes
EOF
run_with_env "${APP_SH}" --deps --clean --fmt --lint --test --race --cover --build --smoke --verify --verify-sandbox --release --publish-release --draft --notes-file "${TMP_DIR}/notes.md" >/dev/null
assert_order "${TRACE_FILE}" deps clean fmt lint test race cover build smoke verify verify-sandbox release publish-release

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

echo "test: verify-sandbox isolates env"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
run_with_env "${APP_SH}" --verify-sandbox >/dev/null
assert_file_contains "${TRACE_FILE}" "verify-sandbox"
assert_file_contains "${LOG_FILE}" "go test ./... -count=1"
assert_file_contains "${LOG_FILE}" "CMA_DISABLE_KEYRING=1"
assert_file_contains "${LOG_FILE}" "XDG_CONFIG_HOME=${TMP_DIR}/sandbox/xdg"
assert_file_contains "${LOG_FILE}" "CODEX_HOME=${TMP_DIR}/sandbox/codex"

echo "test: publish-release gates on verify-sandbox and release"
: > "${LOG_FILE}"
: > "${TRACE_FILE}"
cat > "${TMP_DIR}/notes.md" <<'EOF'
release notes
EOF
run_with_env "${APP_SH}" --publish-release --draft --tag v9.9.9 --notes-file "${TMP_DIR}/notes.md" >/dev/null
assert_order "${TRACE_FILE}" publish-release verify-sandbox release
assert_file_contains "${LOG_FILE}" "gh auth status"
assert_file_contains "${LOG_FILE}" "gh release create v9.9.9"
assert_file_contains "${LOG_FILE}" "shasum -a 256"

echo "all app.sh tests passed"
