#!/usr/bin/env bash
set -euo pipefail
if command -v systemctl >/dev/null 2>&1; then
  systemctl --user disable --now chatgpt-remote.service >/dev/null 2>&1 || true
fi
rm -f "$HOME/.config/systemd/user/chatgpt-remote.service"
if command -v systemctl >/dev/null 2>&1; then
  systemctl --user daemon-reload >/dev/null 2>&1 || true
fi
rm -f "$HOME/.local/bin/chatgpt-remote"
echo 'Binary and service removed.'
echo "Configuration and token intentionally retained at ~/.config/chatgpt-remote/"
echo 'Remove that folder manually only if you want to delete settings and token.'
