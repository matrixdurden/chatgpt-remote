#!/usr/bin/env bash
# Delete only files installed by ChatGPT Remote, not project files.
set -Eeuo pipefail
BIN="$HOME/.local/bin/chatgpt-remote"
DATA_DIR="$HOME/.local/share/chatgpt-remote"
STATE_DIR="$HOME/.local/state/chatgpt-remote"
CONFIG_DIR="${XDG_CONFIG_HOME:-$HOME/.config}/chatgpt-remote"
UNIT="$HOME/.config/systemd/user/chatgpt-remote.service"
if [[ -f "$UNIT" ]]; then
  if command -v systemctl >/dev/null 2>&1; then
    systemctl --user disable --now chatgpt-remote.service >/dev/null 2>&1 || true
  fi
  rm -f -- "$UNIT"
  if command -v systemctl >/dev/null 2>&1; then systemctl --user daemon-reload >/dev/null 2>&1 || true; fi
fi
if [[ -f "$STATE_DIR/server.pid" ]]; then
  PID="$(cat "$STATE_DIR/server.pid")"
  if [[ "$PID" =~ ^[0-9]+$ && -e "/proc/$PID/exe" ]]; then
    PROC_EXE="$(readlink "/proc/$PID/exe" || true)"
    if [[ "$PROC_EXE" == "$BIN" || "$PROC_EXE" == "$BIN (deleted)" ]]; then kill "$PID" 2>/dev/null || true; fi
  fi
fi
rm -f -- "$BIN"
rm -rf -- "$STATE_DIR" "$CONFIG_DIR"
if [[ -f "$DATA_DIR/created-workspace" ]]; then
  WORKSPACE="$(cat "$DATA_DIR/created-workspace")"
  if [[ "$WORKSPACE" == "$HOME/chatgpt-remote-workspace" && ! -L "$WORKSPACE" ]]; then
    rmdir -- "$WORKSPACE" 2>/dev/null || echo "Preserved nonempty workspace: $WORKSPACE"
  fi
fi
rm -rf -- "$DATA_DIR"
for d in "$HOME/.local/bin" "$HOME/.local/state" "$HOME/.local/share" "$HOME/.local" "$HOME/.config/systemd/user" "$HOME/.config/systemd" "$HOME/.config"; do
  rmdir -- "$d" 2>/dev/null || true
done
echo 'ChatGPT Remote fully removed: binary, service, settings, token and logs.'
echo 'Projects were not touched; disconnect an external tunnel/plugin separately.'
