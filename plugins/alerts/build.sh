#!/usr/bin/env bash
# Packages the Alerts plugin into dist/alerts.zip with virta-plugin.json and gui/ at the
# zip root.
set -euo pipefail
cd "$(dirname "$0")"

OUT="dist/alerts.zip"
mkdir -p dist
rm -f "$OUT"

# -X drops platform extra fields for reproducible-ish archives. __virta.js is host-injected at
# serve time and must never ship in the package (excluded defensively in case a dev copy exists).
zip -qrX "$OUT" virta-plugin.json gui -x "gui/__virta.js"

echo "built $OUT"
unzip -l "$OUT"
