#!/usr/bin/env bash
set -euo pipefail

REPO="cafaye/cafaye-cli"
BIN_NAME="cafaye"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
VERSION="${VERSION:-latest}"
LOCAL_BINARY_PATH="${LOCAL_BINARY_PATH:-}"

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"

case "$os" in
  darwin|linux) ;;
  *)
    echo "unsupported OS: $os (expected darwin or linux)" >&2
    exit 1
    ;;
esac

case "$arch" in
  x86_64|amd64)
    arch="amd64"
    ;;
  arm64|aarch64)
    arch="arm64"
    ;;
  *)
    echo "unsupported architecture: $arch (expected amd64 or arm64)" >&2
    exit 1
    ;;
esac

asset="${BIN_NAME}-${os}-${arch}"
checksums_asset="SHA256SUMS"

if [[ "$VERSION" == "latest" ]]; then
  url="https://github.com/${REPO}/releases/latest/download/${asset}"
  checksums_url="https://github.com/${REPO}/releases/latest/download/${checksums_asset}"
else
  url="https://github.com/${REPO}/releases/download/${VERSION}/${asset}"
  checksums_url="https://github.com/${REPO}/releases/download/${VERSION}/${checksums_asset}"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

out="${tmpdir}/${BIN_NAME}"
checksums_out="${tmpdir}/${checksums_asset}"

if [[ -n "$LOCAL_BINARY_PATH" ]]; then
  if [[ ! -f "$LOCAL_BINARY_PATH" ]]; then
    echo "LOCAL_BINARY_PATH does not exist: $LOCAL_BINARY_PATH" >&2
    exit 1
  fi
  cp "$LOCAL_BINARY_PATH" "$out"
  chmod +x "$out"
else
  echo "Downloading ${url}"
  curl -fsSL "$url" -o "$out"
  chmod +x "$out"

  echo "Downloading ${checksums_url}"
  curl -fsSL "$checksums_url" -o "$checksums_out"

  expected="$(awk -v file="$asset" '$2 == file { print $1 }' "$checksums_out")"
  if [[ -z "$expected" ]]; then
    echo "missing checksum for ${asset} in ${checksums_asset}" >&2
    exit 1
  fi

  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "$out" | awk '{print $1}')"
  elif command -v sha256sum >/dev/null 2>&1; then
    actual="$(sha256sum "$out" | awk '{print $1}')"
  else
    echo "shasum or sha256sum is required for checksum verification" >&2
    exit 1
  fi

  if [[ "$actual" != "$expected" ]]; then
    echo "checksum mismatch for ${asset}" >&2
    echo "expected: $expected" >&2
    echo "actual:   $actual" >&2
    exit 1
  fi
fi

mkdir -p "$INSTALL_DIR"
if [[ -w "$INSTALL_DIR" ]]; then
  mv "$out" "${INSTALL_DIR}/${BIN_NAME}"
else
  sudo mv "$out" "${INSTALL_DIR}/${BIN_NAME}"
fi

echo "Installed ${BIN_NAME} to ${INSTALL_DIR}/${BIN_NAME}"
"${INSTALL_DIR}/${BIN_NAME}" --help >/dev/null
"${INSTALL_DIR}/${BIN_NAME}" version 2>/dev/null || true
