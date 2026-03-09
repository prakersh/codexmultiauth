#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
BIN_DIR="${SCRIPT_DIR}/bin"
DIST_DIR="${SCRIPT_DIR}/dist"
VERSION_FILE="${SCRIPT_DIR}/cmd/VERSION"
BINARY_PATH="${BIN_DIR}/cma"
CHECKSUM_FILE="${DIST_DIR}/sha256sums.txt"
SCRIPT_VERSION="1.0.0"
HOST_HOME="${HOME:-}"
HOST_CODEX_AUTH="${HOST_HOME}/.codex/auth.json"
HOST_CMA_DIR="${HOST_HOME}/.config/cma"

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

stat_line() {
    local path="$1"
    if stat -f '%N|%HT|%m|%z|%Sp' "$path" >/dev/null 2>&1; then
        stat -f '%N|%HT|%m|%z|%Sp' "$path"
    else
        stat -c '%n|%F|%Y|%s|%A' "$path"
    fi
}

file_digest() {
    local path="$1"
    if command -v shasum >/dev/null 2>&1; then
        shasum "${path}"
        return
    fi
    if command -v sha1sum >/dev/null 2>&1; then
        sha1sum "${path}"
        return
    fi
    error "verify-sandbox: requires shasum or sha1sum for host metadata hashing"
    exit 1
}

capture_host_metadata() {
    local output="$1"
    : > "${output}"
    for path in "${HOST_CODEX_AUTH}" "${HOST_CMA_DIR}"; do
        if [[ ! -e "${path}" ]]; then
            printf 'MISSING|%s\n' "${path}" >> "${output}"
            continue
        fi
        if [[ -f "${path}" ]]; then
            printf 'FILE|%s\n' "${path}" >> "${output}"
            stat_line "${path}" >> "${output}"
            file_digest "${path}" >> "${output}"
            continue
        fi
        if [[ -d "${path}" ]]; then
            printf 'DIR|%s\n' "${path}" >> "${output}"
            stat_line "${path}" >> "${output}"
            find "${path}" -print0 | sort -z | while IFS= read -r -d '' item; do
                stat_line "${item}" >> "${output}"
                if [[ -f "${item}" ]]; then
                    file_digest "${item}" >> "${output}"
                fi
            done
            continue
        fi
        printf 'OTHER|%s\n' "${path}" >> "${output}"
        stat_line "${path}" >> "${output}"
    done
}

ensure_host_unchanged() {
    local before="$1"
    local after="$2"
    if cmp -s "${before}" "${after}"; then
        success "verify-sandbox: host paths unchanged"
        return
    fi
    error "verify-sandbox: host mutation detected"
    diff -u "${before}" "${after}" || true
    exit 1
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
  --verify-sandbox    Full verification matrix in isolated temp HOME/XDG/CODEX
  --release           Build dist binaries for darwin/linux amd64+arm64
  --publish-release   Draft-first GitHub release publish flow
  --tag <tag>         Override release tag (default: v<cmd/VERSION>)
  --draft             Create GitHub release as draft (default behavior)
  --notes-file <path> Release notes file for GitHub release
  --version           Print script/app version info

${CYAN}EXAMPLES:${NC}
  ./app.sh --build
  ./app.sh --test --cover
  ./app.sh --run -- version
  ./app.sh --run -- tui
  ./app.sh --verify
  ./app.sh --verify-sandbox
  ./app.sh --release
  ./app.sh --publish-release --draft --notes-file docs/release-notes.md

${CYAN}ORDER:${NC}
  deps -> clean -> fmt -> lint -> test -> race -> cover -> build -> smoke -> verify -> verify-sandbox -> release -> publish-release -> run
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

default_release_tag() {
    echo "v$(read_app_version)"
}

checksum_cmd() {
    if command -v shasum >/dev/null 2>&1; then
        echo "shasum -a 256"
        return
    fi
    if command -v sha256sum >/dev/null 2>&1; then
        echo "sha256sum"
        return
    fi
    error "release: requires shasum or sha256sum for checksums"
    exit 1
}

required_release_assets() {
    local version="$1"
    printf '%s\n' \
        "${DIST_DIR}/cma_${version}_darwin_arm64" \
        "${DIST_DIR}/cma_${version}_darwin_amd64" \
        "${DIST_DIR}/cma_${version}_linux_amd64" \
        "${DIST_DIR}/cma_${version}_linux_arm64"
}

generate_checksums() {
    local version="$1"
    local checksum_tool
    local assets=()
    local asset
    checksum_tool="$(checksum_cmd)"
    info "release: generating ${CHECKSUM_FILE}"
    mkdir -p "${DIST_DIR}"
    while IFS= read -r asset; do
        assets+=("${asset}")
    done < <(required_release_assets "${version}")
    case "${checksum_tool}" in
        "shasum -a 256")
            (cd "${DIST_DIR}" && shasum -a 256 "$(basename "${assets[0]}")" "$(basename "${assets[1]}")" "$(basename "${assets[2]}")" "$(basename "${assets[3]}")") > "${CHECKSUM_FILE}"
            ;;
        "sha256sum")
            (cd "${DIST_DIR}" && sha256sum "$(basename "${assets[0]}")" "$(basename "${assets[1]}")" "$(basename "${assets[2]}")" "$(basename "${assets[3]}")") > "${CHECKSUM_FILE}"
            ;;
    esac
}

