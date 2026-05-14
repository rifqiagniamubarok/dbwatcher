# DBWatch — Architecture

## Overview

DBWatch is a CLI tool for monitoring Postgres database changes in realtime. It is designed for **development environments**, not production observability. Core philosophy: `tail -f` for Postgres.

When a developer is testing or debugging code that touches Postgres, DBWatch displays every INSERT, UPDATE, and DELETE live in the terminal — with diff view for UPDATE events. This gives developers (and AI agents like Claude Code) direct visibility into the side effects of the code they're working on.

## Goals & non-goals

### Goals

- **Realtime:** event latency from Postgres to terminal < 1 second
- **Zero friction:** `dbwatch tail` is all you need to get started
- **Minimal dependencies:** single binary, embedded UI, no external services required
- **Reusable core:** Listener and Store are designed to be reused for a Web UI in future phases

### Non-goals

- Production audit trail (use Debezium, pgaudit, etc. for that)
- Persistent storage / long-term retention
- Authentication, RBAC, multi-tenant
- High availability, clustering
- Multi-database support in MVP (Postgres only)

## High-level architecture

```
┌─────────────────┐         ┌──────────────────────────────────────┐
│                 │         │  DBWatch binary (Go)                 │
│  Postgres       │         │                                      │
│  dev database   │ ──────▶ │  ┌────────────────────────────────┐  │
│                 │ pgoutput│  │ Listener                       │  │
│  wal_level=     │         │  │ - WAL streaming                │  │
│  logical        │         │  │ - Schema cache                 │  │
│                 │         │  │ - Decode INSERT/UPDATE/DELETE  │  │
└─────────────────┘         │  └──────────────┬─────────────────┘  │
                            │                 │ push                │
                            │                 ▼                     │
                            │  ┌────────────────────────────────┐  │
                            │  │ Store                          │  │
                            │  │ - Ring buffer (1000 events)    │  │
                            │  │ - Pub/sub channel              │  │
                            │  │ - Filter logic                 │  │
                            │  └──────────────┬─────────────────┘  │
                            │                 │ subscribe           │
                            │                 ▼                     │
                            │  ┌────────────────────────────────┐  │
                            │  │ Renderer (Bubble Tea TUI)      │  │
                            │  │ - Live feed                    │  │
                            │  │ - Filter sidebar               │  │
                            │  │ - Diff view (UPDATE)           │  │
                            │  │ - Keybindings                  │  │
                            │  └──────────────┬─────────────────┘  │
                            └─────────────────┼────────────────────┘
                                              │
                                              ▼
                                    ┌──────────────────┐
                                    │ Developer's      │
                                    │ terminal         │
                                    └──────────────────┘
```

All components inside the DBWatch binary live in a single process and communicate via Go channels (in-memory). There are no network calls between components and no internal serialization.

## Component breakdown

### Listener

**Responsibility:** Connect to Postgres as a logical replication consumer, read the WAL stream, and decode binary pgoutput into meaningful `Event` structs.

**How it works:**

1. On startup, query `information_schema.columns` for all tables in the public schema. Cache the results in a map keyed by table OID.
2. Create a temporary replication slot (auto-dropped on disconnect) or use an existing one.
3. Start streaming from the current LSN.
4. For each pgoutput message:
   - `RelationMessage` — update schema cache if not seen before
   - `InsertMessage`, `UpdateMessage`, `DeleteMessage` — decode into `Event`, push to Store
   - `BeginMessage`, `CommitMessage` — track transaction context (used for grouping in v0.2)
5. Acknowledge LSN periodically to prevent Postgres from accumulating WAL.

**Dependencies:** `github.com/jackc/pglogrepl`, `github.com/jackc/pgx/v5`

**Edge cases handled:**

- TOAST values (large columns not shipped in UPDATE if unchanged) → displayed as `"[unchanged]"`
- Replica identity not FULL → old values unavailable for UPDATE, shown with a warning
- Schema change at runtime (`ALTER TABLE`) → cache refreshed on new `RelationMessage`
- Connection dropped → returns error for caller to handle retry

### Store

**Responsibility:** Keep recent events in memory and distribute them to all active subscribers.

**How it works:**

1. Maintain a circular slice with a fixed capacity (default 1000). Oldest events are automatically overwritten.
2. Maintain a list of subscriber channels. When a new event arrives:
   - Append to the ring buffer
   - Broadcast to all subscriber channels (non-blocking — drop if channel is full)
