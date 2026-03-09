#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
APP_SH="${SCRIPT_DIR}/app.sh"

usage() {
    cat <<EOF
test.sh - thin wrapper over app.sh

Usage:
  ./test.sh <quick|full|prerelease|publish> [extra app.sh args...]

Commands:
  quick       Run ./app.sh --smoke
  full        Run ./app.sh --verify-sandbox
  prerelease  Run ./app.sh --verify-sandbox --release
  publish     Run ./app.sh --publish-release --draft

Examples:
  ./test.sh quick
  ./test.sh full
  ./test.sh prerelease
  ./test.sh publish -- --notes-file docs/release-notes.md
EOF
}

if [[ $# -eq 0 ]]; then
    usage
    exit 0
fi

command="$1"
shift

case "${command}" in
    quick)
        exec "${APP_SH}" --smoke "$@"
        ;;
    full)
        exec "${APP_SH}" --verify-sandbox "$@"
        ;;
    prerelease)
        exec "${APP_SH}" --verify-sandbox --release "$@"
        ;;
    publish)
        exec "${APP_SH}" --publish-release --draft "$@"
        ;;
    --help|-h|help)
        usage
        ;;
    *)
        echo "Unknown test.sh command: ${command}" >&2
        usage
        exit 1
        ;;
esac
