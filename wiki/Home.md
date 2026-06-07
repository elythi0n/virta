# Virta Wiki

> **Alpha software.** Virta is under active development. Interfaces, config keys, and data formats may change between releases. Back up your data before upgrading.

Virta is a unified live chat dashboard for streamers. It pulls Twitch and Kick chat into one real-time feed, with moderation tools, filter rules, message history, an AI assistant, and an OBS overlay — all from a single daemon that you run locally or host on a server.

---

## Pages

| Section | What it covers |
|---|---|
| [Installation](Installation) | Docker, self-hosted binary, desktop app |
| [Configuration](Configuration) | All environment variables |
| [Connecting Platforms](Connecting-Platforms) | Twitch and Kick OAuth setup |
| [Workspaces and Profiles](Workspaces-and-Profiles) | Multiple channel sets, switching workspaces |
| [Message Logging](Message-Logging) | Enabling history, required for Ask AI |
| [Ask AI and MCP](Ask-AI-and-MCP) | AI chat assistant, connecting Claude Desktop / Cursor |
| [OBS Integration](OBS-Integration) | Chat overlay and obs-websocket stats push |
| [Hosted Multi-User Mode](Hosted-Multi-User-Mode) | Running Virta as a shared server |
| [API Tokens](API-Tokens) | Scoped tokens for bots, Stream Deck, overlays |
| [Desktop App](Desktop-App) | The native Wails v3 desktop client |
| [Troubleshooting](Troubleshooting) | Common problems and fixes |

---

## Quick start (Docker)

```bash
git clone https://github.com/elythi0n/virta
cd virta
cp .env.example .env
# Edit .env: set VIRTA_TOKEN and POSTGRES_PASSWORD
docker compose up -d
open http://localhost:8344
```

The web UI opens at `http://localhost:8344`. Sign in with the credentials you set. Add your first stream by clicking **+** in the Streams sidebar.

---

## Current status

Virta is **alpha**. Core features are working but expect rough edges:

- All platforms: Twitch, Kick
- Chat aggregation, filter rules, moderation queue
- Message history and AI assistant (requires logging enabled)
- OBS overlay (Browser Source and `virta-overlay` native binary)
- Desktop app (Linux, macOS, Windows via Wails v3)
- Multi-user hosted mode

Not yet stable: plugin system, X/Twitter platform, advanced AI tool integration.
