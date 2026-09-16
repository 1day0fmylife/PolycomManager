#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"

VERSION="${VERSION:-dev}"

if ! command -v make >/dev/null 2>&1; then
  echo "ERROR: make is required" >&2
  exit 1
fi

make VERSION="$VERSION" release

echo "Done:"
ls -lh dist/
