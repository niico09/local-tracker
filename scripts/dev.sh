#!/usr/bin/env bash
#
# Local dev driver for local-tracker.
#
# Mirrors the Makefile targets so `make` is not required. Needs `go` and `npx`
# on PATH; intended for Git Bash or any POSIX shell.
#
# run/seed/backup execute through `go run` rather than the binary in bin/.
# Where Windows Smart App Control is enforced, Code Integrity blocks the
# freshly built unsigned binary (Permission denied, exit 126); `go run`
# compiles to a temp image that the policy does not block.
#
# Usage:
#   scripts/dev.sh <command> [args]
#
# Commands:
#   install  Install the dev-only Node deps (Tailwind CLI). Run once after clone.
#   css      Build the minified Tailwind bundle into internal/ui/static/app.css.
#   build    Build the CSS bundle, then compile the Go binary into bin/.
#   test     Run the Go test suite.
#   run      Build the CSS, then serve the app via go run. Default command.
#   seed     Apply the embedded Top 100 seed (idempotent) via go run.
#   backup   Snapshot the DB via go run: backup [-uploads] <dest>.
#   help     Print this help.
#
set -euo pipefail

# Resolve paths from the script location so it works from any directory.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO="${GO:-go}"

# input.css does `@import "tailwindcss"`, so the CLI needs the package
# installed locally; without it Tailwind fails with a cryptic resolve error.
ensure_deps() {
  if [[ ! -d node_modules ]]; then
    printf '%s\n' \
      'error: node_modules is missing; the Tailwind CLI needs local dev deps.' \
      '       Fix it with: ./scripts/dev.sh install' >&2
    exit 1
  fi
}

cmd_install() {
  npm install
}

cmd_css() {
  ensure_deps
  npx @tailwindcss/cli \
    -i internal/ui/static/input.css \
    -o internal/ui/static/app.css \
    --minify
}

cmd_build() {
  cmd_css
  CGO_ENABLED=0 "$GO" build -o bin/tracker ./cmd/tracker
}

cmd_test() {
  "$GO" test ./...
}

# Assets are embedded at compile time, so the CSS must be regenerated before
# `go run` compiles the server.
cmd_run() {
  cmd_css
  CGO_ENABLED=0 "$GO" run ./cmd/tracker serve
}

cmd_seed() {
  CGO_ENABLED=0 "$GO" run ./cmd/tracker seed
}

cmd_backup() {
  CGO_ENABLED=0 "$GO" run ./cmd/tracker backup "$@"
}

cmd_help() {
  printf '%s\n' \
    'Usage: scripts/dev.sh <command> [args]' \
    '' \
    'Commands:' \
    '  install  Install the dev-only Node deps (Tailwind CLI). Run once after clone.' \
    '  css      Build the minified Tailwind bundle into internal/ui/static/app.css.' \
    '  build    Build the CSS bundle, then compile the Go binary into bin/.' \
    '  test     Run the Go test suite.' \
    '  run      Build the CSS, then serve the app via go run. Default command.' \
    '  seed     Apply the embedded Top 100 seed (idempotent) via go run.' \
    '  backup   Snapshot the DB via go run: backup [-uploads] <dest>.' \
    '  help     Print this help.'
}

case "${1:-run}" in
  install)        cmd_install ;;
  css)            cmd_css ;;
  build)          cmd_build ;;
  test)           cmd_test ;;
  run)            cmd_run ;;
  seed)           cmd_seed ;;
  backup)         shift; cmd_backup "$@" ;;
  help|-h|--help) cmd_help ;;
  *)              printf 'unknown command: %s\n\n' "$1" >&2; cmd_help; exit 1 ;;
esac
