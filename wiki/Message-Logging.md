# Message Logging

Message logging persists chat messages to the database. It is required for:

- The **Ask AI** assistant's history tools (search messages, top chatters, channel stats)
- The **chat history** search feature
- VOD review and moderation auditing

Logging is **off by default** to respect user privacy. You opt in explicitly.

---

## Enabling logging

### Option A — Settings UI

1. Open **Settings → Chat** (or **Settings → Storage**).
2. Toggle **Log messages** on.

This takes effect immediately and applies to the active workspace.

### Option B — Environment variable

Set `VIRTA_LOGGING_ENABLED=1` before starting the daemon. This enables logging globally without needing the UI toggle.

```env
VIRTA_LOGGING_ENABLED=1
```

When this env var is set, the toggle in Settings reflects it.

---

## What gets logged

When logging is enabled:

- Every chat message, subscription event, raid, gift, and announcement from all joined channels
- The author's display name, login, platform, badges, and message content
- The channel key and timestamp
- Deletion/ban markers (messages are soft-deleted, not removed)

What is **not** stored:

- Message content from channels where logging was off at the time the message arrived
- Authentication tokens, passwords, or platform credentials

---

## Retention

Messages are kept until you explicitly clear them. A background sweeper applies the retention policy set in Settings:

| Policy | What it does |
|---|---|
| **Forever** (default) | Messages accumulate indefinitely |
| **30 days** | Messages older than 30 days are deleted nightly |
| **7 days** | Messages older than 7 days are deleted nightly |
| **1 day** | Messages older than 24 hours are deleted nightly |

The sweeper runs once per hour. Deleting a large backlog may take a few passes.

---

## Storage requirements

Message volume depends entirely on the channels you follow. A rough guide:

| Channels | Activity | Approx. per day |
|---|---|---|
| 2–5 small channels | Low | ~5 MB |
| 5–10 mid-size channels | Medium | ~50 MB |
| 10+ large channels | High | ~200 MB+ |

Use Postgres for server installs to avoid SQLite locking issues under high write load.

---

## Disabling logging

Turn off the **Log messages** toggle in Settings. Incoming messages stop being stored immediately. Existing messages are not deleted (unless you also reduce the retention window).

---

## Clearing all history

Settings → Storage → **Clear all message history**. This is irreversible.
