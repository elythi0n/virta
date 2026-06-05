#!/usr/bin/env bash
# Coverage gate (ADR-024): overall >= 80%, core packages >= 90%.
# Lenient about packages that don't exist yet — core floors apply only to
# core packages that are actually present.
set -euo pipefail

OVERALL_FLOOR="${OVERALL_FLOOR:-80}"
CORE_FLOOR="${CORE_FLOOR:-90}"

# Core package suffixes (under internal/). Platform normalizers added as they appear.
CORE_PKGS=(pipeline engine "store/sqlite" "store/postgres" "secrets/keychain" \
           "secrets/agevault" "intel/usage" buildinfo clock)

echo "running tests with coverage…"
# Per-package coverage lines ("coverage: NN% of statements") + a merged profile.
go test -covermode=atomic -coverprofile=coverage.out ./... 2>&1 | tee coverage.txt

# --- overall ---
if [ -s coverage.out ]; then
  total="$(go tool cover -func=coverage.out | awk '/^total:/ {gsub(/%/,"",$NF); print $NF}')"
else
  total="0"
fi
total="${total:-0}"
printf 'overall coverage: %s%% (floor %s%%)\n' "$total" "$OVERALL_FLOOR"

# If there are no covered statements at all yet, the floor is vacuously met.
have_code="$(grep -c 'coverage: [0-9]' coverage.txt || true)"
if [ "${have_code:-0}" -gt 0 ]; then
  awk -v t="$total" -v f="$OVERALL_FLOOR" 'BEGIN{ exit (t+0 < f+0) ? 1 : 0 }' || {
    echo "FAIL: overall coverage ${total}% < ${OVERALL_FLOOR}%"; exit 1; }
fi

# --- core packages (only those that exist & have tests) ---
fail=0
while IFS= read -r line; do
  pkg="$(awk '{for(i=1;i<=NF;i++) if($i ~ /^github\.com\//){print $i; exit}}' <<<"$line")"
  cov="$(sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p' <<<"$line")"
  [ -z "$pkg" ] && continue
  [ -z "$cov" ] && continue
  for c in "${CORE_PKGS[@]}"; do
    if [[ "$pkg" == *"/internal/$c" ]]; then
      if awk -v t="$cov" -v f="$CORE_FLOOR" 'BEGIN{ exit (t+0 < f+0) ? 0 : 1 }'; then
        printf 'FAIL core: %s %s%% < %s%%\n' "$pkg" "$cov" "$CORE_FLOOR"; fail=1
      fi
    fi
  done
done < <(grep 'coverage: [0-9]' coverage.txt || true)

[ "$fail" -eq 0 ] || exit 1
echo "✓ coverage floors met"
