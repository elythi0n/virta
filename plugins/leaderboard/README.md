# Leaderboard — Virta plugin

Session leaderboards inside a Virta dock panel: **Top chatters** and **Top emotes**, tallied live
from every connected platform (Twitch, Kick, X). Everything is in-memory and session-only — counts
reset when the panel closes.

## What it does

- **Top chatters** — messages per author, with rank, the author's name in their platform color
  (lightened when too dark for the dark background), a platform tag (TW / KK / X), the message
  count, and a bar proportional to rank 1.
- **Top emotes** — emote usage tallied from message segments, with rank, emote name, count, and a
  proportional bar.
- **Header controls** — pause/resume (stops tallying, keeps the display), reset (clears tallies),
  Top-N (10 / 25 / 50), and a channel filter populated from the channels seen in the stream.

## How it counts

- Only ordinary chat messages count toward tallies (`type: "chat"`, or messages with no type).
  Subs, raids, follows, moderation and other event rows are ignored.
- Chatters are keyed by platform + author id (falling back to the author name), so the same name
  on two platforms is two entries and a rename on one platform stays one entry.
- Emotes are counted per emote code from the message's parsed segments — one use per occurrence.
- The channel filter tallies per channel key (`platform:slug`), so switching the filter re-ranks
  from per-channel counts without losing the all-channels totals.
- **Caps**: the tally maps are bounded at **5000 authors** and **2000 emotes**. Once a cap is hit,
  messages from *new* authors (or *new* emotes) are dropped; existing entries keep counting. Use
  **Reset** to start fresh if a long session hits the cap.

## Install

1. Build the archive:

   ```bash
   ./build.sh
   ```

2. In Virta: **Plugins → Install from zip** and pick `dist/leaderboard.zip`, or install from a Git
   URL pointing at this directory.

3. Open the **Leaderboard** panel from the panel catalog (or the command palette).

The plugin requests the `events` scope (live message stream) and the `ui` scope (the dock panel);
Virta asks for consent at install time.

## Dev loop

Edit `gui/`, then:

```bash
./build.sh            # produces dist/leaderboard.zip
# install dist/leaderboard.zip via Virta's Plugins panel to pick up the change
```

## How it works

- **Sandboxed GUI**: static HTML/CSS/JS served by the Virta daemon under a strict CSP — the panel
  itself can make no network calls and runs no inline scripts.
- **Events bridge**: the panel calls `events.subscribe` over the `window.__virta` postMessage
  bridge and receives live `events.message` payloads; it unsubscribes on close.
- **Coalesced rendering**: messages update in-memory tallies immediately, but the DOM re-renders
  at most every ~500 ms, so chat bursts stay cheap.