3. Exposed methods:
   - `Push(event Event)` — called by Listener
   - `Subscribe() <-chan Event` — called by Renderer
   - `Unsubscribe(ch <-chan Event)` — called when subscriber is done
   - `Snapshot() []Event` — returns all buffered events (used for initial render)

**Concurrency:** Uses `sync.RWMutex` for access to the slice and subscriber list. Not a bottleneck since event volume in dev environments is low.

**Design note:** Store intentionally knows nothing about event format or who is subscribing. This makes it reusable when a Web UI is added — just subscribe a new channel.

### Renderer (TUI)

**Responsibility:** Render the terminal UI, handle user input, and subscribe to the Store for realtime updates.

**Architecture:** Follows the Elm/Bubble Tea pattern — `Model`, `Update`, `View`.

**State (Model):**

```go
type Model struct {
    events       []Event          // local snapshot from Store
    cursor       int              // currently highlighted event
    expanded     bool             // detail view open?
    paused       bool             // freeze incoming events?
    filter       map[string]bool  // active tables (true = visible)
    tables       []string         // all tables seen so far
    focus        Focus            // FOCUS_FEED or FOCUS_FILTER
    width, height int
}
```

**Layout:**

```
┌────────────────────────────────────────────────────────────────┐
│ dbwatch — connected to mydb@localhost  •  ▶ live (45 events)   │  header
├──────────────┬─────────────────────────────────────────────────┤
│ Tables       │ 14:32:01.123  INSERT  orders     id=42 ...      │
│ [x] orders   │ 14:32:01.156  INSERT  order_items ...           │
│ [x] users    │ 14:32:01.189  UPDATE  inventory  stock 50 → 47  │  feed
│ [ ] sessions │ 14:32:05.401  DELETE  cart_items user_id=7      │
│ [x] inventory│ ▶ ...                                           │
├──────────────┴─────────────────────────────────────────────────┤
│ space:pause  j/k:nav  enter:expand  f:filter  c:clear  q:quit  │  footer
└────────────────────────────────────────────────────────────────┘
```

**Keybindings:**

- `j` / `↓` — move to next event
- `k` / `↑` — move to previous event
- `enter` — toggle expanded detail view
- `space` — pause / resume feed
- `f` — toggle focus to filter sidebar
- `c` — clear feed (requires confirmation)
- `q` / `Ctrl+C` — quit

**Color coding (lipgloss):**

- INSERT: green
- UPDATE: yellow
- DELETE: red
- Table name: cyan, bold
- Timestamp: gray, dim
- Diff old value: red
- Diff new value: green

**Non-TTY mode:** If `stdout` is not a terminal (piped to `jq`, `grep`, or a file), the Renderer does not start the TUI. Instead, it prints each event as a JSON line to stdout, making the tool pipe-friendly:

```bash
dbwatch tail --db-url=... | jq 'select(.table == "orders")'
```

## Data model

### Event

```go
type EventType string

const (
    EventInsert EventType = "INSERT"
    EventUpdate EventType = "UPDATE"
    EventDelete EventType = "DELETE"
)

type Event struct {
    ID        uint64            // sequence number, monotonically increasing
    Timestamp time.Time         // time the event was received
    LSN       string            // Postgres LSN, for debugging
    Type      EventType
    Schema    string            // e.g. "public"
    Table     string            // e.g. "orders"
    Columns   []Column          // column metadata
    NewValues map[string]any    // for INSERT and UPDATE
    OldValues map[string]any    // for UPDATE and DELETE (requires REPLICA IDENTITY FULL)
    TxID      uint32            // transaction ID, for future grouping
}

type Column struct {
    Name     string
    DataType string
    IsKey    bool
}
```

### Diff (computed at render time)

For UPDATE events, the Renderer compares `OldValues` and `NewValues` at render time — not stored in the Event itself. This saves memory and keeps the diff strategy flexible.

## Configuration

All config is via environment variables and command-line flags. Flags override env vars. No config file in MVP.

| Setting | Env var | Flag | Default | Required |
| --- | --- | --- | --- | --- |
| Database URL | `DBWATCH_DB_URL` | `--db-url` | — | yes |
| Publication name | `DBWATCH_PUBLICATION` | `--publication` | `dbwatch_pub` | no |
| Replication slot | `DBWATCH_SLOT` | `--slot` | `dbwatch_slot` | no |
| Buffer size | `DBWATCH_BUFFER` | `--buffer` | `1000` | no |
| Log level | `DBWATCH_LOG_LEVEL` | `--log-level` | `warn` | no |
| Output format | — | `--output` | `auto` | no |

