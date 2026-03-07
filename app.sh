#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${SCRIPT_DIR}/bin"
DIST_DIR="${SCRIPT_DIR}/dist"
VERSION_FILE="${SCRIPT_DIR}/cmd/VERSION"
BINARY_PATH="${BIN_DIR}/cma"
SCRIPT_VERSION="1.0.0"

REPOSITORY_URL="https://github.com/prakersh/codexmultiauth"
SUPPORT_URL="https://buymeacoffee.com/prakersh"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
BOLD='\033[1m'
NC='\033[0m'

info()    { echo -e "${CYAN}${BOLD}==> $1${NC}"; }
success() { echo -e "${GREEN}${BOLD}==> $1${NC}"; }
warn()    { echo -e "${YELLOW}${BOLD}==> $1${NC}"; }
error()   { echo -e "${RED}${BOLD}==> ERROR: $1${NC}" >&2; }

trace_step() {
    local step="$1"
    if [[ -n "${APP_SH_TRACE_FILE:-}" ]]; then
        echo "${step}" >> "${APP_SH_TRACE_FILE}"
    fi
}

read_app_version() {
    if [[ -f "${VERSION_FILE}" ]]; then
        local v
        v="$(tr -d '[:space:]' < "${VERSION_FILE}")"
        if [[ -n "${v}" ]]; then
            echo "${v}"
            return
        fi
    fi
    echo "dev"
}

build_commit() {
    git rev-parse --short HEAD 2>/dev/null || echo "none"
}

build_date() {
    date -u +%Y-%m-%dT%H:%M:%SZ
}

build_ldflags() {
    local version="$1"
    local commit="$2"
    local date="$3"
    echo "-X github.com/prakersh/codexmultiauth/cmd.Version=${version} -X github.com/prakersh/codexmultiauth/cmd.Commit=${commit} -X github.com/prakersh/codexmultiauth/cmd.Date=${date}"
}

usage() {
    cat <<EOF
${BOLD}cma orchestration script${NC}

${CYAN}USAGE:${NC}
  ./app.sh [FLAGS...] [-- run-args...]

${CYAN}FLAGS:${NC}
  --help, -h          Show usage
  --deps, -d          Verify required tooling (go, git)
  --clean, -c         Remove build and coverage artifacts
  --fmt               Run gofmt on project Go files
  --lint              Run go vet ./...
  --test, -t          Run go test ./... -count=1
  --race              Run go test -race ./... -count=1
  --cover             Run coverage commands and print summary
  --build, -b         Build ./bin/cma with ldflags metadata
  --run, -r           Build and run cma (args after --)
  --smoke, -s         Quick checks (vet + build + short test)
  --verify            Full verification matrix
  --release           Build dist binaries for darwin/linux amd64+arm64
  --version           Print script/app version info

${CYAN}EXAMPLES:${NC}
  ./app.sh --build
  ./app.sh --test --cover
  ./app.sh --run -- version
  ./app.sh --run -- tui
  ./app.sh --verify
  ./app.sh --release

${CYAN}ORDER:${NC}
  deps -> clean -> fmt -> lint -> test -> race -> cover -> build -> smoke -> verify -> release -> run
EOF
}

ensure_tools() {
    trace_step "deps"
    info "deps: verifying required tooling"
    local missing=0
    if ! command -v go >/dev/null 2>&1; then
        error "Go is required. Install from https://go.dev/dl/"
        missing=1
    else
        info "deps: $(go version)"
    fi
    if ! command -v git >/dev/null 2>&1; then
        error "git is required. Install from https://git-scm.com/downloads"
        missing=1
    else
        info "deps: $(git --version)"
    fi
    if [[ "${missing}" -ne 0 ]]; then
        exit 1
    fi
    success "deps: ready"
}

do_clean() {
    trace_step "clean"
    info "clean: removing artifacts"
    rm -rf "${BIN_DIR}" "${DIST_DIR}"
    rm -f "${SCRIPT_DIR}/coverage.out" "${SCRIPT_DIR}/coverage_internal.out" "${SCRIPT_DIR}/internal_coverage.out"
    go clean -testcache
    success "clean: done"
}