ensure_release_artifacts() {
    local version="$1"
    local asset
    while IFS= read -r asset; do
        if [[ ! -f "${asset}" ]]; then
            error "release: missing artifact ${asset}"
            exit 1
        fi
    done < <(required_release_assets "${version}")
    if [[ ! -f "${CHECKSUM_FILE}" ]]; then
        error "release: missing checksum file ${CHECKSUM_FILE}"
        exit 1
    fi
}

do_verify_sandbox() {
    trace_step "verify-sandbox"
    if [[ "${VERIFY_SANDBOX_DONE}" == "true" ]]; then
        success "verify-sandbox: already passed"
        return
    fi

    info "verify-sandbox: running full matrix in isolated temp sandbox"
    local tmproot pre_meta post_meta sandbox_home sandbox_xdg sandbox_codex
    tmproot="$(mktemp -d)"
    sandbox_home="${tmproot}/home"
    sandbox_xdg="${tmproot}/xdg"
    sandbox_codex="${tmproot}/codex"
    mkdir -p "${sandbox_home}" "${sandbox_xdg}" "${sandbox_codex}"
    pre_meta="${tmproot}/host_pre.txt"
    post_meta="${tmproot}/host_post.txt"

    capture_host_metadata "${pre_meta}"
    (
        export HOME="${sandbox_home}"
        export XDG_CONFIG_HOME="${sandbox_xdg}"
        export CODEX_HOME="${sandbox_codex}"
        export CMA_DISABLE_KEYRING=1
        info "verify-sandbox: HOME=${HOME}"
        info "verify-sandbox: XDG_CONFIG_HOME=${XDG_CONFIG_HOME}"
        info "verify-sandbox: CODEX_HOME=${CODEX_HOME}"
        do_verify
    )
    capture_host_metadata "${post_meta}"
    ensure_host_unchanged "${pre_meta}" "${post_meta}"
    VERIFY_SANDBOX_DONE=true
    success "verify-sandbox: done"
}

do_release() {
    trace_step "release"
    if [[ "${RELEASE_DONE}" == "true" ]]; then
        success "release: already built"
        return
    fi
    local version commit date ldflags
    version="$(read_app_version)"
    commit="$(build_commit)"
    date="$(build_date)"
    ldflags="$(build_ldflags "${version}" "${commit}" "${date}")"
    info "release: building dist artifacts for version ${version}"
    rm -rf "${DIST_DIR}"
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
    generate_checksums "${version}"
    RELEASE_DONE=true
    success "release: done"
}

