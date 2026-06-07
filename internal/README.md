# internal — layer map

The `internal/` tree is intentionally **wide and shallow** (57 packages). Go's unit of
encapsulation is the package, not the directory depth; nesting everything under layer folders
would rewrite 150+ import paths and add no compiler-checked guarantee. What follows is a
**reading aid and dependency contract** that replaces the need for artificial nesting.

---

## Layers and the packages that belong to each

Packages may freely import anything in their own layer or any layer **above** them in this
table. They must **never** import from a layer below. `internal/app` is the sole exemption —
it is the wiring layer and imports everything.

| Layer | Packages | Role |
|---|---|---|
| **Foundation** (leaf utils) | `clock` `id` `buildinfo` `config` `crash` `userctx` | No internal deps; pure utilities used everywhere. |
| **Domain kernel** | `platform` `pipeline` | `UnifiedMessage` model, `Adapter` port. The spine everything else threads through. |
| **Platform adapters** | `platform/twitch` `platform/kick` `auth/twitch` `auth/kick` `command` `dispatch` `ratelimit` | I/O with live platforms and the outbound send path. |
| **Stream enrichment** | `emotes` `badges` `filter` `velocity` `streams` `scrollback` `stats` `held` | Annotate or observe the message stream. No side effects on the wire. |
| **Engine + workspace** | `engine` `profiles` | Channel routing, the active workspace, logging policy. |
| **Persistence** | `store` `store/sqlcommon` `store/sqlite` `store/postgres` `secrets` `secrets/filevault` `secrets/keychain` `logbook` `search` `search/meilisearch` `search/noop` | Durable state, credentials, message log, full-text search. |
| **Intelligence** | `intel` `llm` `llm/anthropic` `llm/openaicompat` `translate` | LLM agent loop, provider implementations, translation. |
| **Extensions** | `plugin/host` `plugin/source` `plugin/markets` `plugin/xbridge` | External-data seam, plugin platform, sandboxed execution, subprocess bridge. |
| **Delivery** | `api` `webhook` `hosted` `webui` | Network surface: HTTP/WS API, outbound webhooks, multi-tenant auth, embedded SPA. |
| **Go frontends** | `tui` `uikit` `desktop` | In-repo Go UI surfaces (terminal, shared UI kit, desktop shell helpers). |
| **Wiring** | `app` | The only package allowed to import implementations. Wires everything together in `wire.go`. |

---

## Port pattern

Three subsystems fully follow the port pattern (port + implementation subpackages, enforced
by `depguard`):

| Port | Implementations |
|---|---|
| `platform` | `platform/twitch` `platform/kick` |
| `store` | `store/sqlite` `store/postgres` |
| `secrets` | `secrets/filevault` `secrets/keychain` |

Two were promoted to this pattern as part of the Option B restructure:

| Port | Implementations |
|---|---|
| `llm` | `llm/anthropic` `llm/openaicompat` |
| `search` | `search/meilisearch` `search/noop` |

`translate` is a single flat package with no second backend; it stays flat and is not guarded.

---

## Depguard rule

`.golangci.yml` enforces: outside `internal/app`, no file may import an implementation
subpackage (e.g. `platform/twitch`, `store/sqlite`, `llm/anthropic`). Only the port
(`platform`, `store`, `llm`) is freely importable. This is the central invariant of the
architecture — feature code depends on interfaces, only `app` depends on implementations.

---

## Plugin subsystem (`internal/plugin/`)

Four packages form one conceptual subsystem under `internal/plugin/`:

| Package | Was | Role |
|---|---|---|
| `plugin/host` | `pluginhost` | Manifest parsing, Git URL installer, WASM runtime, GUI sandbox, lifecycle. |
| `plugin/source` | `plugins` | The `DataSource` seam: how a server-side poller publishes `plugin.<id>.*` events onto the WS bus. |
| `plugin/markets` | `markets` | A concrete `DataSource` (Binance WS + CoinGecko REST crypto price ticks). |
| `plugin/xbridge` | `xbridge` | The `virtad` ↔ x-bridge subprocess protocol and supervisor for the X.com scrape bridge. |

---

## What NOT to do

- Do not re-nest the spine, enrichment, or persistence packages under layer directories
  (Option C). It rewrites 150+ import paths and adds no guarantee the compiler checks.
- Do not introduce a `pkg/` directory until there is a deliberate, supported Go client library
  to publish.
- Do not add `init()`-based registration or a DI framework; explicit wiring in `app/wire.go`
  is the rule.
