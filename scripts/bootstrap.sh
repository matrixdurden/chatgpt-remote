#!/usr/bin/env bash
# Download + checksum-verify the latest published release archive.
# No source-code execution from the network and no sudo is required.
set -euo pipefail
if (($# != 2)) || [[ "$1" != "--root" ]]; then
  echo 'Usage: bash scripts/bootstrap.sh --root /path/to/project' >&2; exit 2
fi
ROOT="$2"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64) ARCH=amd64 ;;
  aarch64) ARCH=arm64 ;;
  *) echo "Unsupported architecture: $ARCH" >&2; exit 1 ;;
esac
for cmd in curl sha256sum tar; do command -v "$cmd" >/dev/null || { echo "Missing $cmd" >&2; exit 1; }; done
BASE='https://github.com/matrixdurden/chatgpt-remote/releases/latest/download'
TMP="$(mktemp -d)"; trap 'rm -rf "$TMP"' EXIT
FILE="chatgpt-remote-linux-$ARCH.tar.gz"
curl --fail --location --silent --show-error "$BASE/$FILE" -o "$TMP/$FILE"
curl --fail --location --silent --show-error "$BASE/SHA256SUMS" -o "$TMP/SHA256SUMS"
(cd "$TMP" && grep " $FILE$" SHA256SUMS | sha256sum --check --status) || { echo 'Checksum verification failed' >&2; exit 1; }
tar -xzf "$TMP/$FILE" -C "$TMP"
"$TMP/chatgpt-remote-linux-$ARCH/scripts/install.sh" --root "$ROOT"
