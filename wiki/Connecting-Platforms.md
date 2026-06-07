# Connecting Platforms

Virta reads chat from Twitch and Kick. You can watch anonymously without signing in, but sending messages and using moderation requires an authenticated account.

---

## Adding streams (no sign-in required)

1. Click **+** in the Streams sidebar (or open the Streams panel).
2. Choose the platform (Twitch or Kick) and enter the channel name.
3. Click **Add stream**.

Chat starts appearing immediately. You are connected as an anonymous viewer.

---

## Twitch

### Anonymous chat (read-only)

Works out of the box. Just add a Twitch channel — no credentials needed.

### Authenticated (send messages, moderation)

Twitch authentication uses the **Device Code Grant** flow. You authenticate directly with Twitch; Virta never sees your password.

1. Open **Settings → Connections → Twitch**.
2. Click **Sign in with Twitch**.
3. Virta shows a short code and a URL (e.g. `https://www.twitch.tv/activate`).
4. Open the URL on any device, enter the code, and authorize the app.
5. Virta detects the approval and stores your token in the keychain.

#### Setting up your own Twitch app (optional)

By default Virta uses a shared client ID. To use your own:

1. Go to [dev.twitch.tv/console/apps](https://dev.twitch.tv/console/apps) and create a new application.
2. Set the OAuth redirect URL to `http://localhost`.
3. Copy the Client ID.
4. In Virta: Settings → Connections → Twitch → Client ID, paste it.
5. Or set `VIRTA_TWITCH_CLIENT_ID` in your environment before the daemon starts.

#### Permissions (scopes)

Virta requests the minimum scopes needed:
- `chat:read` — receive chat
- `chat:edit` — send messages
- `moderator:manage:chat_messages` — delete messages
- `moderator:manage:banned_users` — ban/unban
- `channel:moderate` — general mod actions

---

## Kick

### Anonymous chat (read-only)

Works out of the box via Kick's Pusher WebSocket. Just add a Kick channel.

### Authenticated (send messages)

Kick uses OAuth 2.0 with PKCE.

1. Open **Settings → Connections → Kick**.
2. Click **Sign in with Kick**.
3. A browser window opens to Kick's authorization page.
4. Approve the request.
5. Virta captures the callback and stores your token.

#### Setting up your own Kick app

1. Go to [kick.com/settings/developer](https://kick.com/settings/developer) and create an app.
2. Set the redirect URI to `http://localhost:9876/callback` (Virta's loopback port).
3. Copy the Client ID and Client Secret.
4. Set `VIRTA_KICK_CLIENT_ID` and `VIRTA_KICK_CLIENT_SECRET`, or enter them in Settings → Connections → Kick.

---

## Checking connection status

The status dot next to each stream in the Streams sidebar shows the connection state:

- **Green** — connected and receiving events
- **Gray** — connecting or degraded
- No dot — offline (stream is not live or adapter is stopped)

Hover over a stream card to see more detail.

---

## Removing an account

Settings → Connections → [platform] → Disconnect. This removes the stored token from the keychain. The stream channels themselves remain; they will continue in anonymous read-only mode.
