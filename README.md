# DBWatch

> `tail -f` for your Postgres database. Watch inserts, updates, and deletes in realtime while you develop.

## What is DBWatch?

DBWatch is a developer CLI tool that streams every INSERT, UPDATE, and DELETE from your Postgres database directly to your terminal — in realtime, with diff view for updates. Think of it as `tail -f` for your database.

When you're debugging code that touches Postgres, DBWatch shows you exactly what's changing and when, without writing a single query. It uses Postgres logical replication, so there's zero overhead on your application.

**This is a dev tool**, not a production observability solution. For production use cases, look at Debezium, pgaudit, or similar.

## Demo

```text
┌────────────────────────────────────────────────────────────────┐
│ dbwatch — mydb@localhost  •  ▶ live (45 events)                │
├──────────────┬─────────────────────────────────────────────────┤
│ Tables       │ 14:32:01.123  INSERT  orders     id=42          │
│ [x] orders   │ 14:32:01.156  INSERT  order_items id=87         │
│ [x] users    │ 14:32:01.189  UPDATE  inventory  stock 50 → 47  │
│ [ ] sessions │ 14:32:05.401  DELETE  cart_items id=7           │
│ [x] inventory│                                                 │
├──────────────┴─────────────────────────────────────────────────┤
│ space:pause  j/k:nav  enter:expand  f:filter  c:clear  q:quit  │
└────────────────────────────────────────────────────────────────┘
```

## Quick Start

```bash
# 1. Start Postgres with logical replication enabled
docker run -d --name pg-dev \
  -e POSTGRES_USER=local \
  -e POSTGRES_PASSWORD=local \
  -e POSTGRES_DB=test \
  -p 5432:5432 \
  postgres:16 -c wal_level=logical

# 2. Grant REPLICATION privilege to the user (one-time setup)
docker exec -i pg-dev psql -U local -d test -c "ALTER USER local REPLICATION;"

# 3. Install and run DBWatch
go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@latest
dbwatch tail --db-url="postgres://local:local@localhost:5432/test?sslmode=disable&replication=database"

# 4. In another terminal, make changes — watch them appear live
psql "postgres://local:local@localhost:5432/test?sslmode=disable" \
  -c "CREATE TABLE IF NOT EXISTS users (id int PRIMARY KEY, name text);" \
  -c "INSERT INTO users VALUES (1, 'alice');"
```

### Adapting the connection URL

The URL shape is:

```text
postgres://<user>:<password>@<host>:<port>/<database>?sslmode=disable&replication=database
```

Rename the four placeholders to match your setup. For the values used in Quick Start (`user=local`, `password=local`, `host=localhost`, `port=5432`, `database=test`) the full URL is:

```text
postgres://local:local@localhost:5432/test?sslmode=disable&replication=database
```

`sslmode=disable` is for local Postgres without TLS — drop it (or change it) for managed databases. `replication=database` is **required** for logical replication; do not omit it.

## Installation

### Via `go install`

```bash
go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@latest
```

### Via Docker

```bash
docker run --rm -it ghcr.io/rifqiagniamubarok/dbwatcher:latest tail --db-url=...
```

### Manual download

