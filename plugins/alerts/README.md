# Alerts

Streamlabs-style stream alerts for Virta: subs, resubs, gift subs, raids, hosts, and follows,
played in an animated OBS overlay you configure from a Virta panel — with a live preview that
renders through the exact same code path as the overlay.

## Features

- **Six event types** — `sub`, `resub`, `giftsub`, `raid`, `host`, `follow` — each with its own
  enable toggle, animation, card style, hold duration, and text template.
- **Threshold tiers** — escalate the celebration with the magnitude: e.g. a raid plays
  `slide-up`/`minimal-dark` by default, `pop`/`neon-outline` at 50+ viewers, and
  `confetti`/`gold-epic` at 200+. Tiers can override any subset of animation/style/template/
  duration; the highest matching `min` wins.
- **10 animation presets** — slide-up, slide-bounce, pop, fade-zoom, flip-in, glitch, shake,
  neon-pulse, confetti (canvas particle burst), rainbow-sweep. All Web Animations API, no
  dependencies. `prefers-reduced-motion` degrades every preset to a plain fade.
- **6 card style presets** — minimal-dark, neon-outline, gradient-glass, gold-epic,
  retro-arcade, clean-light.
- **Live preview** — the panel's 16:9 checkerboard strip plays test alerts *and* real incoming
  events using the same `renderAlert` + stylesheet as the overlay, so it doubles as an on-air
  confidence monitor.
- **Safe by construction** — alert text is built with `textContent` only; chat-controlled data
  can never become markup. The overlay runs read-only (a read-scoped token).

## Overlays — as many as you want

The **Overlays** section in the panel manages overlay *instances*. Each instance is its own OBS
browser source with its own URL, an alert-type filter, and a position — so you can run one
overlay that only shows raids top-center and another that shows everything else bottom-left.
Selecting no types on an instance means "everything plays here". Instances (like all alert
settings) are saved in Virta's plugin config on disk, so they're there every time you open
Virta. Removing an instance kills its URL (the page shows a hint instead of playing alerts).

## OBS setup

1. In Virta, open the **Alerts** panel. In **Overlays**, add/name an overlay, pick its alert
   types and position, then click its **Copy URL**. (If the button reports that the host
   doesn't support overlay URLs, update Virta.)
2. In OBS: **Sources → + → Browser**, name it after the overlay.
3. Paste the URL, set **Width 1920, Height 1080**. Leave Custom CSS empty — the page background
   is already transparent.
4. Done. Click **Test** on any event type in the panel; the same alert plays in OBS. Repeat
   from step 1 for each additional overlay.

Notes:

- The URL embeds a long-lived read-scoped token minted for this plugin. Treat it like a
  password; revoking the token in Virta's settings invalidates every copied URL.
- The overlay re-fetches the rules every 60 seconds and on every reconnect, so panel edits
  propagate without restarting OBS.
- One alert plays at a time (enter → hold → exit → 0.5s gap). The queue is capped at 20; in a
  raid-storm the oldest queued alerts are dropped rather than playing minutes of stale hype.

## Config model

Everything lives in the plugin config, persisted to disk by Virta's plugin store: `rules` and
`overlays` (the panel manages them; `overlay_token` is host-managed — leave it alone).
`overlays` is a list of `{ id, name, types, position }` where empty `types` means every alert
type plays on that instance. The rules object:

```json
{
  "rules": {
    "version": 1,
    "enabled": true,
    "position": "bottom",
    "types": {
      "raid": {
        "enabled": true,
        "animation": "slide-up",
        "style": "minimal-dark",
        "durationMs": 6000,
        "template": "{author} is raiding with {viewers} viewers!",
        "tiers": [
          { "min": 50, "animation": "pop", "style": "neon-outline" },
          { "min": 200, "animation": "confetti", "style": "gold-epic" }
        ]
      }
    }
  }
}
```

- `position`: `top` | `center` | `bottom` — which band of the 1080p canvas alerts appear in.
- Templates substitute `{author}`, `{count}` (gift subs), `{viewers}` (raid/host), `{months}`
  (resub), `{tier}` — always as plain text.
- Tiers exist for the types with a magnitude: `giftsub` → count, `raid`/`host` → viewers,
  `resub` → months. A tier is `{ min, animation?, style?, template?, durationMs? }`; omitted
  fields inherit from the base rule.
- Magnitudes are parsed from the platform's system-message text (the wire carries no structured
  counts), e.g. "… is raiding with a party of 1,234." → `viewers: 1234`. Parsing is
  best-effort with safe fallbacks (`count` 1, `viewers`/`months` 0).

## Extending it — the registry pattern

The whole animation/style system is a flat registry in `gui/animations.js`
(`window.VirtaAlertAnimations`), shared by the panel and the overlay. Both pages read the
registry at runtime, so adding an entry automatically populates the panel's selects and the
overlay's playback — no other file changes, no build step.

### Add an animation preset

Append one object to the `presets` array in `gui/animations.js`:

```js
{
  id: "heartbeat",            // stored in config; must be stable
  name: "Heartbeat",          // shown in the panel selects
  enter: function (el) {      // el is the .va-card; resolve when the entrance is done
    return animate(el, [
      { transform: "scale(0.8)", opacity: 0 },
      { transform: "scale(1.06)", opacity: 1, offset: 0.5 },
      { transform: "scale(1)", opacity: 1 },
    ], { duration: 500, easing: "ease-out", fill: "both" });
  },
  exit: function (el) {       // resolve when the card may be removed
    return animate(el, [{ opacity: 1 }, { opacity: 0 }], { duration: 300, fill: "both" });
  },
}
```

House rules for presets:

- Use the in-file helpers: `animate()` (never rejects, so a broken animation can't wedge the
  queue), `holdLoop()` for effects that run while the card is held (neon-pulse, rainbow-sweep),
  and call `stopHold(el)` at the start of `exit` if you used `holdLoop`.
- `el.parentElement` is the stage — use it for ambient effects (see `burstConfetti`).
- Don't special-case reduced motion in the preset; `get()` already swaps every preset for a
  fade when the viewer asked for reduced motion.

### Add a card style preset

1. Add a class in `gui/alert-styles.css`:

   ```css
   .va-style-vaporwave {
     background: linear-gradient(135deg, #FF71CE, #01CDFE);
     border: 1px solid #B967FF;
     color: #14181F;
   }
   ```

2. Register it in the `styles` array in `gui/animations.js`:

   ```js
   { id: "vaporwave", name: "Vaporwave", className: "va-style-vaporwave" }
   ```

The card DOM a style can target: `.va-card` (container) → `.va-badge` (the "RAID" tag) and
`.va-text` (the template line, with `.va-tpl-author`, `.va-tpl-count`, `.va-tpl-viewers`,
`.va-tpl-months`, `.va-tpl-tier` spans inside).

### Add an event type

Add an entry to `eventTypes` (id must match the wire's `UnifiedMessage.type`), a default rule
in `defaultRules()`, and — if the type has a magnitude — a branch in `parseMagnitudes`.

## Layout

```
virta-plugin.json      manifest (scopes: events, ui, storage)
gui/index.html|app.js|style.css      configurator panel (runs in Virta, uses __virta.js)
gui/overlay.html|overlay.js|overlay.css  standalone OBS browser source (token auth, raw WS)
gui/animations.js      shared registry: animations, styles, rules, renderer, queue
gui/alert-styles.css   shared card style classes
build.sh               packages dist/alerts.zip
```

## Build

```sh
./build.sh   # → dist/alerts.zip with virta-plugin.json + gui/ at the zip root
```
