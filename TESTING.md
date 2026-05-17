# TESTING

Testing guide for DBWatch. This file evolves with the project.

## Test layers

DBWatch has three test layers:

1. **Unit test** — package-level, no external dependencies
2. **Integration test** — requires a running Postgres, tagged `integration`
3. **Manual test** — end-to-end scenarios, run at the end of every phase

## Running tests

### Unit test

```bash
make test
# or
go test ./... -race -count=1
```

`-race` is required to detect data races. `-count=1` prevents the test cache from hiding problems.

### Integration test

```bash
# Start the Postgres test instance first
./scripts/start-postgres.sh

# Run the integration tests
go test -tags=integration ./... -race
```

### Coverage

```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Minimum coverage targets:

- `internal/store/` — 80%
- `internal/listener/decoder.go` — 70%
- `internal/listener/schema_cache.go` — 70%
- Everything else — best effort

## Test database setup

`scripts/start-postgres.sh` does the following:

1. Stop and remove the `dbwatch-test-pg` container if it exists
2. Start Postgres 16 in Docker with `wal_level=logical`
3. Wait until it accepts connections
4. Run `scripts/seed.sql` to create the sample tables

Tables created:

- `users` (id, name, email, created_at)
- `orders` (id, user_id, total, status, created_at)
- `order_items` (id, order_id, product_id, qty, price)
- `inventory` (id, product_id, stock, updated_at)

Connection string: `postgres://test:test@localhost:5432/test`

## Manual test scenarios

Run the scenarios below after each phase is complete. Tick `[x]` once verified.

### Phase 0 — Skeleton

- [ ] `make build` succeeds
- [ ] `./bin/dbwatch --help` shows a help message
- [ ] `./bin/dbwatch tail` shows a placeholder message
- [ ] `./bin/dbwatch version` prints the version
- [ ] `make docker-build` succeeds
- [ ] `docker run --rm dbwatch:dev version` prints the version

### Phase 1 — Listener core

- [ ] Connects to the Postgres test instance successfully
- [ ] An INSERT on `users` appears as a JSON event on stdout
- [ ] An UPDATE on `users` shows up with both `old_values` and `new_values`
- [ ] A DELETE on `users` shows up with `old_values`
- [ ] Several consecutive INSERTs are all captured, in order, with no losses
- [ ] Disconnecting Postgres while dbwatch is running produces a friendly error and exits with a non-zero code
- [ ] SIGINT (Ctrl+C) shuts down cleanly with no goroutine leak
- [ ] Output can be piped to `jq`: `dbwatch tail ... | jq .table`

### Phase 2 — Store layer

- [ ] Behavior from the user's point of view is unchanged from Phase 1
- [ ] Periodic stats appear on stderr with reasonable numbers
- [ ] `go test -race ./internal/store/...` passes

### Phase 3 — TUI

- [ ] `dbwatch tail` enters TUI mode automatically
- [ ] INSERT appears in green
- [ ] UPDATE appears in yellow; expanding it shows the diff
- [ ] DELETE appears in red
- [ ] `j`/`k` move the cursor
- [ ] `enter` toggles the detail view
- [ ] `space` pauses/resumes; events arriving while paused appear after resume
- [ ] `f` toggles sidebar focus; navigation and table-filter toggling work
- [ ] `c` clears the feed (with a confirmation press)
- [ ] `q` quits cleanly
- [ ] Resizing the terminal keeps the layout tidy
- [ ] `dbwatch tail | head -5` works in JSON mode (non-TTY detected)
- [ ] `dbwatch tail --output=json` forces JSON even on a TTY
- [ ] `dbwatch tail --output=tui` forces TUI (when run on a TTY)

### Phase 4 — Release

- [ ] `go install github.com/<user>/dbwatch/cmd/dbwatch@v0.1.0` succeeds from a clean Go environment
- [ ] `docker run ghcr.io/<user>/dbwatch:v0.1.0 version` succeeds
- [ ] GitHub Releases binaries run on Linux, macOS (Intel & Apple Silicon)
- [ ] README quick start from scratch — events appear with no surprises
- [ ] Every error message tested against its failure scenario (wrong URL, permission denied, etc.)

