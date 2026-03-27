#!/usr/bin/env bash
set -euo pipefail

REPO="kaka-ruto/cleo"
BIN_NAME="cleo"
VERSION="${CLEO_VERSION:-latest}"

if command -v cleo >/dev/null 2>&1; then
  echo "cleo found: $(command -v cleo)"
  cleo version
  exit 0
fi

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch="amd64" ;;
  arm64|aarch64) arch="arm64" ;;
esac

if [[ "$VERSION" == "latest" ]]; then
  base_url="https://github.com/${REPO}/releases/latest/download"
else
  base_url="https://github.com/${REPO}/releases/download/${VERSION}"
fi

tmpdir="$(mktemp -d)"
trap 'rm -rf "$tmpdir"' EXIT

candidates=(
  "${BIN_NAME}-${os}-${arch}"
  "${BIN_NAME}_${os}_${arch}"
  "${BIN_NAME}-${os}-${arch}.tar.gz"
  "${BIN_NAME}_${os}_${arch}.tar.gz"
  "${BIN_NAME}-${os}-${arch}.zip"
  "${BIN_NAME}_${os}_${arch}.zip"
)

downloaded=""
for asset in "${candidates[@]}"; do
  url="${base_url}/${asset}"
  if curl -fsSL "$url" -o "${tmpdir}/${asset}"; then
    downloaded="${tmpdir}/${asset}"
    break
  fi
done

if [[ -z "$downloaded" ]]; then
  echo "cleo release binary not found for ${os}/${arch}; falling back to go install"
  go install github.com/kaka-ruto/cleo/cmd/cleo@latest
  export PATH="$PATH:$(go env GOPATH)/bin"
  if ! command -v cleo >/dev/null 2>&1; then
    echo "error: failed to install cleo from GitHub source using go install" >&2
    exit 1
  fi
  cleo version
  exit 0
fi

install_dir="/usr/local/bin"
if [[ -n "${RUNNER_TEMP:-}" ]]; then
  install_dir="${RUNNER_TEMP}/bin"
  mkdir -p "$install_dir"
  export PATH="$install_dir:$PATH"
fi

if [[ "$downloaded" == *.tar.gz ]]; then
  tar -xzf "$downloaded" -C "$tmpdir"
  bin="$(find "$tmpdir" -type f -name "${BIN_NAME}" | head -n1)"
elif [[ "$downloaded" == *.zip ]]; then
  unzip -o "$downloaded" -d "$tmpdir" >/dev/null
  bin="$(find "$tmpdir" -type f -name "${BIN_NAME}" | head -n1)"
else
  bin="$downloaded"
fi

if [[ -z "${bin:-}" || ! -f "$bin" ]]; then
  echo "error: downloaded cleo asset does not contain a usable binary" >&2
  exit 1
fi

chmod +x "$bin"
if [[ -w "$install_dir" ]]; then
  mv "$bin" "${install_dir}/${BIN_NAME}"
else
  sudo mv "$bin" "${install_dir}/${BIN_NAME}"
fi

if ! command -v cleo >/dev/null 2>&1; then
  echo "error: cleo CLI is required but not installed on this runner."
  exit 1
fi

cleo version
