#!/usr/bin/env bash
set -euo pipefail

if command -v cleo >/dev/null 2>&1; then
  echo "cleo found: $(command -v cleo)"
  cleo version
  exit 0
fi

if [[ -n "${CLEO_INSTALL_COMMAND:-}" ]]; then
  echo "cleo not found; running CLEO_INSTALL_COMMAND"
  eval "$CLEO_INSTALL_COMMAND"
fi

if ! command -v cleo >/dev/null 2>&1; then
  echo "error: cleo CLI is required but not installed on this runner."
  echo "set CLEO_INSTALL_COMMAND secret/variable in GitHub Actions, for example a curl installer command."
  exit 1
fi

cleo version
