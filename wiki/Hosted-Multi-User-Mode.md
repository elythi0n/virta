# Hosted Multi-User Mode

Hosted mode lets multiple people share one Virta server. Each user registers an account, logs in, and gets their own private workspace: their own channel list, filter rules, message history, and settings.

This is the right mode for a team, a streaming agency, or anyone who wants to access Virta from multiple devices.

---

## Enabling hosted mode

Set `VIRTA_HOSTED=1` in your environment:

```env
VIRTA_HOSTED=1
VIRTA_STORAGE=postgres        # required — hosted mode needs Postgres
VIRTA_DB_DSN=postgres://...
VIRTA_TOKEN=...               # root admin token
```

Restart the daemon. The web UI shows a **Sign in / Create account** screen.

> Hosted mode requires Postgres. SQLite cannot handle concurrent writes from multiple users safely.

---

## User registration

With hosted mode enabled, anyone who can reach the UI can create an account. There is no invite gate in alpha — use network-level access control (Cloudflare Access, VPN, firewall rule) if you want to restrict who can sign up.

### First account

The first user to register is a regular user. There is no automatic admin promotion in alpha. The operator uses the root `VIRTA_TOKEN` for administrative API calls.

---

## Data isolation

Every piece of user data is scoped to the account:

- Channels (joined streams)
- Workspaces and filter rules
- Message history
- Platform connections (Twitch/Kick tokens)
- LLM API keys
- Settings

Users cannot see other users' data.

The operator's `VIRTA_TOKEN` bypasses per-user scoping and has full access to all data — keep it secret.

---

## Platform OAuth in hosted mode

In hosted mode, Twitch and Kick sign-in use the **server-configured OAuth app** (set via `VIRTA_TWITCH_CLIENT_ID` etc.) rather than each user's own app. Users click "Sign in with Twitch" in Settings and authenticate with Twitch through the shared app.

If you leave the platform client IDs empty, platform sign-in is disabled (users can still read chat anonymously).

---

## Reverse proxy setup

Virta should be placed behind a reverse proxy (Nginx, Caddy, Traefik) for TLS and domain routing.

### Caddy example

```caddyfile
virta.example.com {
    reverse_proxy localhost:8344
}
```

### Nginx example

```nginx
server {
    listen 443 ssl;
    server_name virta.example.com;

    ssl_certificate     /etc/ssl/virta.crt;
    ssl_certificate_key /etc/ssl/virta.key;

    location / {
        proxy_pass http://localhost:8344;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_read_timeout 86400;
    }
}
```

The `Upgrade` and `Connection` headers are required for the WebSocket feed to work through the proxy.

---

## Session management

Sessions expire after 30 days of inactivity. Users are prompted to re-authenticate. The daemon sweeps expired sessions hourly.

---

## Removing a user

There is no admin UI for user management in alpha. Use Postgres directly:

```sql
-- Find user
SELECT id, email FROM accounts;

-- Delete user and all associated data
DELETE FROM accounts WHERE id = 'user-id-here';
```

This cascades to all user-scoped data.