Download the binary for your platform from [GitHub Releases](https://github.com/rifqiagniamubarok/dbwatcher/releases).

## Postgres Setup

DBWatch uses Postgres logical replication. Your database needs:

### 1. Enable logical replication

In `postgresql.conf`:

```ini
wal_level = logical
```

Then restart Postgres.

For Docker, add `-c wal_level=logical` to your `docker run` command.

### 2. User privileges

```sql
ALTER USER myuser REPLICATION;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO myuser;
```

### 3. Create a publication

DBWatch creates this automatically, or you can create it manually:

```sql
CREATE PUBLICATION dbwatch_pub FOR ALL TABLES;
```

### 4. Optional: full diff for UPDATE/DELETE

By default, UPDATE and DELETE only include the primary key in old values. For full row diffs:

```sql
ALTER TABLE mytable REPLICA IDENTITY FULL;
```

### Quick dev setup with Docker

```bash
docker run -d \
  --name pg-dev \
  -e POSTGRES_PASSWORD=dev \
  -p 5432:5432 \
  postgres:16 \
  -c wal_level=logical
```

## Usage

```bash
dbwatch tail [flags]
```

### Background mode

If you want DBWatch to keep collecting events while you close and reopen terminals — or watch from more than one terminal at once — run it as a daemon:

```bash
# Start in the background
dbwatch daemon start --db-url="postgres://...&replication=database" --detach

# Check it's healthy
dbwatch daemon status
# running (pid 4521, uptime 3m12s, events 142, clients 0)

# Attach a live TUI (Ctrl+C / q leaves the daemon running)
dbwatch attach

# Or attach as a JSON stream
dbwatch attach --output=json | jq 'select(.table=="orders")'

# Tail the daemon's own log file
dbwatch daemon logs --follow

# Stop it
dbwatch daemon stop
```

Multiple daemons can run side-by-side via `--name`:

```bash
dbwatch daemon start --name=myapp     --db-url=... --detach
dbwatch daemon start --name=analytics --db-url=... --detach
dbwatch daemon list
dbwatch attach --name=myapp
```

Runtime files (one set per `--name`):

- Socket: `$XDG_RUNTIME_DIR/dbwatch/<name>.sock` (fallback `~/.dbwatch/<name>.sock`)
- PID:    `$XDG_RUNTIME_DIR/dbwatch/<name>.pid` (fallback `~/.dbwatch/<name>.pid`)
- Log:    `$XDG_RUNTIME_DIR/dbwatch/<name>.log` (fallback `~/.dbwatch/<name>.log`)

Override the runtime directory with `DBWATCH_SOCKET_DIR`. For Linux systemd or macOS launchd setup, see [`docs/dbwatch.service`](./docs/dbwatch.service) and [`docs/dbwatch.plist`](./docs/dbwatch.plist).

> The daemon currently runs on Linux and macOS. The lifecycle helpers use POSIX signals (`syscall.Kill`), so full Windows support for `daemon start/stop` is not in this release — use `dbwatch tail` on Windows.

### Flags

Used by `tail` and `daemon start`:

| Flag | Env var | Default | Description |
| --- | --- | --- | --- |
| `--db-url` | `DBWATCH_DB_URL` | — | Postgres connection URL (required) |
| `--publication` | `DBWATCH_PUBLICATION` | `dbwatch_pub` | Publication name |
| `--slot` | `DBWATCH_SLOT` | `dbwatch_slot` | Replication slot name |
| `--buffer` | `DBWATCH_BUFFER` | `1000` | Event ring buffer size |
| `--log-level` | `DBWATCH_LOG_LEVEL` | `warn` | Log level: debug, info, warn, error |

Used by `tail` and `attach`:

| Flag | Default | Description |
| --- | --- | --- |
| `--output` | `auto` | Output mode: `auto`, `tui`, `json` |

Used by daemon / attach commands:

| Flag | Default | Description | Commands |
| --- | --- | --- | --- |
| `--name` | `default` | Daemon instance name (drives socket / PID / log paths) | all `daemon`, `attach` |
| `--detach` | `false` | Fork into the background after starting | `daemon start` |
| `--follow` | `false` | Tail the log file instead of printing once | `daemon logs` |

### Connection URL format

```text
postgres://user:password@host:port/database?sslmode=disable&replication=database
```

Note: `replication=database` is required in the URL for logical replication.

### Pipe-friendly mode

When stdout is not a TTY (piped to another command), DBWatch automatically outputs one JSON event per line:

```bash
dbwatch tail --db-url=... | jq 'select(.table == "orders")'
dbwatch tail --db-url=... --output=json > events.log
```

## Keybindings

| Key | Action |
| --- | --- |
| `j` / `↓` | Move cursor down |
| `k` / `↑` | Move cursor up |
| `g` | Jump to oldest event |
| `G` | Jump to newest event |
| `enter` | Expand / collapse event detail |
| `space` | Pause / resume live feed |
| `f` | Toggle filter sidebar focus |
| `space` (in filter) | Toggle table visibility |
| `esc` / `f` (in filter) | Return to feed |
| `c` | Clear feed (press `c` again to confirm) |
| `?` | Toggle help overlay |
| `q` / `Ctrl+C` | Quit |

## Troubleshooting

### Cannot connect

```text
Cannot connect to Postgres.
  → Is the database running and accessible at the given address?
```

- Check that Postgres is running: `pg_isready -h localhost -p 5432`
- Verify the connection URL is correct
- Try connecting with `psql` first to isolate the issue

### wal_level error

```text
Postgres is not configured for logical replication.
  → Set wal_level=logical in postgresql.conf and restart Postgres.
```

Check current setting: `SHOW wal_level;`

### REPLICATION privilege

```text
The database user does not have REPLICATION privilege.
  → Run: ALTER USER <your-user> REPLICATION;
```

### Replication slot already exists

```text
Replication slot already exists (possibly from a previous crashed run).
  → Drop it: SELECT pg_drop_replication_slot('<slot-name>');
```

### TLS error

Add `?sslmode=disable` to your connection URL if your local Postgres doesn't use TLS:

```text
postgres://user:pass@localhost:5432/db?sslmode=disable&replication=database
```

## Architecture

See [`ARCHITECTURE.md`](./ARCHITECTURE.md) for technical design details, and [`CHANGELOG.md`](./CHANGELOG.md) for what shipped in each release.

## Development

```bash
# Run from source
go run ./cmd/dbwatch tail --db-url=...

# Build binary
make build

# Run tests
make test

# Start test Postgres (Docker required, listens on localhost:5433)
./scripts/start-postgres.sh
```

See [`CONTRIBUTING.md`](./CONTRIBUTING.md) for branch strategy, commit conventions, and the release process.

## License

MIT — see [`LICENSE`](./LICENSE).
