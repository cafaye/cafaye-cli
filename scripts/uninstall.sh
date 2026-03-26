#!/usr/bin/env bash
set -euo pipefail

BIN_NAME="cafaye"
INSTALL_DIR="${INSTALL_DIR:-/usr/local/bin}"
PURGE_CONFIG="${PURGE_CONFIG:-false}"

TARGET="${INSTALL_DIR}/${BIN_NAME}"

if [[ -e "$TARGET" ]]; then
  if [[ -w "$TARGET" || -w "$INSTALL_DIR" ]]; then
    rm -f "$TARGET"
  else
    sudo rm -f "$TARGET"
  fi
  echo "Removed ${TARGET}"
else
  echo "No binary found at ${TARGET}"
fi

if [[ "$PURGE_CONFIG" == "true" ]]; then
  CONFIG_DIR="${HOME}/.config/cafaye"
  if [[ -d "$CONFIG_DIR" ]]; then
    rm -rf "$CONFIG_DIR"
    echo "Removed ${CONFIG_DIR}"
  fi
fi

echo "Uninstall complete"
