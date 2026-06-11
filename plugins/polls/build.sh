#!/usr/bin/env bash
# Packages the Polls plugin into dist/polls.zip with virta-plugin.json and gui/ at the
# zip root, ready to install in Virta.
set -euo pipefail
cd "$(dirname "$0")"

OUT="dist/polls.zip"
mkdir -p dist
rm -f "$OUT"
zip -qr "$OUT" virta-plugin.json gui -x "dist/*" -x "build.sh"
echo "built $OUT"
unzip -l "$OUT"
