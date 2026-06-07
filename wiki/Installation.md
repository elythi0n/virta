# Installation

Virta ships as a single self-contained binary (`virtad`) that serves both the HTTP API and the embedded web UI. There are three ways to run it.

---

## Option 1 — Docker Compose (recommended)

The fastest path for a server or local install with Postgres.

### Prerequisites

- Docker + Docker Compose v2
- A terminal

### Steps

```bash
# 1. Clone the repo
git clone https://github.com/elythi0n/virta
cd virta

# 2. Create your environment file
cp .env.example .env
```

Open `.env` and set **at minimum**:

```env
VIRTA_TOKEN=<output of: openssl rand -hex 32>
POSTGRES_PASSWORD=<a strong random password>
```

```bash
# 3. Start everything
docker compose up -d

# 4. Open the UI
open http://localhost:8344
```

The first startup creates the database schema automatically. There is no separate migration step.

### Stopping and upgrading

```bash
docker compose down          # stop
docker compose pull          # pull latest images (when they exist)
docker compose up -d         # restart with latest
```

Data persists in Docker named volumes (`virta-data`, `virta-db`).

---

## Option 2 — Pre-built binary

For running directly on the host without Docker.

### Prerequisites

- Linux, macOS, or Windows
- SQLite is the default database (zero setup); Postgres is optional

### Download

Grab the latest release binary from the [Releases](https://github.com/elythi0n/virta/releases) page for your OS. Binaries are named `virtad-linux-amd64`, `virtad-darwin-arm64`, etc.

```bash
# Linux example
curl -Lo virtad https://github.com/elythi0n/virta/releases/latest/download/virtad-linux-amd64
chmod +x virtad

# Run with a random token
export VIRTA_TOKEN=$(openssl rand -hex 32)
./virtad
```

The daemon prints its URL and the location of the bearer token:

```
time=... msg="api listening" addr=127.0.0.1:54321
time=... msg="dev feed" url=http://127.0.0.1:54321/dev token="in ~/.local/share/virta/run/daemon.json"
```

Open the printed URL in a browser.

### Systemd service (Linux)

```ini
# /etc/systemd/system/virtad.service
[Unit]
Description=Virta daemon
After=network.target

[Service]
User=youruser
EnvironmentFile=/etc/virta/env
ExecStart=/usr/local/bin/virtad
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

```bash
sudo systemctl enable --now virtad
```

---

## Option 3 — Desktop app

The native desktop application bundles `virtad` inside a Wails v3 window. It starts the daemon automatically — no terminal needed.

See [Desktop App](Desktop-App) for platform-specific instructions and limitations.

---

## Building from source

```bash
git clone https://github.com/elythi0n/virta
cd virta

# Build the daemon only
make daemon          # outputs dist/virtad

# Build the full desktop app (requires Wails CLI and WebKit dev libs)
make app             # outputs frontends/desktop/build/bin/virta
```

Go 1.22+ is required. The web UI is pre-built in the repo; if you want to rebuild it: `make web`.
