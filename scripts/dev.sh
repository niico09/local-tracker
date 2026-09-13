#!/usr/bin/env bash
#
# Local dev driver for local-tracker.
#
# Mirrors the Makefile targets so `make` is not required. Needs `go` and `npx`
# on PATH; intended for Git Bash or any POSIX shell.
#
# Usage:
#   scripts/dev.sh <command> [args]
#
# Commands:
#   install  Install the dev-only Node deps (Tailwind CLI). Run once after clone.
#   css      Build the minified Tailwind bundle into internal/ui/static/app.css.
#   build    Build the CSS bundle, then compile the Go binary into bin/.
#   test     Run the Go test suite.
#   run      Build, then serve the app. This is the default command.
#   seed     Build if needed, then apply the embedded Top 100 seed (idempotent).
#   backup   Build if needed, then snapshot the DB: backup [-uploads] <dest>.
#   help     Print this help.
#
set -euo pipefail

# Resolve paths from the script location so it works from any directory.
ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

GO="${GO:-go}"

# `go build -o bin/tracker` appends .exe on Windows, so resolve whichever
# name the toolchain actually produced instead of assuming one.
tracker_bin() {
  if [[ -f bin/tracker.exe ]]; then
    printf '%s' "bin/tracker.exe"
  else
    printf '%s' "bin/tracker"
  fi
}

ensure_built() {
  [[ -f bin/tracker || -f bin/tracker.exe ]] || cmd_build
}

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

cmd_run() {
  cmd_build
  "./$(tracker_bin)" serve
}

cmd_seed() {
  ensure_built
  "./$(tracker_bin)" seed
}

cmd_backup() {
  ensure_built
  "./$(tracker_bin)" backup "$@"
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
    '  run      Build, then serve the app. This is the default command.' \
    '  seed     Build if needed, then apply the embedded Top 100 seed (idempotent).' \
    '  backup   Build if needed, then snapshot the DB: backup [-uploads] <dest>.' \
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
