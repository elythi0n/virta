# Configuration

All configuration is done through environment variables. There are no config files — drop them in a `.env` file when using Docker Compose, or export them in your shell / systemd unit.

---

## Required

| Variable | Description |
|---|---|
| `VIRTA_TOKEN` | Bearer token that the web UI and API clients use to authenticate. Generate with `openssl rand -hex 32`. Required in all deployment modes. |

---

## Storage

| Variable | Default | Description |
|---|---|---|
| `VIRTA_STORAGE` | `sqlite` | Storage backend. `sqlite` (single file, good for local) or `postgres` (recommended for server/hosted). |
| `VIRTA_DB_DSN` | — | Postgres connection string. Required when `VIRTA_STORAGE=postgres`. Example: `postgres://virta:password@localhost:5432/virta?sslmode=disable` |
| `VIRTA_DATA_DIR` | OS default | Directory for the SQLite database and persistent data. Defaults to the OS user config dir (`~/.config/virta` on Linux). |
| `VIRTA_CACHE_DIR` | OS default | Directory for ephemeral cache data (emote CDN cache, etc.). |
| `VIRTA_RUNTIME_DIR` | OS default | Directory for the discovery file (`daemon.json`). Defaults to `$XDG_RUNTIME_DIR/virta` on Linux. |

---

## Networking

| Variable | Default | Description |
|---|---|---|
| `VIRTA_ADDR` | `127.0.0.1:0` | Address the HTTP server binds to. Use `0.0.0.0:8344` in Docker or when serving on a fixed port. The `:0` default picks a random available port (good for local/desktop use). |

---

## Features

| Variable | Default | Description |
|---|---|---|
| `VIRTA_HOSTED` | `0` | Set to `1` to enable multi-user mode: user registration, login, and per-user workspaces. Requires Postgres. See [Hosted Multi-User Mode](Hosted-Multi-User-Mode). |
| `VIRTA_LOGGING_ENABLED` | `0` | Set to `1` to enable persistent message logging to the database. Required for the Ask AI history tools and chat search. See [Message Logging](Message-Logging). |
| `VIRTA_MCP_RELAY_URL` | — | Public base URL at which the MCP server is reachable by external AI clients (Claude Desktop, Cursor, etc.). Example: `https://virta.example.com`. Leave empty for local-only use. See [Ask AI and MCP](Ask-AI-and-MCP). |

---

## Platform OAuth

These set the default OAuth app credentials for the whole server. Individual users can also enter their own keys in Settings → Connections.

| Variable | Description |
|---|---|
| `VIRTA_TWITCH_CLIENT_ID` | Twitch application Client ID. Needed for Twitch sign-in (send messages, moderation). Read-only anonymous chat works without this. |
| `VIRTA_KICK_CLIENT_ID` | Kick application Client ID. Needed for Kick sign-in. |
| `VIRTA_KICK_CLIENT_SECRET` | Kick application Client Secret. Needed for Kick sign-in. |

---

## Example .env

```env
# Required
VIRTA_TOKEN=your_long_random_secret_here
POSTGRES_PASSWORD=another_strong_random_password

# Storage
VIRTA_STORAGE=postgres
VIRTA_DB_DSN=postgres://virta:${POSTGRES_PASSWORD}@db:5432/virta?sslmode=disable

# Networking
VIRTA_ADDR=0.0.0.0:8344

# Features
VIRTA_HOSTED=0
VIRTA_LOGGING_ENABLED=1

# Platforms (optional — users can set in Settings)
VIRTA_TWITCH_CLIENT_ID=
VIRTA_KICK_CLIENT_ID=
VIRTA_KICK_CLIENT_SECRET=
```

---

## Secrets storage

Sensitive credentials (platform OAuth tokens, LLM API keys, OBS WebSocket password) are stored in the OS keychain on the desktop app, or in an encrypted file vault on the server. They are never stored in plain text in the database or config files.
