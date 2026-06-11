# VOD Replay

Replays a Twitch VOD's chat synced to a playback clock, so VOD reviews and reruns get
live-feeling chat. Paste a VOD URL (`https://www.twitch.tv/videos/123456789`) or a bare
VOD ID, press Load, and the chat feed plays back in real time with play/pause, seeking,
and 1x–2x playback speed. Chat pages are buffered ahead of the clock in the background.

## Install

- Virta → Plugins panel → install from the marketplace, or point the install URL at a
  release zip built with `./build.sh` (produces `dist/vod-replay.zip`).

## Network access

Comments come from Twitch's public (unofficial) GQL endpoint. All requests go through
Virta's plugin HTTP bridge, and the manifest allowlist permits only
`https://gql.twitch.tv/gql` — the plugin can reach nothing else.
