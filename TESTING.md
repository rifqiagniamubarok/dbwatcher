# TESTING

Panduan testing untuk DBWatch. File ini berkembang seiring project.

## Test layers

DBWatch punya tiga layer test:

1. **Unit test** — package-level test, no external dependency
2. **Integration test** — butuh Postgres running, di-tag `integration`
3. **Manual test** — skenario end-to-end, dijalankan per akhir phase

## Running tests

### Unit test
```bash
make test
# atau
go test ./... -race -count=1
```

`-race` wajib untuk detect data race. `-count=1` mencegah test cache yang kadang menyembunyikan masalah.

### Integration test
```bash
# Start Postgres test instance dulu
./scripts/start-postgres.sh

# Jalankan integration test
go test -tags=integration ./... -race
```

### Coverage
```bash
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

Target coverage minimum:
- `internal/store/` — 80%
- `internal/listener/decoder.go` — 70%
- `internal/listener/schema_cache.go` — 70%
- Komponen lain — best effort

## Test database setup

Script `scripts/start-postgres.sh` melakukan:

1. Stop dan remove container `dbwatch-test-pg` kalau ada
2. Start Postgres 16 di Docker dengan `wal_level=logical`
3. Tunggu sampai siap menerima koneksi
4. Jalankan `scripts/seed.sql` untuk buat table contoh

Tabel yang dibuat:
- `users` (id, name, email, created_at)
- `orders` (id, user_id, total, status, created_at)
- `order_items` (id, order_id, product_id, qty, price)
- `inventory` (id, product_id, stock, updated_at)

Connection string: `postgres://test:test@localhost:5432/test`

## Manual test scenarios

Skenario di bawah dijalankan setelah setiap phase selesai. Centang `[x]` saat sudah verified.

### Phase 0 — Skeleton
- [ ] `make build` sukses
- [ ] `./bin/dbwatch --help` tampilkan help
- [ ] `./bin/dbwatch tail` tampilkan placeholder message
- [ ] `./bin/dbwatch version` tampilkan version
- [ ] `make docker-build` sukses
- [ ] `docker run --rm dbwatch:dev version` tampilkan version

### Phase 1 — Listener core
- [ ] Connect ke Postgres test instance sukses
- [ ] INSERT di `users` muncul sebagai event JSON di stdout
- [ ] UPDATE di `users` muncul dengan field old_values dan new_values
- [ ] DELETE di `users` muncul dengan old_values
- [ ] Beberapa INSERT berturut-turut semua tertangkap, urut, tanpa hilang
- [ ] Disconnect Postgres saat dbwatch jalan → error message ramah, exit dengan code != 0
- [ ] SIGINT (Ctrl+C) shutdown rapi tanpa goroutine leak
- [ ] Output bisa di-pipe ke `jq`: `dbwatch tail ... | jq .table`

### Phase 2 — Store layer
- [ ] Behavior dari user POV sama dengan Phase 1
- [ ] Stats di stderr muncul periodik dengan angka yang masuk akal
- [ ] `go test -race ./internal/store/...` lulus

### Phase 3 — TUI
- [ ] `dbwatch tail` start TUI mode otomatis
- [ ] INSERT muncul dengan warna hijau
- [ ] UPDATE muncul dengan warna kuning, expand tampilkan diff
- [ ] DELETE muncul dengan warna merah
- [ ] `j`/`k` navigasi cursor
- [ ] `enter` toggle expand detail
- [ ] `space` pause/resume, event yang masuk saat paused muncul setelah resume
- [ ] `f` toggle fokus sidebar, navigasi dan toggle table filter
- [ ] `c` clear feed (dengan konfirmasi)
- [ ] `q` quit dengan rapi
- [ ] Resize terminal — layout tetap rapi
- [ ] `dbwatch tail | head -5` jalan di mode JSON (non-TTY detected)
- [ ] `dbwatch tail --output=json` paksa JSON walaupun TTY
- [ ] `dbwatch tail --output=tui` paksa TUI (kalau dijalankan di TTY)

### Phase 4 — Release
- [ ] `go install github.com/<user>/dbwatch/cmd/dbwatch@v0.1.0` sukses dari clean Go env
- [ ] `docker run ghcr.io/<user>/dbwatch:v0.1.0 version` sukses
- [ ] Binary dari GitHub Releases jalan di Linux, macOS (Intel & Apple Silicon)
- [ ] README quick start dari nol sampai event muncul jalan tanpa masalah
- [ ] Semua error message di-test dengan skenario gagal (wrong URL, permission denied, dll)

## Verified scenarios log

Catat tanggal dan environment saat manual test lulus. Format:

```
[YYYY-MM-DD] Phase N — passed on macOS 14 (M1), Postgres 16
[YYYY-MM-DD] Phase N — passed on Ubuntu 22.04, Postgres 15
```

(Section ini akan diisi seiring project berjalan)

## Edge cases to test (Phase 1+)

Beberapa skenario yang sering bikin masalah di Postgres logical replication:

### TOAST values
```sql
CREATE TABLE big (id serial, data text);
INSERT INTO big (data) VALUES (repeat('x', 100000));
UPDATE big SET id = id WHERE id = 1; -- update tidak menyentuh `data`
```
Expected: event UPDATE muncul, kolom `data` ditandai `[unchanged]` atau di-skip.

### REPLICA IDENTITY default vs FULL
```sql
-- Default identity: cuma primary key di old_values
ALTER TABLE users REPLICA IDENTITY DEFAULT;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values cuma {id: 1}, bukan full row
```

Expected: tool jalan, tapi diff cuma tampilkan new values dengan warning soal REPLICA IDENTITY.

```sql
ALTER TABLE users REPLICA IDENTITY FULL;
UPDATE users SET name = 'bob' WHERE id = 1;
-- old_values berisi full row
```

Expected: diff lengkap, old → new untuk setiap kolom yang berubah.

### Schema change saat runtime
```sql
-- Saat dbwatch jalan:
ALTER TABLE users ADD COLUMN phone text;
INSERT INTO users (name, email, phone) VALUES ('carol', 'c@e.com', '123');
```

Expected: event INSERT muncul dengan kolom `phone`, tanpa restart dbwatch.

### Truncate
```sql
TRUNCATE users;
```

Expected (di MVP): di-ignore atau tampilkan satu event "TRUNCATE on users". Definitif: belum diputuskan, dokumentasikan saat implement.

### Transactional rollback
```sql
BEGIN;
INSERT INTO users (name) VALUES ('temp');
ROLLBACK;
```

Expected: tidak ada event muncul (logical replication memang skip transaction yang di-rollback).

### Long-running transaction
```sql
BEGIN;
INSERT INTO users (name) VALUES ('one');
-- tunggu 1 menit
INSERT INTO users (name) VALUES ('two');
COMMIT;
```

Expected: kedua event muncul setelah COMMIT, dengan timestamp commit, bukan saat statement individual.

## Performance smoke test

Bukan benchmark serius, tapi sanity check:

```sql
-- Generate 10k inserts
INSERT INTO users (name, email)
SELECT 'user' || i, 'user' || i || '@example.com'
FROM generate_series(1, 10000) i;
```

Expected:
- Tidak ada event hilang
- Memory usage stabil (ring buffer cap 1000, jadi memori tidak naik linear)
- Latency event terakhir < 5 detik dari commit