### Phase 5 — Daemon mode

- [ ] `dbwatch daemon start --db-url=... --detach` returns immediately and writes a PID file
- [ ] `dbwatch daemon status` reports the running daemon (pid, uptime, event count)
- [ ] `dbwatch attach` connects and shows the TUI with the initial snapshot
- [ ] Two `dbwatch attach` clients run simultaneously and both receive every event
- [ ] Closing an `attach` with `q` leaves the daemon running; reattaching works
- [ ] `dbwatch attach --output=json | jq ...` works without the TUI
- [ ] `dbwatch daemon stop` cleans up the socket and PID file
- [ ] Killing the daemon with SIGKILL leaves a stale socket; the next `daemon start` reports it clearly
- [ ] `dbwatch tail` (foreground all-in-one) still behaves exactly as before — no regression

### Phase 6 — Marker HTTP API

- [ ] `curl http://localhost:6677/health` returns 200 with `status:"ok"` while a daemon or tail is running
- [ ] `curl -X POST localhost:6677/marker -d "TEST"` produces a separator line in the TUI feed (default color)
- [ ] `curl -X POST -H "Content-Type: application/json" localhost:6677/marker -d '{"label":"x","color":"yellow"}'` produces a yellow separator
- [ ] `curl -X POST localhost:6677/log -d "starting suite"` produces an inline `[log]` line (no separator)
- [ ] Invalid color (`{"color":"purple"}`) returns 400 with an actionable error
- [ ] Empty label returns 400; nothing is pushed to the feed
- [ ] Marker server is bound to `127.0.0.1`, not reachable from another host on the LAN
- [ ] `--no-marker` disables the server; nothing listens on `6677`
- [ ] Custom `--marker-port=N` binds to N and is reachable
- [ ] `dbwatch daemon start --detach` propagates `--marker-port` / `--marker-bind` / `--no-marker` to the child
- [ ] In the TUI, `[` and `]` jump the cursor between markers
- [ ] In the TUI, `M` clears every item before the most recent marker
- [ ] Marker / log items pass through the table filter (never hidden even when their "table" filter would not match)
- [ ] Marker / log items don't appear in the sidebar table list
- [ ] Marker server runs in `dbwatch tail` mode too (no daemon required)
- [ ] A test runner that POSTs to a stopped DBWatch fails silently (the curl connect error doesn't break the suite)

### Phase 7 — DDL tracking

- [ ] Without `--track-ddl`, no event trigger is installed and behavior is unchanged from before (no regression)
- [ ] `dbwatch ddl-tools print-sql` prints the install SQL
- [ ] `dbwatch ddl-tools install` installs the trigger (as superuser); `status` then reports "installed"
- [ ] `dbwatch ddl-tools install` as a non-superuser returns a friendly privilege error
- [ ] `dbwatch ddl-tools uninstall` removes the trigger; `status` then reports "not installed"
- [ ] `dbwatch ddl-tools install` run twice does not error (idempotent)
- [ ] `tail --track-ddl` (auto mode, superuser) auto-installs the trigger and captures DDL
- [ ] `CREATE TABLE` / `ALTER TABLE` / `CREATE INDEX` / `DROP TABLE` each produce a DDL event in the feed
- [ ] `tail --track-ddl` without superuser prints an actionable message and continues with DML only (no crash)
- [ ] DDL events render in the TUI with a distinct magenta `⚡ DDL` style
- [ ] Expanding a DDL event shows command tag, object type, identity, timestamp
- [ ] DDL events appear in attach (IPC) and JSON (pipe) output
- [ ] DDL events pass through the table filter and don't appear in the sidebar table list
- [ ] `--ddl-install-mode=none` skips installation and just LISTENs
- [ ] `--ddl-install-mode=manual` does not install; reports the trigger is missing
- [ ] The detached daemon child inherits `--track-ddl` / `--ddl-install-mode`
- [ ] Integration test: `DBWATCH_TEST_DB_URL=... go test -tags=integration ./internal/ddlwatcher/...` passes

### Phase 8 — Snapshot & Compare

- [ ] `dbwatch snapshot take --db-url=... --label=X` captures and stores a snapshot
- [ ] `dbwatch snapshot list` shows stored snapshots, newest first
- [ ] `dbwatch snapshot show X` prints schema + row counts
- [ ] Re-taking with the same label replaces (does not duplicate) the snapshot
- [ ] `dbwatch snapshot compare A B` diffs two stored snapshots
- [ ] `dbwatch snapshot compare A` (no second arg) diffs against the live database
- [ ] Adding a column shows up as an `added` schema change
- [ ] Removing a column / table is flagged as `breaking` (⚠)
- [ ] `NULL → NOT NULL` and a type change are flagged as `breaking`
- [ ] A column whose NULLs disappeared is flagged as `notable` data drift
- [ ] `--format=json` and `--format=markdown` produce valid output
- [ ] `dbwatch snapshot delete X` removes the snapshot; deleting again reports not-found
- [ ] `--snapshot-tables` / `--snapshot-exclude` restrict the captured tables
- [ ] `--snapshot-tables` and `--snapshot-exclude` together are rejected
- [ ] Snapshots persist across runs (stored in `~/.dbwatch/data.db`)
- [ ] `--data-dir` / `DBWATCH_DATA_DIR` override the storage location
- [ ] A snapshot of a database with ~100k rows completes in well under 5 seconds
- [ ] Integration test: `DBWATCH_TEST_DB_URL=... go test -tags=integration ./internal/snapshot/...` passes

## Verified scenarios log

Record the date and environment when manual tests pass. Format:

```
[YYYY-MM-DD] Phase N — passed on macOS 14 (M1), Postgres 16
[YYYY-MM-DD] Phase N — passed on Ubuntu 22.04, Postgres 15
```

(This section fills in as the project progresses)

## Edge cases to test (Phase 1+)

A few scenarios that commonly cause problems with Postgres logical replication:

### TOAST values

```sql
CREATE TABLE big (id serial, data text);
INSERT INTO big (data) VALUES (repeat('x', 100000));
UPDATE big SET id = id WHERE id = 1; -- update does not touch `data`
```

Expected: the UPDATE event appears with `data` marked `[unchanged]` or skipped.

### REPLICA IDENTITY default vs FULL

```sql
-- Default identity: only the primary key in old_values
ALTER TABLE users REPLICA IDENTITY DEFAULT;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values is just {id: 1}, not the full row
```

Expected: the tool runs but the diff only shows new values, with a warning about REPLICA IDENTITY.

```sql
ALTER TABLE users REPLICA IDENTITY FULL;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values contains the full row
```

Expected: full diff, old → new for every changed column.

### Schema change at runtime

```sql
-- While dbwatch is running:
ALTER TABLE users ADD COLUMN phone text;
INSERT INTO users (name, email, phone) VALUES ('carol', 'c@e.com', '123');
```

Expected: the INSERT event appears with the `phone` column, no dbwatch restart needed.

### Truncate

```sql
TRUNCATE users;
```

Expected (MVP): ignored, or one "TRUNCATE on users" event. Definitive behavior is TBD; document when implemented.

### Transactional rollback

```sql
BEGIN;
INSERT INTO users (name) VALUES ('temp');
ROLLBACK;
```

Expected: no events appear (logical replication legitimately skips rolled-back transactions).

### Long-running transaction

```sql
BEGIN;
INSERT INTO users (name) VALUES ('one');
-- wait 1 minute
INSERT INTO users (name) VALUES ('two');
COMMIT;
```

Expected: both events appear after COMMIT, with the commit timestamp, not the per-statement timestamp.

## Performance smoke test

Not a serious benchmark — just a sanity check:

```sql
-- Generate 10k inserts
INSERT INTO users (name, email)
SELECT 'user' || i, 'user' || i || '@example.com'
FROM generate_series(1, 10000) i;
```

Expected:

- No events lost
- Memory usage stable (ring buffer cap 1000, so memory doesn't grow linearly)
- Latency of the last event < 5 seconds after commit