do_fmt() {
    trace_step "fmt"
    info "fmt: running gofmt"
    find "${SCRIPT_DIR}" -type f -name '*.go' -not -path '*/dist/*' -not -path '*/bin/*' -exec gofmt -w {} +
    success "fmt: done"
}

do_lint() {
    trace_step "lint"
    info "lint: go vet ./..."
    (cd "${SCRIPT_DIR}" && go vet ./...)
    success "lint: done"
}

do_test() {
    trace_step "test"
    info "test: go test ./... -count=1"
    (cd "${SCRIPT_DIR}" && go test ./... -count=1)
    success "test: done"
}

do_race() {
    trace_step "race"
    info "race: go test -race ./... -count=1"
    (cd "${SCRIPT_DIR}" && go test -race ./... -count=1)
    success "race: done"
}

run_cover_commands() {
    (cd "${SCRIPT_DIR}" && go test ./... -covermode=atomic -coverprofile=coverage.out)
    (cd "${SCRIPT_DIR}" && go tool cover -func=coverage.out)
    (cd "${SCRIPT_DIR}" && go test ./internal/... -covermode=atomic -coverprofile=coverage_internal.out)
    (cd "${SCRIPT_DIR}" && go tool cover -func=coverage_internal.out)
}

coverage_gate_status() {
    local overall internal
    overall="$(cd "${SCRIPT_DIR}" && go tool cover -func=coverage.out | awk '/^total:/ {gsub("%","",$3); print $3}')"
    internal="$(cd "${SCRIPT_DIR}" && go tool cover -func=coverage_internal.out | awk '/^total:/ {gsub("%","",$3); print $3}')"
    if awk "BEGIN { exit !(${overall} >= 80.0) }"; then
        success "coverage gate overall >= 80: ${overall}%"
    else
        warn "coverage gate overall >= 80: FAILED (${overall}%)"
    fi
    if awk "BEGIN { exit !(${internal} >= 80.0) }"; then
        success "coverage gate internal >= 80: ${internal}%"
    else
        warn "coverage gate internal >= 80: FAILED (${internal}%)"
    fi
}

do_cover() {
    trace_step "cover"
    info "cover: running coverage commands"
    run_cover_commands
    coverage_gate_status
    success "cover: done"
}

do_build() {
    trace_step "build"
    local version commit date ldflags
    version="$(read_app_version)"
    commit="$(build_commit)"
    date="$(build_date)"
    ldflags="$(build_ldflags "${version}" "${commit}" "${date}")"
    info "build: creating ${BINARY_PATH} (version=${version}, commit=${commit})"
    mkdir -p "${BIN_DIR}"
    (cd "${SCRIPT_DIR}" && go build -ldflags "${ldflags}" -o "${BINARY_PATH}" .)
    success "build: done"
}

do_smoke() {
    trace_step "smoke"
    info "smoke: vet + build + short test"
    (cd "${SCRIPT_DIR}" && go vet ./...)
    local version commit date ldflags
    version="$(read_app_version)"
    commit="$(build_commit)"
    date="$(build_date)"
    ldflags="$(build_ldflags "${version}" "${commit}" "${date}")"
    mkdir -p "${BIN_DIR}"
    (cd "${SCRIPT_DIR}" && go build -ldflags "${ldflags}" -o "${BINARY_PATH}" .)
    (cd "${SCRIPT_DIR}" && go test ./... -short -count=1)
    success "smoke: done"
}

do_verify() {
    trace_step "verify"
    info "verify: running full matrix"
    (cd "${SCRIPT_DIR}" && go test ./... -count=1)
    (cd "${SCRIPT_DIR}" && go test -race ./... -count=1)
    run_cover_commands
    coverage_gate_status
    (cd "${SCRIPT_DIR}" && GOOS=darwin GOARCH=arm64 go build ./...)
    (cd "${SCRIPT_DIR}" && GOOS=darwin GOARCH=amd64 go build ./...)
    (cd "${SCRIPT_DIR}" && GOOS=linux GOARCH=amd64 go build ./...)
    (cd "${SCRIPT_DIR}" && GOOS=linux GOARCH=arm64 go build ./...)
    success "verify: done"
}

