#!/usr/bin/env bash
# One-line bootstrap for Linux and WSL2.
set -Eeuo pipefail
umask 077
REPO='matrixdurden/chatgpt-remote'
ROOT=''
NO_START=0
CREATED_WORKSPACE=0
while (($#)); do
  case "$1" in
    --root) ROOT="${2:?--root requires a directory}"; shift 2 ;;
    --no-start) NO_START=1; shift ;;
    -h|--help) printf 'Usage: curl -fsSL https://raw.githubusercontent.com/%s/main/install.sh | bash\n' "$REPO"; exit 0 ;;
    *) printf 'Unknown option: %s\n' "$1" >&2; exit 2 ;;
  esac
done
[[ "$(uname -s)" == 'Linux' ]] || { echo 'Use Linux or Ubuntu inside WSL2 (not PowerShell).' >&2; exit 1; }
case "$(uname -m)" in x86_64) ARCH=amd64;; aarch64) ARCH=arm64;; *) echo 'Supported CPUs: x86_64 and aarch64.' >&2; exit 1;; esac
for tool in curl tar sha256sum mktemp; do command -v "$tool" >/dev/null || { echo "Missing: $tool" >&2; exit 1; }; done
if [[ -z "$ROOT" ]]; then
  if [[ -d "$HOME/projects" ]]; then ROOT="$HOME/projects"
  else
    ROOT="$HOME/chatgpt-remote-workspace"
    if [[ ! -d "$ROOT" ]]; then mkdir -p "$ROOT"; CREATED_WORKSPACE=1; fi
  fi
fi
[[ -d "$ROOT" ]] || { echo "Directory missing: $ROOT" >&2; exit 1; }
ROOT="$(cd "$ROOT" && pwd -P)"
TEMP="$(mktemp -d)"
trap 'rm -rf -- "$TEMP"' EXIT
BASE="https://github.com/$REPO/releases/latest/download"
ARCHIVE="chatgpt-remote-linux-$ARCH.tar.gz"
STAGING=''
if curl -fLsS "$BASE/SHA256SUMS" -o "$TEMP/SHA256SUMS" 2>/dev/null; then
  if curl -fLsS "$BASE/$ARCHIVE" -o "$TEMP/$ARCHIVE"; then
    (cd "$TEMP" && grep -F " $ARCHIVE" SHA256SUMS | sha256sum --check --status) || { echo 'Release checksum mismatch.' >&2; exit 1; }
    tar -xzf "$TEMP/$ARCHIVE" -C "$TEMP"
    STAGING="$TEMP/chatgpt-remote-linux-$ARCH"
    echo 'Installing checksum-verified release.'
  fi
fi
if [[ -z "$STAGING" ]]; then
  echo 'No published release: compiling source from GitHub.'
  curl -fLsS "https://github.com/$REPO/archive/refs/heads/main.tar.gz" -o "$TEMP/source.tar.gz"
  tar -xzf "$TEMP/source.tar.gz" -C "$TEMP"
  STAGING="$TEMP/chatgpt-remote-main"
  [[ -f "$STAGING/go.mod" ]] || { echo 'Invalid source archive.' >&2; exit 1; }
  GO_BIN=''
  if command -v go >/dev/null 2>&1 && go version | grep -Eq 'go1\.(2[3-9]|[3-9][0-9])\.'; then GO_BIN="$(command -v go)"; fi
  if [[ -z "$GO_BIN" ]]; then
    echo 'Temporarily downloading Go (not installing it permanently).'
    case "$ARCH" in
      amd64) HASH=d3847fef834e9db11bf64e3fb34db9c04db14e068eeb064f49af747010454f90 ;;
      arm64) HASH=52ce172f96e21da53b1ae9079808560d49b02ac86cecfa457217597f9bc28ab3 ;;
    esac
    TOOLCHAIN="go1.23.12.linux-$ARCH.tar.gz"
    curl -fLsS "https://go.dev/dl/$TOOLCHAIN" -o "$TEMP/$TOOLCHAIN"
    printf '%s  %s\n' "$HASH" "$TEMP/$TOOLCHAIN" | sha256sum --check --status || { echo 'Toolchain checksum mismatch.' >&2; exit 1; }
    mkdir -p "$TEMP/toolchain"
    tar -xzf "$TEMP/$TOOLCHAIN" -C "$TEMP/toolchain"
    GO_BIN="$TEMP/toolchain/go/bin/go"
  fi
  mkdir -p "$STAGING/bin" "$TEMP/gocache"
  (cd "$STAGING" && GOTOOLCHAIN=local GOCACHE="$TEMP/gocache" GOPATH="$TEMP/gopath" "$GO_BIN" build -trimpath -o bin/chatgpt-remote ./cmd/remote)
fi
ARGS=(--root "$ROOT")
[[ "$NO_START" -eq 1 ]] && ARGS+=(--no-start)
CHATGPT_REMOTE_CREATED_WORKSPACE="$CREATED_WORKSPACE" bash "$STAGING/scripts/install.sh" "${ARGS[@]}"
