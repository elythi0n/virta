# Ask AI and MCP

Virta includes a built-in AI assistant (**Ask AI** pane) that can answer questions about your community using live chat data. It connects to any OpenAI-compatible API or runs locally via Ollama.

The same tools are also exposed as an **MCP server** (Model Context Protocol), letting external AI clients like Claude Desktop and Cursor query your chat history directly.

---

## Setting up a provider

1. Open **Settings → Intelligence**.
2. Enable the AI assistant.
3. Click **Add provider** and choose your provider type.
4. Enter your API key and select a default model.

### Supported providers

| Provider | Notes |
|---|---|
| OpenAI | GPT-4o, GPT-4 Turbo, etc. |
| Anthropic | Claude Sonnet, Claude Opus |
| xAI | Grok models |
| Mistral | Mistral Large, Mistral Small |
| Google | Gemini Pro, Flash |
| OpenAI-compatible | Any API that follows the OpenAI chat format (Together, Groq, etc.) |
| Ollama | Local models, no API key required |

### Ollama (local AI)

1. Install [Ollama](https://ollama.ai) and pull a model:
   ```bash
   ollama pull llama3.2
   ```
2. In Virta: **Settings → Intelligence → Add provider → Ollama**.
3. Set the base URL: `http://localhost:11434` (or `http://host.docker.internal:11434` in Docker).
4. No API key needed. Select a model from the list.

> In Docker, use `http://host.docker.internal:11434` to reach Ollama on the host machine.

---

## Using Ask AI

1. Open the **Ask AI** panel from the panel catalog (Panels → Ask AI).
2. Select a model from the dropdown.
3. Type a question and press Enter.

### Example questions

- "Who were the top chatters this week?"
- "Show me what people said about the last game."
- "How many unique chatters did we have today?"
- "Find messages mentioning 'poggers' in the last hour."
- "What was the raid message from xQc?"

**Requires message logging to be enabled.** Without logging the AI can still answer general questions but has no access to your chat history.

---

## MCP server

The MCP server exposes the same tools to external AI clients over HTTP.

### Endpoint

```
http://127.0.0.1:PORT/mcp
```

The port is random by default. Find it in the discovery file or check **Settings → Intelligence → MCP server**.

### Authentication

All MCP requests require a bearer token. Use the root token (`VIRTA_TOKEN`) or mint a scoped read token in **Settings → Integrations → API tokens**.

### Connecting Claude Desktop

Add this to your `claude_desktop_config.json`:

```json
{
  "mcpServers": {
    "virta": {
      "command": "curl",
      "args": [
        "-s",
        "-X", "POST",
        "-H", "Authorization: Bearer YOUR_TOKEN",
        "-H", "Content-Type: application/json",
        "-d", "@-",
        "http://127.0.0.1:PORT/mcp"
      ]
    }
  }
}
```

Or use a Streamable HTTP transport if your client supports it:

```json
{
  "mcpServers": {
    "virta": {
      "type": "streamable-http",
      "url": "http://127.0.0.1:PORT/mcp",
      "headers": {
        "Authorization": "Bearer YOUR_TOKEN"
      }
    }
  }
}
```

### Connecting Cursor

In Cursor settings → MCP, add a new server with type `Streamable HTTP` and the same URL and token.

### Public relay URL

For cloud AI clients to reach your MCP server, you need a public URL. Set up a reverse proxy (Nginx, Caddy, Cloudflare Tunnel) and point `VIRTA_MCP_RELAY_URL` to the public base URL:

```env
VIRTA_MCP_RELAY_URL=https://virta.example.com
```

The Ask AI pane will then tell you the exact connection URL when you ask about MCP setup.

---

## Available tools

| Tool | What it does |
|---|---|
| `search_messages` | Full-text search over logged messages |
| `get_user_history` | All messages from a specific chatter |
| `top_chatters` | Most active users in a time window |
| `channel_stats` | Message volume, unique chatters, top emotes |
| `get_messages_range` | Messages from a channel within a time range |
| `list_channels` | Currently joined channels |

All tools are read-only and size-capped to prevent context blowout.
