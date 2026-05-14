# DBWatch — Architecture

## Overview

DBWatch is a CLI tool for monitoring Postgres database changes in realtime. It targets **development environments**, not production observability. Its core idea: `tail -f` for Postgres.

While a developer tests or debugs, DBWatch shows every INSERT, UPDATE, and DELETE happening in the database live in the terminal, with a diff view for UPDATE. This gives developers (and AI agents like Claude Code) immediate visibility into the side effects of the code they're working on.

## Goals & non-goals

### Goals

- Realtime: event latency from Postgres to terminal < 1 second
- Zero friction: `docker run` or `dbwatch tail` and you're going
- Minimal dependencies: one binary, embedded UI, no external service required
- Reusable core: Listener and Store are designed so they can be reused for a Web UI in later phases

### Non-goals

- Production audit trail (Debezium, pgaudit, etc. already exist)
- Persistent storage / long-term retention
- Authentication, RBAC, multi-tenant
- High availability, clustering
- Multi-database support in the MVP (Postgres only)

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

All components inside the DBWatch binary live in one process and talk through Go channels (in-memory). There are no network calls between components and no internal serialization.

## Component breakdown

### Listener

**Responsibility:** Connect to Postgres as a logical replication consumer, read the WAL stream, decode binary pgoutput into meaningful `Event` structs.

**How it works:**

1. On startup, query `information_schema.columns` for every table in the public schema. Cache the result in a map keyed by table OID.
2. Create a temporary replication slot (auto-cleaned on disconnect) or reuse an existing one.
3. Start streaming from the latest LSN.
4. For each pgoutput message:
   - `RelationMessage`: update the schema cache if missing
   - `InsertMessage`, `UpdateMessage`, `DeleteMessage`: decode into an `Event` and push to the Store
   - `BeginMessage`, `CommitMessage`: track transaction context (for grouping in v0.2)
5. Acknowledge the LSN periodically so Postgres doesn't accumulate WAL.

**Dependencies:** `github.com/jackc/pglogrepl`, `github.com/jackc/pgx/v5`.

**Edge cases that must be handled:**

- TOAST values (large columns not shipped on UPDATE when unchanged) → render as "[unchanged]"
- Replica identity not FULL → old values are unavailable on UPDATE; show what we have with a warning
- Schema change at runtime (ALTER TABLE) → refresh the cache when a new RelationMessage arrives
- Connection drop → retry with exponential backoff

### Store

**Responsibility:** Keep recent events in memory and distribute them to every active subscriber.

**How it works:**

1. Maintain a circular slice with a fixed capacity (default 1000). The oldest event is overwritten automatically.
2. Maintain a list of subscriber channels. On a new event:
   - Append to the ring buffer
   - Broadcast to every subscriber channel (non-blocking, drop if a channel is full)
3. Public API:
   - `Push(event Event)` — called by the Listener
   - `Subscribe() <-chan Event` — called by the Renderer
   - `Unsubscribe(ch <-chan Event)` — when a subscriber is done
   - `Snapshot() []Event` — all events still in the ring (for initial render)

**Concurrency:** `sync.Mutex` for slice and subscriber-list access. Not a bottleneck because event volume in a dev environment is low.

**Design note:** The Store deliberately knows nothing about event formats or who is subscribing. That's what makes it reusable when the Web UI lands — just subscribe a new channel.

### Renderer (TUI)

**Responsibility:** Render the UI in the terminal, handle user interaction, subscribe to the Store for realtime updates.

**Architecture:** Elm / Bubble Tea pattern — `Model`, `Update`, `View`.

**State (Model):**

```go
type Model struct {
    events       []Event              // local snapshot from the Store
    cursor       int                  // currently highlighted event
    expanded     bool                 // detail view open?
    paused       bool                 // freeze incoming events?
    filter       map[string]bool      // active tables (true = visible)
    tables       []string             // every table seen so far
    focus        Focus                // FOCUS_FEED or FOCUS_FILTER
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

- `j` / `↓` — move to the next event
- `k` / `↑` — move to the previous event
- `enter` — toggle detail expand
- `space` — pause / resume the feed
- `f` — toggle focus to the filter sidebar
- `c` — clear the feed
- `q` / `Ctrl+C` — quit

**Color coding (lipgloss):**

- INSERT: green
- UPDATE: yellow
- DELETE: red
- Table name: cyan, bold
- Timestamp: gray, dim
- Diff old value: red / strikethrough
- Diff new value: green

**Non-TTY mode:** If `stdout` is not a terminal (piped to `jq`, `grep`, or a file), the Renderer does not start the TUI. Instead, every event is printed as a JSON line to stdout. This makes the tool pipe-friendly:

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
    Timestamp time.Time         // when the event was received
    LSN       string            // Postgres LSN, for debugging
    Type      EventType
    Schema    string            // e.g. "public"
    Table     string            // e.g. "orders"
    Columns   []Column          // column metadata
    NewValues map[string]any    // for INSERT and UPDATE
    OldValues map[string]any    // for UPDATE and DELETE (if REPLICA IDENTITY FULL)
    TxID      uint32            // transaction ID, for later grouping
}

type Column struct {
    Name     string
    DataType string
    IsKey    bool
}
```