## Postgres requirements

1. `wal_level = logical` in `postgresql.conf` (requires restart)
2. `max_replication_slots >= 1` (default is usually sufficient)
3. User must have `REPLICATION` privilege and `SELECT` on target tables
4. A publication covering the tables to watch:

   ```sql
   CREATE PUBLICATION dbwatch_pub FOR ALL TABLES;
   ```

5. (Optional, for full diffs on UPDATE/DELETE) `REPLICA IDENTITY FULL` per table:

   ```sql
   ALTER TABLE mytable REPLICA IDENTITY FULL;
   ```

For a Docker-based dev environment:

```bash
docker run -d \
  -e POSTGRES_PASSWORD=dev \
  -p 5432:5432 \
  postgres:16 \
  -c wal_level=logical
```

## Distribution

Three distribution channels:

1. **Single binary** via `go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@latest`
2. **Docker image** via `docker run -it ghcr.io/rifqiagniamubarok/dbwatcher tail --db-url=...`
3. **GitHub Releases** with pre-built binaries for Linux, macOS, Windows × amd64, arm64

All built automatically by GoReleaser triggered by a git tag.

## Folder structure

```
dbwatcher/
├── cmd/
│   └── dbwatch/
│       └── main.go              # entry point, Cobra wiring
├── internal/
│   ├── listener/
│   │   ├── listener.go          # logical replication consumer
│   │   ├── decoder.go           # pgoutput → Event
│   │   ├── schema_cache.go      # column metadata cache
│   │   └── errors.go            # user-friendly error messages
│   ├── store/
│   │   ├── store.go             # ring buffer + pub/sub
│   │   └── event.go             # Event and Column structs
│   ├── tui/
│   │   ├── app.go               # Bubble Tea Model + Update + View
│   │   ├── feed.go              # event list component
│   │   ├── filter.go            # filter sidebar component
│   │   ├── detail.go            # detail/diff component
│   │   └── styles.go            # lipgloss styles
│   └── config/
│       └── config.go            # parse env vars and flags
├── scripts/
│   ├── start-postgres.sh        # spin up test Postgres
│   └── seed.sql                 # test schema
├── .github/workflows/
│   ├── ci.yml                   # run tests on push/PR
│   └── release.yml              # GoReleaser on tag push
├── go.mod
├── go.sum
├── Makefile
├── Dockerfile                   # for local docker build
├── Dockerfile.goreleaser        # for GoReleaser (binary already built)
├── .goreleaser.yml
├── README.md
├── CONTRIBUTING.md
├── CHANGELOG.md
└── LICENSE
```

## Tech stack

| Concern | Choice | Rationale |
| --- | --- | --- |
| Language | Go 1.22+ | Mature Postgres replication ecosystem, easy cross-compile, single binary |
| Postgres replication | `jackc/pglogrepl` | De facto library for logical replication in Go |
| Postgres driver | `jackc/pgx/v5` | Modern, pairs well with pglogrepl |
| TUI framework | `charmbracelet/bubbletea` | Elm-style architecture, rich ecosystem (lipgloss, bubbles) |
| CLI parsing | `spf13/cobra` | Industry standard for Go CLI tools |
| Config | env var + flag | Simple, no Viper dependency |
| Logging | `log/slog` (stdlib) | Built-in since Go 1.21, no third-party dep |
| Testing | stdlib `testing` + `stretchr/testify` | Standard combination |
| Build & release | Makefile + GoReleaser | Auto multi-platform build + Docker + GitHub Releases |

## Future extension points

This architecture is designed so the following features can be added **without changing the Listener or Store**:

- **Web UI** — add `internal/server/` with HTTP + WebSocket, subscribe to the same Store
- **Session correlation** — add `ApplicationName` and `Pid` fields to Event, decoded from Postgres metadata
- **MCP server** — add `internal/mcp/` exposing Store as an MCP resource for AI agents
- **Persistent storage** — add a new Store subscriber that writes to SQLite, without replacing the ring buffer
- **Multi-database** — refactor Listener into an interface, implement `postgres_listener.go` and `mysql_listener.go` separately

All extensions above are **additive**. The core application does not need to change.
