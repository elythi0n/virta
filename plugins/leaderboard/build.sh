#!/usr/bin/env bash
# Packages the Leaderboard plugin into dist/leaderboard.zip with virta-plugin.json and gui/ at the
# archive root, ready to install from Virta's Plugins panel.
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist
rm -f dist/leaderboard.zip
zip -qr dist/leaderboard.zip virta-plugin.json gui -x "dist/*" -x "build.sh"
echo "built dist/leaderboard.zip"
unzip -l dist/leaderboard.zip