### Diff (computed at render time)

For UPDATE events, the Renderer compares `OldValues` and `NewValues` at runtime — the diff is not stored on the Event. This saves memory and stays flexible if the diff strategy changes.

## Configuration

All config flows through environment variables and command-line flags. Flags override env vars. No config file in the MVP.

| Setting | Env var | Flag | Default | Required |
|---|---|---|---|---|
| Database URL | `DBWATCH_DB_URL` | `--db-url` | — | yes |
| Publication name | `DBWATCH_PUBLICATION` | `--publication` | `dbwatch_pub` | no |
| Replication slot | `DBWATCH_SLOT` | `--slot` | `dbwatch_slot` | no |
| Buffer size | `DBWATCH_BUFFER` | `--buffer` | `1000` | no |
| Log level | `DBWATCH_LOG_LEVEL` | `--log-level` | `warn` | no |
| Output format | — | `--output` | `tui` (or `json` if non-TTY) | no |

## Postgres setup requirements

The tool requires the following Postgres configuration. It is documented clearly in the README:

1. `wal_level = logical` in `postgresql.conf` (requires restart)
2. `max_replication_slots >= 1` (the default is usually enough)
3. A user with `REPLICATION` and `SELECT` privilege on the tables to watch
4. A publication covering the tables to watch:

   ```sql
   CREATE PUBLICATION dbwatch_pub FOR ALL TABLES;
   ```

5. (Optional, for a full diff on UPDATE/DELETE) `ALTER TABLE foo REPLICA IDENTITY FULL;` per table

For a dev environment with Postgres in Docker, an example command:
```bash
docker run -d \
  -e POSTGRES_PASSWORD=test \
  -p 5432:5432 \
  postgres:16 \
  -c wal_level=logical
```

## Distribution

Three distribution channels:

1. **Single binary** via `go install github.com/<user>/dbwatch/cmd/dbwatch@latest`
2. **Docker image** via `docker run -it ghcr.io/<user>/dbwatch tail --db-url=...`
3. **GitHub Releases** with prebuilt binaries for Linux, macOS, Windows × amd64, arm64

Everything is built automatically by GoReleaser, triggered by a git tag.

## Folder structure

```
dbwatch/
├── cmd/
│   └── dbwatch/
│       └── main.go              # entry point, wire Cobra
├── internal/
│   ├── listener/
│   │   ├── listener.go          # logical replication consumer
│   │   ├── decoder.go           # pgoutput → Event
│   │   └── schema_cache.go      # column metadata cache
│   ├── store/
│   │   ├── store.go             # ring buffer + pub/sub
│   │   └── event.go             # Event, Column structs
│   ├── tui/
│   │   ├── app.go               # Bubble Tea Model + Update
│   │   ├── feed.go              # event list component
│   │   ├── filter.go            # sidebar filter component
│   │   ├── detail.go            # detail / diff component
│   │   └── styles.go            # lipgloss styles
│   └── config/
│       └── config.go            # parse env + flag
├── go.mod
├── go.sum
├── Makefile
├── .goreleaser.yml
├── Dockerfile
├── README.md
└── LICENSE
```

## Tech stack summary

| Concern | Choice | Rationale |
|---|---|---|
| Language | Go 1.22+ | Mature Postgres replication ecosystem, easy cross-compile, single binary distribution |
| Postgres replication | `jackc/pglogrepl` | De facto library for logical replication in Go |
| Postgres driver | `jackc/pgx/v5` | Modern, paired with pglogrepl |
| TUI framework | `charmbracelet/bubbletea` | Elm-style architecture, rich ecosystem (lipgloss, bubbles) |
| CLI parsing | `spf13/cobra` | Industry standard for Go CLI tools |
| Config | env var + flag | Simple, no Viper dependency |
| Logging | `log/slog` (stdlib) | Built-in since Go 1.21, no third-party dep |
| Testing | stdlib `testing` + `stretchr/testify` | Standard combination |
| Build & release | Makefile + GoReleaser | Auto multi-platform build + Docker + GitHub Release |

## Daemon mode (Phase 5)

Starting from Phase 5, DBWatch supports a daemon mode: a background process that runs the Listener and Store, plus TUI/JSON clients that attach over a Unix socket. The `tail` mode (all-in-one, foreground) remains the default and is unaffected.

