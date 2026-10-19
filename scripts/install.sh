#!/usr/bin/env bash
set -euo pipefail

ROOT=""
SERVICE=0
while (($#)); do
  case "$1" in
    --root) ROOT="${2:?--root needs a path}"; shift 2 ;;
    --service) SERVICE=1; shift ;;
    *) echo "Unknown option: $1" >&2; echo 'Usage: ./scripts/install.sh --root /my/project [--service]' >&2; exit 2 ;;
  esac
done
if [[ -z "$ROOT" ]]; then
  echo 'Usage: ./scripts/install.sh --root /my/project [--service]' >&2; exit 2
fi
PROJECT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
BIN_DIR="$HOME/.local/bin"
mkdir -p "$BIN_DIR"
if [[ -x "$PROJECT_DIR/bin/chatgpt-remote" ]]; then
  # Precompiled installation bundle; no Go toolchain required.
  cp "$PROJECT_DIR/bin/chatgpt-remote" "$BIN_DIR/chatgpt-remote"
elif command -v go >/dev/null 2>&1; then
  (cd "$PROJECT_DIR" && go build -trimpath -o "$BIN_DIR/chatgpt-remote" ./cmd/remote)
else
  echo 'Go 1.23+ is required for source builds. Alternatively use a precompiled Linux bundle.' >&2; exit 1
fi
chmod 700 "$BIN_DIR/chatgpt-remote"
CONFIG_PATH="$("$BIN_DIR/chatgpt-remote" config-path)"
if [[ ! -e "$CONFIG_PATH" ]]; then
  "$BIN_DIR/chatgpt-remote" setup --root "$ROOT"
else
  echo 'Existing configuration left unchanged; inspect it with: chatgpt-remote status'
fi
if [[ "$SERVICE" -eq 1 ]]; then
  if ! command -v systemctl >/dev/null 2>&1 || ! systemctl --user status >/dev/null 2>&1; then
    echo 'systemd user manager unavailable; binary installed, service skipped.' >&2
  else
    SYSTEMD_DIR="$HOME/.config/systemd/user"; mkdir -p "$SYSTEMD_DIR"
    cat > "$SYSTEMD_DIR/chatgpt-remote.service" <<UNIT
[Unit]
Description=ChatGPT Remote MCP server
After=network.target

[Service]
Type=simple
ExecStart=%h/.local/bin/chatgpt-remote serve
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=full

[Install]
WantedBy=default.target
UNIT
    systemctl --user daemon-reload
    systemctl --user enable --now chatgpt-remote.service
    echo 'systemd user service enabled (authenticated local mode).'
  fi
fi
echo
printf 'Installed: %s\n' "$BIN_DIR/chatgpt-remote"
echo 'Start: ~/.local/bin/chatgpt-remote serve'
echo 'Check: ~/.local/bin/chatgpt-remote status'
echo 'See README.md for tunnel setup and ChatGPT plan limitations.'
