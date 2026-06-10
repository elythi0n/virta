#!/bin/sh
# Virta installer for Linux: downloads the latest AppImage release and puts `virta` on your PATH.
# Usage: curl -fsSL https://virta.lol/install.sh | sh
set -eu

REPO="elythi0n/virta"
ASSET="Virta-x86_64.AppImage"
URL="https://github.com/$REPO/releases/latest/download/$ASSET"
BIN_DIR="${VIRTA_INSTALL_DIR:-$HOME/.local/bin}"
DEST="$BIN_DIR/virta"

case "$(uname -s)" in
  Linux) ;;
  *) echo "This installer is Linux-only — grab a build from https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
case "$(uname -m)" in
  x86_64 | amd64) ;;
  *) echo "Prebuilt AppImages are x86_64-only for now — see https://github.com/$REPO/releases" >&2; exit 1 ;;
esac
command -v curl >/dev/null 2>&1 || { echo "curl is required" >&2; exit 1; }

echo "Downloading $ASSET…"
mkdir -p "$BIN_DIR"
tmp=$(mktemp)
trap 'rm -f "$tmp"' EXIT
curl -fL --progress-bar -o "$tmp" "$URL"
chmod +x "$tmp"
mv "$tmp" "$DEST"
trap - EXIT

echo "✓ Installed to $DEST"
case ":$PATH:" in
  *":$BIN_DIR:"*) echo "Run: virta" ;;
  *) echo "Note: $BIN_DIR is not on your PATH — add it to your shell profile, or run $DEST directly." ;;
esac