### Architecture

```
┌────────────────────────────────────────┐
│  dbwatch daemon (background process)   │
│                                        │
│  Postgres ──▶ Listener ──▶ Store       │
│                              │         │
│                              ▼         │
│                         IPC Server     │
│                       (Unix socket)    │
└──────────────┬─────────────────────────┘
               │ NDJSON
       ┌───────┼───────┬───────────┐
       ▼       ▼       ▼           ▼
   attach   attach   attach     dbwatch serve
   (TUI)    (JSON)   (TUI #2)   (future Web UI)
```

Each client is just another subscriber on the same Store — no duplicate Listener, no extra Postgres connection. The daemon is the single source of truth; clients are lightweight views.

### IPC layer

New package `internal/ipc/`:

- **Transport:** Unix domain socket at `$XDG_RUNTIME_DIR/dbwatch/<name>.sock` (fallback `~/.dbwatch/<name>.sock`), permission `0600`. Windows 10+ supports AF_UNIX, so cross-platform stays on one path.
- **Wire format:** newline-delimited JSON. Each line is a message envelope:

  ```json
  {"type": "event",    "data": { /* Event */ }}
  {"type": "snapshot", "data": [ /* Event[] */ ]}
  {"type": "hello",    "data": {"version": "0.2.0", "uptime_s": 142}}
  {"type": "stats",    "data": {"events": 4521, "clients": 2}}
  {"type": "ping"}     {"type": "pong"}
  ```

- **Connection lifecycle:** the server sends `hello` + `snapshot` when a client connects, then streams `event` messages. Clients send `ping` periodically (15s) to keep the connection alive. The server closes the connection if it sees no ping for > 60s.
- **Backpressure:** the Store already drops events for slow subscribers when their channel is full. The same applies to the IPC server — if a socket is slow (client hung), events are dropped for that client only.

### Process model

| Mode | Listener | Store | TUI | IPC Server | IPC Client |
| --- | --- | --- | --- | --- | --- |
| `tail` (default) | ✓ | ✓ | ✓ | — | — |
| `daemon start` | ✓ | ✓ | — | ✓ | — |
| `attach` | — | — | ✓ | — | ✓ |

A small refactor is needed: extract a `runCore(ctx, cfg, store)` function that only starts the Listener. Then `tail` = runCore + local renderer; `daemon` = runCore + IPC server; `attach` = IPC client + local renderer.

**The Listener and Store don't change at all.** Phase 5 only adds the `internal/ipc/` package and two new subcommands.

### Folder structure additions

```
internal/
├── ipc/
│   ├── protocol.go        # message envelope, type constants
│   ├── server.go          # accept connections, fan out from Store
│   ├── client.go          # connect, expose <-chan Event
│   └── socket_path.go     # resolve socket path from --name
└── daemon/
    ├── lifecycle.go       # PID file, detach, log redirection
    └── status.go          # status / list helpers
```

### Configuration additions

| Setting | Flag | Default | Used by |
| --- | --- | --- | --- |
| Daemon name | `--name` | `default` | `daemon start/stop/status`, `attach` |
| Detach | `--detach` | `false` | `daemon start` |
| Socket dir | `DBWATCH_SOCKET_DIR` (env) | `$XDG_RUNTIME_DIR/dbwatch` or `~/.dbwatch` | all daemon/attach commands |

### Security model

- Socket file `0600`, owned by the user who started the daemon — kernel-level access control via filesystem permissions
- No protocol-layer auth (local-only); if an attacker can read the socket file, they already have access as the same user
- No TCP listener — the daemon **never** binds to a network address. For remote access, use SSH port-forwarding, or `dbwatch serve` (Phase 6) which is explicitly network-facing

## Future extension points

The architecture is designed so the features below can be added **without changing the Listener or the Store**:

- **Daemon mode (Phase 5):** see the section above. Adds `internal/ipc/` and `internal/daemon/`.
- **Web UI (Phase 6):** add `internal/server/` with HTTP + WebSocket. Can subscribe to the Store directly (standalone mode) or act as an IPC client to the daemon (shared mode).
- **Session correlation:** add `ApplicationName` and `Pid` fields on Event. The Listener decodes them from Postgres metadata.
- **MCP server:** add `internal/mcp/` that exposes the Store as an MCP resource for AI agents. Could also be implemented as an IPC client to the daemon.
- **Persistent storage:** add a new subscriber to the Store that writes to SQLite. Does not replace the ring buffer.
- **Multi-database:** refactor the Listener into an interface, implement `postgres_listener.go` and `mysql_listener.go` separately.

All of the above are **additive**. The core does not need to change.
