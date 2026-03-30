#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <version-tag>" >&2
  echo "example: $0 v0.2.0" >&2
  exit 1
fi

VERSION="$1"
if [[ ! "$VERSION" =~ ^v[0-9]+\.[0-9]+\.[0-9]+([-.][0-9A-Za-z.]+)?$ ]]; then
  echo "invalid version tag: $VERSION" >&2
  exit 1
fi

REPO="cafaye/cafaye-cli"
FORMULA_PATH="${FORMULA_PATH:-Formula/cafaye.rb}"
URL="https://github.com/${REPO}/archive/refs/tags/${VERSION}.tar.gz"

if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required" >&2
  exit 1
fi

if command -v shasum >/dev/null 2>&1; then
  SHA256="$(curl -fsSL "$URL" | shasum -a 256 | awk '{print $1}')"
elif command -v sha256sum >/dev/null 2>&1; then
  SHA256="$(curl -fsSL "$URL" | sha256sum | awk '{print $1}')"
else
  echo "shasum or sha256sum is required" >&2
  exit 1
fi

if [[ ! -f "$FORMULA_PATH" ]]; then
  echo "formula not found: $FORMULA_PATH" >&2
  echo "tip: formula source-of-truth is in cafaye/homebrew-cafaye-cli" >&2
  exit 1
fi

perl -0777 -i -pe "s#url \\\"https://github.com/${REPO}/archive/refs/tags/[^\\\"]+\\.tar\\.gz\\\"#url \\\"https://github.com/${REPO}/archive/refs/tags/${VERSION}.tar.gz\\\"#g; s#sha256 \\\"[0-9a-f]{64}\\\"#sha256 \\\"${SHA256}\\\"#g" "$FORMULA_PATH"

echo "Updated $FORMULA_PATH"
echo "  version: $VERSION"
echo "  sha256:  $SHA256"
