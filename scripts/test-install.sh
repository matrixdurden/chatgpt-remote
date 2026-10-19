#!/usr/bin/env bash
set -Eeuo pipefail
REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
TEMP="$(mktemp -d)"
trap 'rm -rf -- "$TEMP"' EXIT
mkdir -p "$TEMP/home" "$TEMP/project" "$REPO/bin"
export HOME="$TEMP/home"
export XDG_CONFIG_HOME="$HOME/.config"
export CHATGPT_REMOTE_NO_SYSTEMD=1
(cd "$REPO" && GOCACHE="$TEMP/cache" GOPATH="$TEMP/gopath" go build -o "$REPO/bin/chatgpt-remote" ./cmd/remote)
bash "$REPO/scripts/install.sh" --root "$TEMP/project"
[[ -f "$HOME/.config/chatgpt-remote/config.json" ]]
[[ -f "$HOME/.config/chatgpt-remote/token" ]]
[[ -f "$HOME/.local/state/chatgpt-remote/server.pid" ]]
bash "$HOME/.local/share/chatgpt-remote/uninstall.sh"
[[ ! -e "$HOME/.local/bin/chatgpt-remote" ]]
[[ ! -e "$HOME/.config/chatgpt-remote" ]]
[[ ! -e "$HOME/.local/state/chatgpt-remote" ]]
[[ ! -e "$HOME/.local/share/chatgpt-remote" ]]
[[ -d "$TEMP/project" ]]
echo 'Install/uninstall integration test passed.'
