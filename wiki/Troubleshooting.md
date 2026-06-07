# Troubleshooting

---

## Desktop app shows "Not connected to the daemon. Retrying…"

The embedded daemon is still starting up (usually takes 1–2 seconds). If it persists:

1. Check if a previous daemon process is still running: `pkill virtad`
2. Delete the stale discovery file: `rm -f /run/user/$UID/virta/daemon.json`
3. Restart the app.

If the app never connects, launch from a terminal to see daemon output:
```bash
./virta 2>&1 | head -50
```

---

## Chat is not appearing

**Check the connection status dot** next to the stream in the Streams sidebar.

- **No dot / gray** — the channel is offline or still connecting. Wait a moment.
- **Red dot** — the adapter encountered an error. Check Settings → Connections for the platform.

If the dot is green but no messages appear:

1. Open the dev feed in a browser: `http://localhost:PORT/dev?token=YOUR_TOKEN`
2. If messages appear there but not in the UI, the WebSocket connection from the browser may have failed. Try refreshing.

---

## Twitch authentication fails

- Make sure your Twitch app's OAuth redirect URL is set to `http://localhost`.
- Device Code Grant links expire in about 5 minutes — start fresh if the code timed out.
- Verify your Client ID is correct in Settings → Connections → Twitch.

---

## Kick authentication fails

- Confirm the redirect URI in your Kick app settings is exactly `http://localhost:9876/callback`.
- Check that `VIRTA_KICK_CLIENT_ID` and `VIRTA_KICK_CLIENT_SECRET` are set correctly.

---

## Ask AI returns "message logging is disabled"

Enable logging: **Settings → Chat → Log messages**. History is only available from the point you enable logging; retroactive backfill is not supported.

---

## Ask AI has no models available

1. Go to **Settings → Intelligence**.
2. Check that at least one provider is configured with a valid API key.
3. For Ollama: confirm Ollama is running and reachable at the configured URL.
4. Click **Refresh models** next to the provider.

---

## OBS overlay is blank or not transparent

- In OBS Browser Source settings, check **Allow transparency**.
- Make sure the URL in OBS matches exactly what the OBS panel generated (it includes the token as a query parameter).
- If the overlay loads but appears white, switch the background theme to **Transparent** in the OBS panel.

---

## obs-websocket connection fails

- Confirm OBS 28+ is running and the WebSocket server is enabled: **Tools → WebSocket Server Settings → Enable WebSocket Server**.
- Check the port (default 4455) and password match what you entered in Virta.
- Click **Detect OBS** in the OBS Control tab to re-probe.

---

## "No playable format" when watching a stream in the desktop app

WebKitGTK does not support the WebGPU/WebCodecs stack required by Twitch's IVS video player. Use the **"Watch on Twitch"** button to open the stream in your browser instead.

---

## Docker container exits immediately

Check the logs:
```bash
docker compose logs virtad
```

Common causes:
- `VIRTA_TOKEN` is empty — set it in `.env`
- `POSTGRES_PASSWORD` is empty — set it in `.env`
- Database is not ready yet — wait and retry, or check `docker compose logs db`

---

## High memory usage

If the chat buffer grows large:

- **Settings → Chat → Max messages in feed** — reduce the in-memory buffer (default 2000).
- Enable a message retention policy in **Settings → Storage** if logging is on.

---

## Clearing all data

```bash
# Docker
docker compose down -v    # removes all volumes (data is lost)

# Binary / desktop
rm -rf ~/.local/share/virta     # Linux
rm -rf ~/Library/Application\ Support/virta  # macOS
```

---

## Getting help

- Open an issue at [github.com/elythi0n/virta/issues](https://github.com/elythi0n/virta/issues)
- Include the daemon version (`virtad version`), OS, and what you tried
