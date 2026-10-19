#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
ROOT=''
NO_START=0
while (($#)); do
  case "$1" in
    --root) ROOT="${2:?--root needs a directory}"; shift 2 ;;
    --no-start) NO_START=1; shift ;;
    *) echo 'Usage: scripts/install.sh --root /project/path [--no-start]' >&2; exit 2 ;;
  esac
done
[[ -n "$ROOT" && -d "$ROOT" ]] || { echo '--root must be an existing directory' >&2; exit 1; }
ROOT="$(cd "$ROOT" && pwd -P)"
SOURCE="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd -P)"
BIN_DIR="$HOME/.local/bin"
DATA_DIR="$HOME/.local/share/chatgpt-remote"
STATE_DIR="$HOME/.local/state/chatgpt-remote"
UNIT_DIR="$HOME/.config/systemd/user"
BIN="$BIN_DIR/chatgpt-remote"
mkdir -p "$BIN_DIR" "$DATA_DIR" "$STATE_DIR"
if [[ -f "$UNIT_DIR/chatgpt-remote.service" ]] && command -v systemctl >/dev/null 2>&1; then
  systemctl --user stop chatgpt-remote.service >/dev/null 2>&1 || true
fi
if [[ -f "$STATE_DIR/server.pid" ]]; then
  PID="$(cat "$STATE_DIR/server.pid")"
  if [[ "$PID" =~ ^[0-9]+$ && -e "/proc/$PID/exe" ]]; then
    PROC_EXE="$(readlink "/proc/$PID/exe" || true)"
    if [[ "$PROC_EXE" == "$BIN" || "$PROC_EXE" == "$BIN (deleted)" ]]; then kill "$PID" 2>/dev/null || true; fi
  fi
  rm -f "$STATE_DIR/server.pid"
fi
TMP_BIN="$BIN.tmp.$$"
trap 'rm -f -- "$TMP_BIN"' EXIT
if [[ -x "$SOURCE/bin/chatgpt-remote" ]]; then
  cp "$SOURCE/bin/chatgpt-remote" "$TMP_BIN"
elif command -v go >/dev/null 2>&1; then
  CACHE="$(mktemp -d)"
  trap 'rm -f -- "$TMP_BIN"; rm -rf -- "$CACHE"' EXIT
  (cd "$SOURCE" && GOTOOLCHAIN=local GOCACHE="$CACHE/gocache" GOPATH="$CACHE/gopath" go build -trimpath -o "$TMP_BIN" ./cmd/remote)
else
  echo 'Missing binary and Go. Use the top-level install.sh.' >&2; exit 1
fi
chmod 700 "$TMP_BIN"
mv -f "$TMP_BIN" "$BIN"
install -m 700 "$SOURCE/scripts/uninstall.sh" "$DATA_DIR/uninstall.sh"
if [[ "${CHATGPT_REMOTE_CREATED_WORKSPACE:-0}" == "1" && "$ROOT" == "$HOME/chatgpt-remote-workspace" ]]; then
  printf '%s\n' "$ROOT" > "$DATA_DIR/created-workspace"
fi
CONFIG="$("$BIN" config-path)"
if [[ ! -f "$CONFIG" ]]; then "$BIN" setup --root "$ROOT"
else echo "Keeping settings: $CONFIG"; fi
if [[ "$NO_START" -eq 0 ]]; then
  USING_SYSTEMD=0
  if [[ "${CHATGPT_REMOTE_NO_SYSTEMD:-0}" != "1" ]] && command -v systemctl >/dev/null 2>&1 && systemctl --user show-environment >/dev/null 2>&1; then
    mkdir -p "$UNIT_DIR"
    cat > "$UNIT_DIR/chatgpt-remote.service" <<'UNIT'
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
    USING_SYSTEMD=1
    echo 'Started using systemd --user.'
  else
    nohup "$BIN" serve > "$STATE_DIR/server.log" 2>&1 < /dev/null &
    printf '%s\n' "$!" > "$STATE_DIR/server.pid"
    sleep 1
    kill -0 "$!" 2>/dev/null || { echo 'Server failed; check ~/.local/state/chatgpt-remote/server.log' >&2; exit 1; }
    echo 'Started in background (no systemd).'
  fi
  # systemd Type=simple may report success before the MCP port is listening.
  # Wait for a successful authenticated handshake instead of racing startup.
  READY=0
  LAST_ERROR=''
  for ((ATTEMPT=1; ATTEMPT<=40; ATTEMPT++)); do
    if OUTPUT="$("$BIN" doctor 2>&1)"; then
      printf '%s\n' "$OUTPUT"
      READY=1
      break
    fi
    LAST_ERROR="$OUTPUT"
    if [[ "$USING_SYSTEMD" -eq 0 && -f "$STATE_DIR/server.pid" ]]; then
      PID="$(cat "$STATE_DIR/server.pid")"
      if ! kill -0 "$PID" 2>/dev/null; then break; fi
    fi
    sleep 0.25
  done
  if [[ "$READY" -ne 1 ]]; then
    echo 'ERROR: MCP did not become ready within the startup window.' >&2
    printf '%s\n' "$LAST_ERROR" >&2
    if [[ "$USING_SYSTEMD" -eq 1 ]]; then
      systemctl --user status chatgpt-remote.service --no-pager -l >&2 || true
      journalctl --user -u chatgpt-remote.service -n 25 --no-pager >&2 || true
    elif [[ -f "$STATE_DIR/server.log" ]]; then
      tail -n 25 "$STATE_DIR/server.log" >&2 || true
    fi
    exit 1
  fi
fi
printf '\nInstalled on Linux%s\n' "$(grep -qi microsoft /proc/sys/kernel/osrelease 2>/dev/null && printf ' (WSL)' || true)"
printf 'Shared directory: %s\n' "$ROOT"
printf 'Status: %s status\n' "$BIN"
printf 'Uninstall: bash %s/uninstall.sh\n' "$DATA_DIR"
echo 'Local MCP URL: http://127.0.0.1:8765/mcp'
echo 'Connecting ChatGPT additionally requires an account-supported plugin and tunnel.'