ensure_gh_ready() {
    if ! command -v gh >/dev/null 2>&1; then
        error "publish-release: gh CLI is required"
        exit 1
    fi
    info "publish-release: checking gh auth status"
    gh auth status >/dev/null
}

ensure_release_notes_file() {
    if [[ -n "${RELEASE_NOTES_FILE}" ]]; then
        if [[ ! -f "${RELEASE_NOTES_FILE}" ]]; then
            error "publish-release: notes file not found: ${RELEASE_NOTES_FILE}"
            exit 1
        fi
        echo "${RELEASE_NOTES_FILE}"
        return
    fi

    local tmp_notes version commit
    version="$(read_app_version)"
    commit="$(build_commit)"
    tmp_notes="$(mktemp)"
    cat > "${tmp_notes}" <<EOF
## cma ${version}

- Version: ${version}
- Commit: ${commit}
- Repository: ${REPOSITORY_URL}
- Support: ${SUPPORT_URL}
- Verification: sandbox verification and release artifact build passed
EOF
    echo "${tmp_notes}"
}

do_publish_release() {
    trace_step "publish-release"
    info "publish-release: preparing GitHub release"

    if [[ "${VERIFY_SANDBOX_DONE}" != "true" ]]; then
        do_verify_sandbox
    fi
    if [[ "${RELEASE_DONE}" != "true" ]]; then
        do_release
    fi

    ensure_gh_ready

    local version tag notes_file release_title release_url
    version="$(read_app_version)"
    tag="${RELEASE_TAG:-$(default_release_tag)}"
    notes_file="$(ensure_release_notes_file)"

    if git ls-remote --tags origin "refs/tags/${tag}" | grep -q .; then
        error "publish-release: remote tag already exists: ${tag}"
        exit 1
    fi

    ensure_release_artifacts "${version}"

    release_title="cma ${version}"
    info "publish-release: creating GitHub release ${tag}"
    local release_assets=()
    local release_asset
    while IFS= read -r release_asset; do
        release_assets+=("${release_asset}")
    done < <(required_release_assets "${version}")
    if [[ "${RELEASE_DRAFT}" == "true" ]]; then
        release_url="$(gh release create "${tag}" "${release_assets[@]}" "${CHECKSUM_FILE}" --title "${release_title}" --notes-file "${notes_file}" --draft)"
    else
        release_url="$(gh release create "${tag}" "${release_assets[@]}" "${CHECKSUM_FILE}" --title "${release_title}" --notes-file "${notes_file}")"
    fi
    success "publish-release: ${release_url}"
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
DO_VERIFY_SANDBOX=false
DO_RELEASE=false
DO_PUBLISH_RELEASE=false
DO_VERSION=false
RUN_ARGS=()
RELEASE_TAG=""
RELEASE_DRAFT=true
RELEASE_NOTES_FILE=""
VERIFY_SANDBOX_DONE=false
RELEASE_DONE=false

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
        --verify-sandbox)
            DO_VERIFY_SANDBOX=true
            ;;
        --release)
            DO_RELEASE=true
            ;;
        --publish-release)
            DO_PUBLISH_RELEASE=true
            ;;
        --tag)
            shift
            if [[ $# -eq 0 ]]; then
                error "--tag requires a value"
                exit 1
            fi
            RELEASE_TAG="$1"
            ;;
        --draft)
            RELEASE_DRAFT=true
            ;;
        --notes-file)
            shift
            if [[ $# -eq 0 ]]; then
                error "--notes-file requires a path"
                exit 1
            fi
            RELEASE_NOTES_FILE="$1"
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
if [[ "${DO_VERIFY_SANDBOX}" == "true" ]]; then
    do_verify_sandbox
fi
if [[ "${DO_RELEASE}" == "true" ]]; then
    do_release
fi
if [[ "${DO_PUBLISH_RELEASE}" == "true" ]]; then
    do_publish_release
fi
if [[ "${DO_RUN}" == "true" ]]; then
    if [[ "${#RUN_ARGS[@]}" -gt 0 ]]; then
        do_run "${RUN_ARGS[@]}"
    else
        do_run
    fi
fi
