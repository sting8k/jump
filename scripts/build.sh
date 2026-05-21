#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"

export GOWORK=${GOWORK:-"$ROOT/go.work"}

if [[ -z "${VERSION:-}" ]]; then
  if tag=$(git describe --tags --match 'v[0-9]*' --abbrev=0 2>/dev/null); then
    VERSION=${tag#v}
  else
    VERSION=$(node -e "process.stdout.write(require('./package.json').version || '')")
  fi
fi
if [[ -z "$VERSION" ]]; then
  printf '[build] VERSION is empty and package.json has no version\n' >&2
  exit 1
fi
export VERSION

WEB_DIST="apps/jump-web/dist"
WEB_EMBED="services/jumpd/cmd/jumpd/web"

printf '[build] protocol\n'
pnpm --filter @jump/protocol build

printf '[build] web\n'
pnpm --filter @jump/web build

printf '[build] sync embedded web assets\n'
mkdir -p "$WEB_EMBED"
find "$WEB_EMBED" -mindepth 1 -maxdepth 1 ! -name .gitignore -exec rm -rf {} +
cp -R "$WEB_DIST"/. "$WEB_EMBED"/

printf '[build] Go binaries\n'
mkdir -p bin
go build -ldflags "-X main.version=$VERSION" -o bin/jump ./cli/jump/cmd/jump
go build -ldflags "-X main.version=$VERSION" -o bin/jumpd ./services/jumpd/cmd/jumpd
go build -o bin/jump-relayd ./services/jump-relayd/cmd/jump-relayd

printf '[build] wrote bin/jump, bin/jumpd, and bin/jump-relayd (VERSION=%s)\n' "$VERSION"
