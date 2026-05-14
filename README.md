# DBWatch

Watch every database change happen live in your terminal — like `tail -f` but for Postgres.

```text
┌────────────────────────────────────────────────────────────────┐
│ dbwatch — test@localhost  •  ▶ live (12 events)                │
├──────────────┬─────────────────────────────────────────────────┤
│ Tables       │ 14:32:01.123  INSERT  users      id=1           │
│ [x] users    │ 14:32:03.456  UPDATE  users      email old→new  │
│ [x] orders   │ 14:32:05.789  DELETE  orders     id=7           │
├──────────────┴─────────────────────────────────────────────────┤
│ space:pause  j/k:nav  enter:expand  f:filter  c:clear  q:quit  │
└────────────────────────────────────────────────────────────────┘
```

---

## Before you start

Your Postgres needs `wal_level = logical`. Check it:

```bash
psql YOUR_DB_URL -c "SHOW wal_level;"
```

If the result is **`logical`** → skip to [Running DBWatch](#running-dbwatch).

If the result is **`replica`** or **`minimal`** → follow the one-time setup below.

---

## One-time Postgres setup

### Using Homebrew Postgres

```bash
# 1. Enable logical replication
psql YOUR_DB_URL -c "ALTER SYSTEM SET wal_level = logical;"

# 2. Restart Postgres (replace @14 with your version)
brew services restart postgresql@14

# 3. Confirm it worked
psql YOUR_DB_URL -c "SHOW wal_level;"
# should print: logical
```

### Using Docker

Add `-c wal_level=logical` when starting the container:

```bash
docker run -d --name my-pg \
  -e POSTGRES_PASSWORD=yourpassword \
  -p 5432:5432 \
  postgres:16 -c wal_level=logical
```

### Grant your user REPLICATION access

```bash
psql YOUR_DB_URL -c "ALTER USER your_username REPLICATION;"
```

---

## Running DBWatch

```bash
# Build first (only once)
make build

# Run — replace with your actual credentials
./bin/dbwatch tail \
  --db-url="postgres://USERNAME:PASSWORD@localhost:5432/DBNAME?sslmode=disable&replication=database"
```

**Example** with username `local`, password `local`, database `test`:

```bash
./bin/dbwatch tail \
  --db-url="postgres://local:local@localhost:5432/test?sslmode=disable&replication=database"
```

The TUI will open automatically. Now go make some changes in your database and watch them appear live.

---

## Navigating the TUI

| Key | What it does |
| --- | --- |
| `j` / `↓` | Move down |
| `k` / `↑` | Move up |
| `enter` | Expand event to see full diff |
| `space` | Pause / resume the live feed |
| `f` | Open table filter (show/hide specific tables) |
| `g` / `G` | Jump to oldest / newest event |
| `c` | Clear all events (press `c` twice to confirm) |
| `?` | Show help |
| `q` | Quit |

---

## Using it as a pipe (JSON mode)

When you pipe the output to another command, DBWatch automatically switches to JSON mode — one event per line:

```bash
# Filter only orders table
./bin/dbwatch tail --db-url="..." | jq 'select(.table == "orders")'

# Save to file
./bin/dbwatch tail --db-url="..." --output=json > events.log
```

---

## All flags

| Flag | Default | Description |
| --- | --- | --- |
| `--db-url` | *(required)* | Your Postgres connection URL |
| `--slot` | `dbwatch_slot` | Replication slot name |
| `--publication` | `dbwatch_pub` | Publication name |
| `--output` | `auto` | `auto`, `tui`, or `json` |
| `--buffer` | `1000` | How many events to keep in memory |
| `--log-level` | `warn` | `debug`, `info`, `warn`, `error` |

You can also set `--db-url` via the `DBWATCH_DB_URL` environment variable instead of passing it every time.

---

## Troubleshooting

**"Cannot connect to Postgres"**
→ Make sure Postgres is running and the URL is correct. Try `psql YOUR_URL` first.

**"Postgres is not configured for logical replication"**
→ Follow the [one-time setup](#one-time-postgres-setup) above.

**"User does not have REPLICATION privilege"**
→ Run: `ALTER USER your_username REPLICATION;`

**"Replication slot already exists"**
→ A previous run crashed. Drop the slot and retry:

```bash
psql YOUR_DB_URL -c "SELECT pg_drop_replication_slot('dbwatch_slot');"
```

**TLS error**
→ Add `?sslmode=disable` to your URL (most local databases don't need TLS).

---

## License

MIT
