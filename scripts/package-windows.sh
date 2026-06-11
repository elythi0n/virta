#!/usr/bin/env bash
# Windows packaging — mirrors `make app` + `make app-inno` for Windows CI runners, which
# have Git Bash but no GNU make (run with `shell: bash` in the workflow, or from Git Bash
# locally). Produces dist/VirtaSetup-<VERSION>.exe. Keep in sync with the Makefile targets.
set -euo pipefail
cd "$(dirname "$0")/.."

VERSION="${VERSION:-dev}"
MODULE=github.com/elythi0n/virta
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo none)
DATE=$(date -u +%Y-%m-%dT%H:%M:%SZ)
LDFLAGS="-s -w \
  -X $MODULE/internal/buildinfo.Version=$VERSION \
  -X $MODULE/internal/buildinfo.Commit=$COMMIT \
  -X $MODULE/internal/buildinfo.Date=$DATE"

# Inno Setup comes preinstalled on GitHub-hosted Windows runners but is not on PATH.
command -v iscc >/dev/null 2>&1 || export PATH="$PATH:/c/Program Files (x86)/Inno Setup 6"
command -v iscc >/dev/null 2>&1 || { echo "iscc not found (choco install innosetup)"; exit 1; }

# Web UI, staged for both the desktop shell and virtad's go:embed.
(cd frontends/web && npm install && npm run build)
find frontends/desktop/assets -mindepth 1 ! -name .gitkeep -delete
cp -r frontends/web/dist/. frontends/desktop/assets/
find internal/webui/dist -mindepth 1 ! -name .gitkeep -delete
cp -r frontends/web/dist/. internal/webui/dist/

# virtad embedded inside the desktop binary, then the desktop shell itself.
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o frontends/desktop/bin/virtad.exe ./cmd/virtad
mkdir -p frontends/desktop/build/bin dist
(cd frontends/desktop && go mod tidy && go build -ldflags '-s -w' -o build/bin/virta.exe .)

# Standalone extras the installer ships next to the app.
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o dist/virtad.exe ./cmd/virtad
CGO_ENABLED=0 go build -ldflags "$LDFLAGS" -o dist/virta-tui.exe ./cmd/virta-tui

# ISCC only accepts slash-prefixed options; the env var stops MSYS from rewriting /D as a path.
MSYS2_ARG_CONV_EXCL="/D" iscc "/DAppVersion=$VERSION" packaging/virta.iss
echo "✓ installer: dist/VirtaSetup-$VERSION.exe"
