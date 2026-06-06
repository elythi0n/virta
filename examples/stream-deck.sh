#!/usr/bin/env bash
# Stream Deck "send to all channels" script.
#
# Bind this to a key-press action in the Stream Deck software (or Deckboard / Boatswain).
# Set VIRTA_TOKEN to a token minted with the `send` scope in Settings → Integrations.
# Set VIRTA_ADDR to your daemon's address (printed at startup, or read from the discovery file).
#
# Usage:
#   VIRTA_TOKEN=vk_... VIRTA_ADDR=127.0.0.1:50432 ./stream-deck.sh "!socials"
#   VIRTA_TOKEN=vk_... VIRTA_ADDR=127.0.0.1:50432 ./stream-deck.sh "gg!"
set -euo pipefail

: "${VIRTA_TOKEN:?Set VIRTA_TOKEN to a token with the 'send' scope}"
: "${VIRTA_ADDR:?Set VIRTA_ADDR to the daemon's listen address}"

MESSAGE="${1:?Usage: $0 <message>}"

# 1. Discover which channels are reachable (the daemon returns can_send=true for signed-in ones).
PREVIEW=$(curl -sf "http://${VIRTA_ADDR}/v1/send/preview" \
  -H "Authorization: Bearer ${VIRTA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$(jq -cn --arg a "${VIRTA_ADDR}" '{channels:[]}')" \
  2>/dev/null) || { echo "Could not reach daemon at ${VIRTA_ADDR}"; exit 1; }

# Extract channel keys where can_send is true.
CHANNELS=$(echo "${PREVIEW}" | jq -r '[.targets[] | select(.can_send) | .channel] | @json')

if [[ "$(echo "${CHANNELS}" | jq 'length')" -eq 0 ]]; then
  echo "No reachable channels (are you signed in?)"
  exit 1
fi

# 2. Send.
RESULT=$(curl -sf "http://${VIRTA_ADDR}/v1/send" \
  -H "Authorization: Bearer ${VIRTA_TOKEN}" \
  -H "Content-Type: application/json" \
  -d "$(jq -cn --argjson ch "${CHANNELS}" --arg txt "${MESSAGE}" '{channels:$ch, text:$txt}')")

echo "Sent: $(echo "${RESULT}" | jq -r '.results[].status' | paste -sd,)"
