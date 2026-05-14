# Testing

Testing guide for DBWatch.

## Test layers

DBWatch has three test layers:

1. **Unit tests** — package-level tests, no external dependencies
2. **Integration tests** — require a running Postgres instance, tagged with `integration`
3. **Manual tests** — end-to-end scenarios, run at the end of each phase

## Running tests

### Unit tests

```bash
make test
# or
go test ./... -race -count=1
```

`-race` is required to detect data races. `-count=1` prevents test caching which can sometimes hide issues.

### Integration tests

```bash
# Start the test Postgres instance first
./scripts/start-postgres.sh

# Run integration tests
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
- Other packages — best effort

## Test database setup

`scripts/start-postgres.sh` does the following:

1. Stop and remove the `dbwatch-test-pg` container if it exists
2. Start Postgres 16 in Docker with `wal_level=logical` on port 5433
3. Wait until the instance is ready to accept connections
4. Run `scripts/seed.sql` to create example tables

Tables created:

- `users` (id, name, email, created_at)
- `orders` (id, user_id, total, status, created_at)
- `order_items` (id, order_id, product_id, qty, price)
- `inventory` (id, product_id, stock, updated_at)

All tables have `REPLICA IDENTITY FULL` set for complete old/new value diffs.

Connection string: `postgres://test:test@localhost:5433/test?sslmode=disable`

## Manual test scenarios

Run these scenarios at the end of each phase. Check `[x]` when verified.

### Phase 0 — Skeleton

- [ ] `make build` succeeds
- [ ] `./bin/dbwatch --help` shows help message
- [ ] `./bin/dbwatch tail` prints placeholder message
- [ ] `./bin/dbwatch version` prints version
- [ ] `make docker-build` succeeds
- [ ] `docker run --rm dbwatch:dev version` prints version

### Phase 1 — Listener core

- [ ] Connects to test Postgres successfully
- [ ] INSERT on `users` appears as a JSON event on stdout
- [ ] UPDATE on `users` appears with `old_values` and `new_values`
- [ ] DELETE on `users` appears with `old_values`
- [ ] Multiple consecutive INSERTs are all captured, in order, without loss
- [ ] Disconnecting Postgres while dbwatch is running → friendly error message, exits with code != 0
- [ ] SIGINT (Ctrl+C) shuts down cleanly without goroutine leaks
- [ ] Output can be piped to `jq`: `dbwatch tail ... | jq .table`

### Phase 2 — Store layer

- [ ] User-facing behavior is identical to Phase 1
- [ ] Periodic stats appear on stderr with reasonable numbers
- [ ] `go test -race ./internal/store/...` passes

### Phase 3 — TUI

- [ ] `dbwatch tail` starts TUI mode automatically
- [ ] INSERT events appear in green
- [ ] UPDATE events appear in yellow; expanding shows diff
- [ ] DELETE events appear in red
- [ ] `j`/`k` moves the cursor
- [ ] `enter` toggles the expanded detail view
- [ ] `space` pauses/resumes; events received while paused appear on resume
- [ ] `f` toggles filter sidebar focus; `j`/`k` navigate, `space` toggles table
- [ ] `c` clears the feed (with confirmation)
- [ ] `q` quits cleanly
- [ ] Resizing the terminal — layout stays correct
- [ ] `dbwatch tail | head -5` works in JSON mode (non-TTY detected)
- [ ] `dbwatch tail --output=json` forces JSON mode even in a TTY
- [ ] `dbwatch tail --output=tui` forces TUI mode

### Phase 4 — Release

- [ ] `go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@v0.1.0` succeeds from a clean Go environment
- [ ] `docker run ghcr.io/rifqiagniamubarok/dbwatcher:v0.1.0 version` succeeds
- [ ] Binaries from GitHub Releases work on Linux, macOS (Intel and Apple Silicon)
- [ ] README quick start works end-to-end without any issues
- [ ] All error messages tested with failure scenarios (wrong URL, permission denied, etc.)

## Verified scenarios log

Record the date and environment when a manual test passes:

```
[YYYY-MM-DD] Phase N — passed on macOS 14 (M1), Postgres 16
[YYYY-MM-DD] Phase N — passed on Ubuntu 22.04, Postgres 15
```

## Edge cases to test (Phase 1+)

Common problem scenarios with Postgres logical replication:

### TOAST values

```sql
CREATE TABLE big (id serial, data text);
INSERT INTO big (data) VALUES (repeat('x', 100000));
UPDATE big SET id = id WHERE id = 1; -- update does not touch `data`
```

Expected: UPDATE event appears, `data` column is marked `[unchanged]`.

### REPLICA IDENTITY default vs FULL

```sql
-- Default identity: only primary key in old_values
ALTER TABLE users REPLICA IDENTITY DEFAULT;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values only has {id: 1}, not the full row
```

Expected: tool works, but diff only shows new values with a warning about REPLICA IDENTITY.

```sql
ALTER TABLE users REPLICA IDENTITY FULL;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values has the full row
```

Expected: full diff, old → new for each changed column.

### Schema change at runtime

```sql
-- While dbwatch is running:
ALTER TABLE users ADD COLUMN phone text;
INSERT INTO users (name, email, phone) VALUES ('carol', 'c@e.com', '123');
```

Expected: INSERT event appears with the `phone` column, no restart needed.

### TRUNCATE

```sql
TRUNCATE users;
```

Expected (MVP): ignored or shown as a single `TRUNCATE on users` event. Behavior to be finalized when implemented.

### Transactional rollback

```sql
BEGIN;
INSERT INTO users (name) VALUES ('temp');
ROLLBACK;
```

Expected: no event appears (logical replication skips rolled-back transactions by design).

### Long-running transaction

```sql
BEGIN;
INSERT INTO users (name) VALUES ('one');
-- wait 1 minute
INSERT INTO users (name) VALUES ('two');
COMMIT;
```

Expected: both events appear after COMMIT, timestamped at commit time, not at the individual statement time.

## Performance smoke test

Not a serious benchmark — just a sanity check:

```sql
-- Generate 10k inserts
INSERT INTO users (name, email)
SELECT 'user' || i, 'user' || i || '@example.com'
FROM generate_series(1, 10000) i;
```

Expected:

- No events are lost
- Memory usage is stable (ring buffer capped at 1000, so memory does not grow linearly)
- Latency for the last event is < 5 seconds from commit
