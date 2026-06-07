# OBS Integration

Virta integrates with OBS in two ways:

1. **Chat overlay** — show the live chat feed inside OBS as a browser source or native transparent window.
2. **OBS Control (optional)** — connect to obs-websocket v5 to push live stats into OBS text sources and trigger scene switches from chat events.

---

## Chat Overlay

### Step 1 — Generate the overlay URL

1. Open the **OBS** panel in Virta (Panels catalog → OBS).
2. Go to the **Overlay** tab.
3. Configure:
   - **What to show**: All chat, Mentions only, Subs & raids, or a panel (Stats, Markets)
   - **Channels**: All channels or specific ones
   - **Background**: Transparent (for OBS Browser Source), Dark glass, or Dark solid
   - **Fade out**: Messages disappear after N seconds (good for game overlays)
   - **Size**: Match your OBS source dimensions
4. Click **Copy URL**.

### Step 2 — Add to OBS

**Option A — Browser Source (recommended)**

Works on any OBS build that has the Browser source (most do).

1. In OBS: **Sources → + → Browser**
2. Paste the URL.
3. Set Width and Height to match the values you chose.
4. Check **Allow transparency** if you used the Transparent background.
5. Check **Shutdown source when not visible** (optional, saves resources).

The overlay connects directly to your local daemon — no internet required.

**Option B — virta-overlay (no Browser Source needed)**

For OBS builds without CEF/Browser Source (some Linux distro packages).

Run in a terminal:
```bash
virta-overlay --panel feed --port 8344 --token YOUR_TOKEN --width 400 --height 600 --x 20 --y 100
```

Then in OBS: **Sources → + → Window Capture (Xcomposite)** → select **virta-overlay:feed**.

This creates a frameless transparent window that OBS captures with full alpha. The window is click-through so it doesn't interfere with other apps.

Flags:
```
--panel   Panel to show: feed, mentions, celebrations, stats, markets
--port    Daemon port (default 8344)
--token   Bearer token (from daemon.json or Settings → Integrations)
--width   Window width in pixels
--height  Window height in pixels
--x       X position on screen
--y       Y position on screen
--title   Window title (default: virta-overlay:<panel>)
```

---

## OBS Control (optional)

Connects Virta to OBS over obs-websocket v5 (built into OBS 28+). Lets Virta push live stats to OBS text sources and fire scene switches from chat events.

> This is completely separate from the chat overlay. You do not need to connect OBS for the overlay to work.

### Prerequisites

- OBS 28 or later (obs-websocket is bundled)
- In OBS: **Tools → WebSocket Server Settings → Enable WebSocket Server**
- Note the port (default: 4455) and password

### Connecting

1. Open the **OBS** panel → **OBS Control** tab.
2. Click **Detect OBS** — Virta probes `localhost:4455` automatically.
3. Enter the WebSocket password if you set one.
4. Click **Save and connect**.

The status dot turns green when connected. OBS version and WebSocket version are shown.

### Pushing stats to OBS text sources

1. In OBS: create a **Text (GDI+)** source (or FreeType on Linux) for each stat.
2. In Virta's **OBS Control** tab, under **Push stats to OBS text sources**:
   - Click **Add stat mapping**
   - Choose the stat: **Msgs / min** or **Unique chatters**
   - Type the exact OBS source name (autocomplete shows your OBS sources)
3. Set the **Update every** interval (default: 5 seconds).
4. Click **Save mappings**.

Virta starts writing values to those text sources immediately.

### Event rules (scene switching)

In the same tab you can set up rules that fire obs-websocket calls when chat events happen:

- **Raid** → switch to a raid scene
- **Sub / resub** → switch scene or toggle a source
- **Gift sub** → same

Scene names autocomplete from your OBS scene list.

---

## Token security

The overlay URL contains a read-only bearer token. Treat it like a password:
- Do not stream the URL on screen
- Regenerate it in **Settings → Integrations** if it leaks

For public overlays, mint a scoped read token that expires instead of using the root token. See [API Tokens](API-Tokens).
