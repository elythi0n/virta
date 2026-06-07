# API Tokens

Virta uses bearer tokens for authentication. There are two types:

- **Root token** (`VIRTA_TOKEN`) — full access to everything. Keep it secret.
- **Scoped tokens** — limited access, mintable from Settings or the API. Good for bots, Stream Deck integrations, overlay URLs, and MCP clients.

---

## Scopes

| Scope | What it allows |
|---|---|
| `read` | Read feed, channel list, stats, profiles, capabilities |
| `send` | Send chat messages |
| `moderate` | Moderation actions (delete, ban, timeout) |
| `control` | Join/leave channels, switch profiles, update settings |
| `admin` | Settings write, token management, full API |

---

## Minting a scoped token

1. Open **Settings → Integrations → API tokens**.
2. Click **New token**.
3. Enter a name (e.g. "Stream Deck bot") and select the scopes you need.
4. Click **Create**. Copy the token — it is shown once and never again.

---

## Using a token

Pass it as a bearer token in the Authorization header:

```bash
curl http://localhost:8344/v1/channels \
  -H "Authorization: Bearer YOUR_TOKEN"
```

Or as a query parameter for WebSocket and overlay URLs:

```
http://localhost:8344/overlay?token=YOUR_TOKEN&panel=feed&transparent=1
ws://localhost:8344/v1/stream?token=YOUR_TOKEN
```

---

## Common use cases

### OBS overlay

Mint a `read`-scoped token. Paste it into the overlay URL from the OBS panel. If the URL leaks, revoke the token and mint a new one — your root token stays safe.

### Chat bot

Mint a `read` + `send`-scoped token. Give it to your bot. The bot calls `POST /v1/channels/{platform}/{slug}/send`.

### Stream Deck

Mint a `read` + `control`-scoped token. Use it with the Stream Deck HTTP Request action to hit Virta's REST endpoints. See the examples in `examples/stream-deck.sh`.

### External AI client (MCP)

Mint a `read`-scoped token. Use it in your MCP client config (Claude Desktop, Cursor, etc.). See [Ask AI and MCP](Ask-AI-and-MCP).

---

## Revoking a token

Settings → Integrations → API tokens → click the trash icon next to the token. The token stops working immediately.

---

## Root token rotation

To rotate the root token:

1. Update `VIRTA_TOKEN` in your environment file to a new value (`openssl rand -hex 32`).
2. Restart the daemon.

The old token stops working immediately. All scoped tokens remain valid (they are stored separately).
