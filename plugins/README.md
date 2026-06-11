# Virta plugins

First-party plugins, developed in-tree for convenience but built as if they were third-party.
**Each directory here is a standalone plugin and a future separate repository.** Nothing in this
tree may import from the rest of the repo, reference repo paths, or rely on being in-tree — a
folder must be extractable to its own repo by copying it, with zero changes elsewhere.

The rules that keep that true:

1. **Self-contained.** A plugin folder holds everything it needs: manifest, GUI assets, build
   script, its own README. No shared code between plugins (copy small helpers instead — they
   will live in different repos).
2. **Same API as everyone else.** Plugins here use the public manifest + bridge surface only.
   If one of these plugins can't do something, the fix is extending the plugin API for all
   plugins — never a private hook for ours.
3. **Installed like everyone else.** The daemon installs these from a zip through the normal
   installer; being in-tree grants nothing.

## Anatomy

```
plugins/<name>/
  virta-plugin.json   # manifest (required, at archive root)
  gui/                # sandboxed panel assets: index.html + js/css
  build.sh            # produces dist/<name>.zip
  README.md           # what it does, config, install
```

## Manifest

Validated by `internal/plugin/host` on install. The important fields:

```json
{
  "id": "com.virta.example",
  "name": "Example",
  "version": "0.1.0",
  "publisher": "Virta",
  "description": "…",
  "engines": { "virta": ">=1.0.0" },
  "scopes": ["events", "ui", "storage"],
  "contributes": { "panels": [{ "kind": "example", "title": "Example", "icon": "zap" }] },
  "main": { "gui": "gui/index.html" },
  "config": { "type": "object", "properties": { } }
}
```

Scopes: `read`, `events`, `send`, `moderate`, `storage`, `http`, `ui`. Declare only what the
plugin uses — the bridge rejects calls outside declared scopes, and users see the list at
install time. Panel icons map to the host icon set (`chat`, `stream`, `stats`, `plugins`,
`mentions`, `filter`, `gift`, `search`, `list`, `grid`, `star`, `zap`); unknown names fall back
to the plug icon.

## The GUI sandbox

Panel pages are served by the daemon under `/plugins/{id}/gui/` with a strict CSP:
`default-src 'none'; script-src 'self'; connect-src 'self' ws: wss:`. In practice:

- No inline `<script>` — all JS in files.
- No external network, CDNs, or fonts. Ship everything in `gui/`.
- Load the host SDK **before** your code: `<script src="__virta.js"></script>` (the daemon
  serves this virtual file — don't create it).
- Build DOM with `textContent`, never `innerHTML`, for anything derived from chat.

## Bridge API (`window.__virta`)

Every privileged operation goes through `v.send({type, payload}) → Promise`; pushed data
arrives via `v.on(type, handler)`.

| Message | Scope | Effect |
|---|---|---|
| `config.get` | — | Resolves the plugin's saved config object |
| `config.set {values}` | `storage` | Replaces the plugin's saved config |
| `http.fetch {url, method?, headers?, body?}` | `http` | Outbound HTTP via the daemon, manifest-declared endpoints only |
| `events.subscribe {channels?}` | `events` | Start receiving live feed messages as `events.message` pushes (`channels` of `"platform:slug"` narrows; omit for all) |
| `events.unsubscribe` | `events` | Stop receiving |
| `overlay.url {path}` | `ui` | Tokenized standalone URL for a file in `gui/` (see Overlays) |

`events.message` payloads are FeedMessages: `{ id, ts, platform, type, author, authorId?,
authorColor?, channel, body, segments, event?: {count, viewers, months, tier}, … }`. `type` is
`chat` (or absent) for ordinary messages; event rows use `sub`, `resub`, `giftsub`, `raid`,
`host`, `follow`, `announcement`, `moderation`, `system`.

## Overlays (OBS browser sources)

A plugin can ship extra standalone pages in `gui/` (e.g. `overlay.html`) meant to run outside
the dock — typically as an OBS browser source. Standalone pages have no host SPA parent, so
`__virta.js` is useless there; instead the panel asks the bridge for `overlay.url`, which mints
a **read-scoped** API token once (persisted in the plugin's config as `overlay_token` — declare
that property in your config schema), and returns
`http://<daemon>/plugins/{id}/gui/overlay.html?token=…`. The standalone page then talks to the
daemon directly, same-origin: `GET /v1/plugins/{id}/config` with `Authorization: Bearer`, and
`WebSocket /v1/stream?token=…` (raw wire events, not FeedMessages). Revoking the token in
Settings → Integrations kills every copied URL.

## Dev loop

```sh
cd plugins/<name>
./build.sh                       # → dist/<name>.zip
```

Then install the zip by absolute path — the installer accepts local paths exactly for this:
paste `/abs/path/to/plugins/<name>/dist/<name>.zip` into the Plugins panel's install box, or
`curl -X POST http://127.0.0.1:<port>/v1/plugins/install -H "Authorization: Bearer <token>" \
-d '{"url":"/abs/path/…/dist/<name>.zip"}'`. Reinstalling the same id replaces the plugin;
reload the panel to pick up GUI changes.

## Publishing

Attach the zip to a GitHub release; users install via the repo URL (the installer resolves the
latest release archive) or the direct zip URL. Version bumps go in the manifest — the installer
keys install dirs by id + archive digest, so a new zip is a clean upgrade.