do_release() {
    trace_step "release"
    local version commit date ldflags
    version="$(read_app_version)"
    commit="$(build_commit)"
    date="$(build_date)"
    ldflags="$(build_ldflags "${version}" "${commit}" "${date}")"
    info "release: building dist artifacts for version ${version}"
    mkdir -p "${DIST_DIR}"

    local targets=(
        "darwin arm64"
        "darwin amd64"
        "linux amd64"
        "linux arm64"
    )
    local os arch output
    for target in "${targets[@]}"; do
        os="${target%% *}"
        arch="${target##* }"
        output="${DIST_DIR}/cma_${version}_${os}_${arch}"
        info "release: ${output}"
        (cd "${SCRIPT_DIR}" && CGO_ENABLED=0 GOOS="${os}" GOARCH="${arch}" go build -ldflags "${ldflags}" -o "${output}" .)
    done
    success "release: done"
}

do_run() {
    trace_step "run"
    if [[ ! -x "${BINARY_PATH}" ]]; then
        do_build
    fi
    local args=("$@")
    if [[ "${#args[@]}" -eq 0 ]]; then
        args=(--help)
    fi
    info "run: ${BINARY_PATH} ${args[*]}"
    "${BINARY_PATH}" "${args[@]}"
}

print_versions() {
    local app_version commit date
    app_version="$(read_app_version)"
    commit="$(build_commit)"
    date="$(build_date)"
    echo "app.sh version: ${SCRIPT_VERSION}"
    echo "cma version: ${app_version}"
    echo "commit: ${commit}"
    echo "date: ${date}"
    echo "repository: ${REPOSITORY_URL}"
    echo "support: ${SUPPORT_URL}"
}

DO_DEPS=false
DO_CLEAN=false
DO_FMT=false
DO_LINT=false
DO_TEST=false
DO_RACE=false
DO_COVER=false
DO_BUILD=false
DO_RUN=false
DO_SMOKE=false
DO_VERIFY=false
DO_RELEASE=false
DO_VERSION=false
RUN_ARGS=()

if [[ $# -eq 0 ]]; then
    usage
    exit 0
fi

while [[ $# -gt 0 ]]; do
    case "$1" in
        --help|-h)
            usage
            exit 0
            ;;
        --deps|-d)
            DO_DEPS=true
            ;;
        --clean|-c)
            DO_CLEAN=true
            ;;
        --fmt)
            DO_FMT=true
            ;;
        --lint)
            DO_LINT=true
            ;;
        --test|-t)
            DO_TEST=true
            ;;
        --race)
            DO_RACE=true
            ;;
        --cover)
            DO_COVER=true
            ;;
        --build|-b)
            DO_BUILD=true
            ;;
        --run|-r)
            DO_RUN=true
            ;;
        --smoke|-s)
            DO_SMOKE=true
            ;;
        --verify)
            DO_VERIFY=true
            ;;
        --release)
            DO_RELEASE=true
            ;;
        --version)
            DO_VERSION=true
            ;;
        --)
            shift
            RUN_ARGS=("$@")
            break
            ;;
        -*)
            error "Unknown flag: $1"
            usage
            exit 1
            ;;
        *)
            error "Unknown argument: $1"
            usage
            exit 1
            ;;
    esac
    shift
done

if [[ "${DO_VERSION}" == "true" ]]; then
    print_versions
fi

if [[ "${DO_DEPS}" == "true" ]]; then
    ensure_tools
fi
if [[ "${DO_CLEAN}" == "true" ]]; then
    do_clean
fi
if [[ "${DO_FMT}" == "true" ]]; then
    do_fmt
fi
if [[ "${DO_LINT}" == "true" ]]; then
    do_lint
fi
if [[ "${DO_TEST}" == "true" ]]; then
    do_test
fi
if [[ "${DO_RACE}" == "true" ]]; then
    do_race
fi
if [[ "${DO_COVER}" == "true" ]]; then
    do_cover
fi
if [[ "${DO_BUILD}" == "true" ]]; then
    do_build
fi
if [[ "${DO_SMOKE}" == "true" ]]; then
    do_smoke
fi
if [[ "${DO_VERIFY}" == "true" ]]; then
    do_verify
fi
if [[ "${DO_RELEASE}" == "true" ]]; then
    do_release
fi
if [[ "${DO_RUN}" == "true" ]]; then
    do_run "${RUN_ARGS[@]}"
fi
