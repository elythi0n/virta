#!/usr/bin/env bash
# Packages the VOD Replay plugin into dist/vod-replay.zip with virta-plugin.json and gui/ at the
# archive root, ready to install from Virta's Plugins panel.
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist
rm -f dist/vod-replay.zip
zip -qr dist/vod-replay.zip virta-plugin.json gui -x "dist/*" -x "build.sh"
echo "built dist/vod-replay.zip"
unzip -l dist/vod-replay.zip
