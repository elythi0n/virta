#!/usr/bin/env bash
# OBS scene-switch on a raid event.
#
# Listens to the Virta event stream and calls the OBS WebSocket API (obs-websocket 5.x) to switch
# to "!raid scene" whenever a raid lands. Requires `wscat` (npm install -g wscat) or replace the
# WebSocket send with your preferred tool.
#
# Prerequisites:
#   - obs-websocket 5.x installed and enabled in OBS → Tools → WebSocket Server Settings
#   - A Virta token with the `read` scope (mint in Settings → Integrations)
#
# Usage:
#   VIRTA_TOKEN=vk_... VIRTA_ADDR=127.0.0.1:50432 \
#   OBS_WS=127.0.0.1:4455 OBS_PASS=yourpassword OBS_SCENE="Raid scene" \
#   ./obs-raid-scene.sh
set -euo pipefail

: "${VIRTA_TOKEN:?Set VIRTA_TOKEN (read scope)}"
: "${VIRTA_ADDR:?Set VIRTA_ADDR}"
: "${OBS_WS:=127.0.0.1:4455}"
: "${OBS_PASS:?Set OBS_PASS}"
: "${OBS_SCENE:?Set OBS_SCENE to the scene name to switch to}"

obs_switch() {
  # obs-websocket 5.x SetCurrentProgramScene request.
  # Replace with an OBS WebSocket client library for a production bot.
  local req
  req=$(jq -cn --arg s "$OBS_SCENE" '{op:6,d:{requestType:"SetCurrentProgramScene",requestId:"raid",requestData:{sceneName:$s}}}')
  echo "$req" | wscat --connect "ws://${OBS_WS}" --wait 2 2>/dev/null || true
}

echo "Watching for raids on ${VIRTA_ADDR}…"
curl -sfN "http://${VIRTA_ADDR}/v1/stream?token=${VIRTA_TOKEN}" \
  -H "Upgrade: websocket" 2>/dev/null \
| while IFS= read -r line; do
  type=$(echo "$line" | jq -r '.type // empty' 2>/dev/null) || continue
  if [[ "$type" == "message" ]]; then
    msg_type=$(echo "$line" | jq -r '.message.type // empty' 2>/dev/null)
    if [[ "$msg_type" == "raid" ]]; then
      raider=$(echo "$line" | jq -r '.message.author.display_name // "unknown"' 2>/dev/null)
      echo "Raid from ${raider} — switching to '${OBS_SCENE}'"
      obs_switch
    fi
  fi
done
