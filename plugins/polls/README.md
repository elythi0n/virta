# Polls

Cross-platform chat polls for Virta. Run a poll from a dock panel; viewers on Twitch, Kick,
and X vote straight from chat, counted once per person.

## How voting works

While a poll is live, any chat message whose entire text is one of these counts as a vote for
option N:

- `1` (just the option number, `1`–`6`)
- `!vote 1`
- `vote 1` (case-insensitive)

Rules:

- One vote per person per platform — voters are keyed by their platform identity, so the same
  account can't vote twice.
- Revoting switches your vote: a later vote from the same person moves their vote to the new
  option (the old one is decremented), it never adds a second vote.
- Numbers higher than the option count, non-chat events, and messages sent before the poll
  started are ignored.

## Panel flow

1. **Compose** — enter a question, 2–6 options, and an optional duration (Off/30s/60s/2m/5m),
   then hit **Start poll**.
2. **Live** — vote counts, percentages, and proportional bars update live; the leading option
   is tinted. A countdown ends the poll automatically if a duration was set, or end it manually
   with **End poll**.
3. **Results** — winner banner with final tallies, plus a **New poll** button.

The last 20 finished polls are kept in a collapsible **History** section, persisted in the
plugin's config storage.

## Install

1. Build the package:

   ```sh
   ./build.sh
   ```

2. Install `dist/polls.zip` in Virta (plugin install from file/URL).
3. Grant the requested scopes: `events` (read live chat for votes), `ui` (the dock panel),
   and `storage` (poll history).

## Dev loop

Edit files under `gui/`, re-run `./build.sh`, and reinstall `dist/polls.zip`. There is no build
step beyond zipping — the GUI is plain HTML/CSS/JS served sandboxed by the Virta host (strict
CSP, no network; everything goes through the `window.__virta` bridge).
