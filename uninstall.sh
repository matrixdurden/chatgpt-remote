#!/usr/bin/env bash
set -Eeuo pipefail
FILE="$HOME/.local/share/chatgpt-remote/uninstall.sh"
if [[ -f "$FILE" ]]; then
  bash "$FILE"
else
  echo 'No local ChatGPT Remote installation detected.'
  echo 'For older installations use scripts/uninstall.sh in the checkout.'
fi
