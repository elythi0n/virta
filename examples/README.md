# Virta API examples

Third-party integrations using the `/v1` API — the same surface the Virta frontends use.
All you need is the token from Settings → Integrations (or the root token from the discovery
file at `$XDG_RUNTIME_DIR/virta/virta.json` on Linux).

## Full API docs

With virtad running: `http://127.0.0.1:<port>/docs`  
OpenAPI spec:  `GET /v1/openapi.json`  
AsyncAPI spec: `GET /v1/asyncapi.json`

## Examples

| File | What it does | Scope needed |
|---|---|---|
| `stream-deck.sh` | Post to all channels from a Stream Deck key-press | `send` |
| `reply-bot/main.go` | Go bot that replies to `!ping` with `!pong` | `read` + `send` |
| `obs-raid-scene.sh` | Switch OBS scene on a raid event | `read` |
