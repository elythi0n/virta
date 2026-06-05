#!/usr/bin/env bash
# Coverage gate (ADR-024): per-package thresholds — every real package must clear the
# overall floor (80%); foundational "core" packages must clear the core floor (90%).
#
# Per-package (not a global average) so one big well-tested package can't mask a thin one.
# Exempt: test-helper packages (*test conformance suites) and cmd/* skeletons — they're
# test infrastructure / entrypoints, covered indirectly or by integration later.
set -euo pipefail

OVERALL_FLOOR="${OVERALL_FLOOR:-80}"
CORE_FLOOR="${CORE_FLOOR:-90}"

# Core package paths relative to internal/ (those that exist are held to CORE_FLOOR).
CORE=(buildinfo clock pipeline platform secrets store store/sqlite store/postgres engine intel/usage)

echo "running tests with coverage…"
go test -covermode=atomic -coverprofile=coverage.out ./... 2>&1 | tee coverage.txt

fail=0
while IFS= read -r line; do
  case "$line" in *"coverage:"*) ;; *) continue ;; esac
  pkg="$(awk '{for(i=1;i<=NF;i++) if($i ~ /^github\.com\//){print $i; exit}}' <<<"$line")"
  cov="$(sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p' <<<"$line")"
  [ -z "$pkg" ] && continue
  [ -z "$cov" ] && continue
  case "$pkg" in
    *test) continue ;;       # platformtest/storetest/secretstest conformance suites
    */cmd/*) continue ;;     # binary entrypoints
  esac
  rel="${pkg#*/internal/}"
  floor="$OVERALL_FLOOR"
  for c in "${CORE[@]}"; do [ "$rel" = "$c" ] && floor="$CORE_FLOOR"; done
  if awk -v t="$cov" -v f="$floor" 'BEGIN{exit (t+0 < f+0)?0:1}'; then
    printf 'FAIL: %-55s %5s%% < %s%%\n' "$pkg" "$cov" "$floor"; fail=1
  else
    printf 'ok:   %-55s %5s%% (>= %s%%)\n' "$pkg" "$cov" "$floor"
  fi
done < coverage.txt

[ "$fail" -eq 0 ] || { echo "coverage floors not met"; exit 1; }
echo "✓ coverage floors met (per-package)"
