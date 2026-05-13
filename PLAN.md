# DBWatch — Development Plan

Plan ini disusun phase per phase. Setiap phase punya tujuan jelas, daftar task detail, dan expected outcome yang harus tercapai sebelum lanjut ke phase berikutnya.

Prinsip kerja:
- **Iteratif:** selesaikan phase sekarang dulu, jangan lompat ke depan
- **Demoable:** setiap akhir phase harus bisa di-demo sesuatu
- **Test as you go:** unit test ditulis bareng implementation, bukan belakangan
- **Commit small:** commit kecil per task, bukan satu commit besar per phase

---

## Phase 0 — Project Skeleton

**Tujuan:** Mendapatkan project structure yang siap dikembangkan. Belum ada fungsionalitas, tapi semua tooling sudah siap.

### Tasks

#### 0.1 — Inisialisasi Go module
- Buat folder root project
- Jalankan `go mod init github.com/<username>/dbwatch`
- Set Go version ke 1.22 atau lebih baru di `go.mod`

#### 0.2 — Buat struktur folder
Buat folder berikut (boleh kosong dulu, atau isi dengan `.gitkeep`):
```
cmd/dbwatch/
internal/listener/
internal/store/
internal/tui/
internal/config/
```

#### 0.3 — Setup Cobra CLI
- Tambah dependency `github.com/spf13/cobra`
- Di `cmd/dbwatch/main.go`, buat root command `dbwatch` dengan dua sub-command:
  - `dbwatch tail` — sementara cetak `"dbwatch tail: not implemented yet"`
  - `dbwatch version` — cetak versi (hardcode `"0.0.0-dev"` dulu)
- Root command harus punya `--help` yang masuk akal

#### 0.4 — Buat Makefile
Target minimum:
- `make build` — kompilasi binary ke `./bin/dbwatch`
- `make test` — jalankan `go test ./...`
- `make run` — jalankan `go run ./cmd/dbwatch`
- `make clean` — hapus folder `./bin`
- `make docker-build` — build Docker image (lihat task 0.5)

#### 0.5 — Buat Dockerfile
- Multi-stage build:
  - Stage 1 (`golang:1.22-alpine`): copy source, jalankan `go build`
  - Stage 2 (`alpine:latest` atau `scratch`): copy binary, set `ENTRYPOINT`
- Image final harus < 30 MB

#### 0.6 — Buat README placeholder
Isi minimal:
- Nama project dan one-line description
- Status: "Work in progress"
- Section "Quick Start" yang akan diisi nanti
- Section "Architecture" yang menunjuk ke `ARCHITECTURE.md`

#### 0.7 — Setup `.gitignore`
- `/bin/`
- `*.test`
- `coverage.out`
- `.env`
- File-file IDE (`.idea/`, `.vscode/`)

#### 0.8 — Initial commit
Commit semuanya ke Git. Push ke repo remote.

### Expected Outcome

Setelah Phase 0 selesai, hal-hal berikut harus jalan:

```bash
$ go run ./cmd/dbwatch --help
# tampil help message dengan deskripsi dan list sub-command

$ go run ./cmd/dbwatch tail
dbwatch tail: not implemented yet

$ go run ./cmd/dbwatch version
0.0.0-dev

$ make build && ./bin/dbwatch version
0.0.0-dev

$ make docker-build
# build sukses, image dbwatch:dev tersedia

$ docker run --rm dbwatch:dev version
0.0.0-dev
```

Repo sudah di Git dengan struktur folder lengkap. Belum ada logic apapun, tapi fondasi sudah siap.

---

## Phase 1 — Listener Core (raw events ke stdout)

**Tujuan:** Bisa connect ke Postgres dan print event INSERT/UPDATE/DELETE sebagai JSON ke stdout. Belum ada TUI dan belum ada Store — output langsung dari Listener.

### Tasks

#### 1.1 — Definisikan struct Event
Di `internal/store/event.go`, definisikan tipe-tipe:
- `EventType` (string enum: `INSERT`, `UPDATE`, `DELETE`)
- `Column` (Name, DataType, IsKey)
- `Event` (ID, Timestamp, LSN, Type, Schema, Table, Columns, NewValues, OldValues, TxID)

Method `Event.String()` dan/atau `Event.JSON()` untuk debugging.

#### 1.2 — Implementasi schema cache
Di `internal/listener/schema_cache.go`:
- Struct `SchemaCache` dengan map `OID → TableMetadata`
- `TableMetadata` berisi schema name, table name, list of columns
- Method `Load(ctx, conn)` untuk query awal ke `information_schema`
- Method `Update(rel pglogrepl.RelationMessage)` untuk update dari pgoutput message
- Method `Get(oid uint32) (TableMetadata, bool)`

Unit test: cache bisa di-load dari koneksi mock, bisa di-update dari RelationMessage.

#### 1.3 — Implementasi decoder
Di `internal/listener/decoder.go`:
- Fungsi `DecodeMessage(msg pglogrepl.Message, cache *SchemaCache) (*Event, error)`
- Handle `InsertMessage`, `UpdateMessage`, `DeleteMessage`
- Decode tuple values pakai tipe data dari schema cache
- Handle TOAST values (mark sebagai `nil` atau string khusus `"[unchanged]"`)
- Skip `BeginMessage`, `CommitMessage`, `RelationMessage`, `TypeMessage` (return nil event tanpa error)

Unit test dengan fixture pgoutput message dari pglogrepl examples.

#### 1.4 — Implementasi listener
Di `internal/listener/listener.go`:
- Struct `Listener` dengan field connection config, output channel
- Method `Start(ctx context.Context) error`:
  1. Buka koneksi replication ke Postgres
  2. Load schema cache awal
  3. Create publication kalau belum ada (atau check existence)
  4. Create temporary replication slot
  5. Start replication dari current LSN
  6. Loop baca message:
     - Decode pakai decoder
     - Push ke output channel
     - Ack LSN setiap N detik
  7. Handle context cancellation untuk graceful shutdown

#### 1.5 — Implementasi config loader
Di `internal/config/config.go`:
- Struct `Config` dengan field `DBURL`, `Publication`, `Slot`, `BufferSize`, `LogLevel`
- Fungsi `Load(cmd *cobra.Command) (*Config, error)` yang baca env var, override dari flag
- Validasi: DBURL wajib ada, kalau kosong return error yang ramah

#### 1.6 — Wire ke command `tail`
Di `cmd/dbwatch/main.go`:
- Tambah flag `--db-url`, `--publication`, `--slot`, `--log-level` ke command `tail`
- Implementasi handler `tail`:
  1. Load config
  2. Setup slog handler
  3. Buat Listener
  4. Buat channel untuk Event
  5. Start Listener di goroutine
  6. Loop baca channel: marshal Event jadi JSON, print ke stdout
  7. Handle SIGINT untuk shutdown

#### 1.7 — Setup test database
Di folder `scripts/` atau `dev/`:
- Script `start-postgres.sh` yang jalankan Postgres di Docker dengan `wal_level=logical`
- Script `seed.sql` yang buat beberapa table contoh (`users`, `orders`, `order_items`)
- Dokumentasikan cara pakai di README

#### 1.8 — Manual integration test
Tulis di `TESTING.md`:
- Cara start Postgres test
- Cara jalanin `dbwatch tail`
- Skenario test: INSERT, UPDATE, DELETE di table `users`
- Expected output JSON

### Expected Outcome

Skenario berikut harus jalan tanpa error:

```bash
# Terminal 1: start Postgres
$ ./scripts/start-postgres.sh

# Terminal 2: jalanin dbwatch
$ go run ./cmd/dbwatch tail --db-url=postgres://test:test@localhost:5432/test
# (hanging, waiting for events)

# Terminal 3: trigger perubahan
$ psql postgres://test:test@localhost:5432/test
> INSERT INTO users (name, email) VALUES ('alice', 'alice@example.com');
> UPDATE users SET email='alice2@example.com' WHERE name='alice';
> DELETE FROM users WHERE name='alice';
```

Di terminal 2 muncul output JSON kira-kira seperti ini, satu event per baris:

```json
{"id":1,"timestamp":"2026-05-13T14:32:01Z","type":"INSERT","table":"users","new_values":{"id":1,"name":"alice","email":"alice@example.com"}}
{"id":2,"timestamp":"2026-05-13T14:32:05Z","type":"UPDATE","table":"users","old_values":{"email":"alice@example.com"},"new_values":{"email":"alice2@example.com"}}
{"id":3,"timestamp":"2026-05-13T14:32:08Z","type":"DELETE","table":"users","old_values":{"id":1}}
```

Pipe-friendly:
```bash
$ go run ./cmd/dbwatch tail --db-url=... | jq 'select(.table == "users")'
# berhasil, jq parse JSON tiap baris
```

Test coverage untuk `decoder.go` dan `schema_cache.go` minimal 70%.

---

## Phase 2 — Store Layer

**Tujuan:** Event yang sudah di-decode tidak langsung print, tapi masuk ke Store dulu. Store bisa di-subscribe oleh multiple consumer. Saat ini consumer-nya cuma satu (printer JSON), tapi arsitekturnya siap untuk TUI.

### Tasks

#### 2.1 — Implementasi Store
Di `internal/store/store.go`:
- Struct `Store` dengan field:
  - `events []Event` — ring buffer
  - `capacity int`
  - `cursor int` — posisi tulis berikutnya
  - `count uint64` — total event diterima sejak start
  - `subs []chan Event` — subscriber channels
  - `mu sync.RWMutex`
- Constructor `New(capacity int) *Store`
- Method `Push(e Event)` — append ke ring, broadcast ke semua subs (non-blocking)
- Method `Subscribe() <-chan Event` — buat channel buffered (cap 100), tambah ke list, return
- Method `Unsubscribe(ch <-chan Event)` — cari di list, close, remove
- Method `Snapshot() []Event` — return copy semua event yang masih ada di ring, urut dari paling lama
- Method `Stats() Stats` — return total count, current size

#### 2.2 — Filter di Store level
Tambah method `Subscribe`-mu dengan opsi filter:
- `SubscribeWithFilter(filter Filter) <-chan Event`
- `Filter` adalah interface dengan method `Match(e Event) bool`
- Implementasi `TableFilter` yang match berdasarkan list nama table
- Implementasi `AllowAllFilter` sebagai default

#### 2.3 — Unit test Store
Test cases yang harus ada di `internal/store/store_test.go`:
- Push ke Store kosong, Snapshot return event tersebut
- Push melebihi capacity, event terlama hilang
- Single subscriber dapat event setelah subscribe
- Multi subscriber semua dapat event yang sama
- Subscriber dengan filter cuma dapat event yang match
- Unsubscribe menghentikan delivery ke channel
- Stats akurat
- Concurrent Push dari multiple goroutine tidak race (jalankan dengan `-race`)

#### 2.4 — Refactor main.go untuk pakai Store
Di handler `tail`:
1. Buat instance Store
2. Start Listener, output channel-nya di-forward ke `store.Push()` di goroutine kecil
3. `store.Subscribe()`, lalu loop print JSON dari channel
4. Pastikan SIGINT trigger Unsubscribe dan stop Listener dengan rapi

#### 2.5 — Tambah counter di output (opsional)
Setiap N detik atau setiap N event, print stats ke stderr (bukan stdout, biar tidak ganggu JSON):
```
[stats] received=1234 buffered=1000 subscribers=1
```

### Expected Outcome

Behavior dari user point of view sama dengan Phase 1. Tapi secara internal:

```bash
$ go run ./cmd/dbwatch tail --db-url=... 2>/dev/null
{"id":1,...}
{"id":2,...}

$ go run ./cmd/dbwatch tail --db-url=...
# stderr ada stats periodik:
# [stats] received=42 buffered=42 subscribers=1
```

Unit test Store harus lulus dengan `go test -race ./internal/store/...`.

Code review checklist:
- [ ] Tidak ada data race
- [ ] Tidak ada goroutine leak (subscriber selalu di-cleanup)
- [ ] Push non-blocking (kalau subscriber lambat, event di-drop untuk subscriber itu, bukan stuck)

---

## Phase 3 — Bubble Tea TUI

**Tujuan:** Tampilan terminal yang interaktif. Ini bagian paling besar dan paling banyak iterasi.

### Tasks

#### 3.1 — Setup Bubble Tea skeleton
Di `internal/tui/app.go`:
- Tambah dependency `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/charmbracelet/bubbles`
- Struct `Model` dengan field minimal: events, width, height
- Implementasi `Init() tea.Cmd`, `Update(msg tea.Msg) (tea.Model, tea.Cmd)`, `View() string`
- Handle `tea.WindowSizeMsg` untuk responsive layout
- Handle `tea.KeyMsg` minimal: `q` dan `Ctrl+C` untuk quit

#### 3.2 — Bridge dari Store ke TUI
Bubble Tea pakai custom message untuk update. Di `internal/tui/app.go`:
- Definisikan `eventMsg Event` sebagai tea.Msg
- Fungsi yang baca dari `<-chan Event` dan kirim ke Bubble Tea pakai `program.Send(eventMsg(e))`
- Di `Update`, handle `eventMsg` dengan append ke `m.events`

#### 3.3 — Komponen feed
Di `internal/tui/feed.go`:
- Render list event dari `m.events`
- Format per baris: `HH:MM:SS.mmm  TYPE  table  ringkasan`
- Highlight baris di posisi `m.cursor`
- Auto-scroll ke bawah kalau belum di-pause
- Pakai `viewport` dari bubbles untuk scrolling

#### 3.4 — Komponen filter sidebar
Di `internal/tui/filter.go`:
- Maintain set table yang pernah muncul (extract dari event yang masuk)
- Render checkbox per table
- Saat focus di sidebar, `j`/`k` navigasi, `space` toggle
- Saat toggle, update filter di Store (atau filter di sisi Model — pilih yang lebih simpel)

#### 3.5 — Komponen detail/diff
Di `internal/tui/detail.go`:
- Saat user tekan `enter` di feed, expand detail event di bawah baris itu (atau di panel kanan)
- Untuk INSERT: tampilkan semua kolom dan nilai
- Untuk DELETE: tampilkan semua kolom dan nilai (dari OldValues)
- Untuk UPDATE: tampilkan diff format:
  ```
  inventory (id=99)
    stock:        50  →  47        [changed]
    updated_at:   ...  → ...       [changed]
    name:         Widget           [unchanged, dim]
  ```
- Tekan `enter` lagi untuk collapse

#### 3.6 — Styling dengan lipgloss
Di `internal/tui/styles.go`:
- Definisikan style untuk: header, footer, INSERT row, UPDATE row, DELETE row, table name, timestamp, diff old, diff new, cursor highlight
- Pakai adaptive color (otomatis sesuaikan dark/light terminal)

#### 3.7 — Keybindings lengkap
Implementasi keybinding di `Update`:
- `j` / `↓`, `k` / `↑` — navigasi feed
- `g` — pindah ke event paling lama
- `G` — pindah ke event terbaru
- `enter` — toggle expand
- `space` — pause / resume
- `f` — toggle focus ke sidebar filter
- `c` — clear feed (konfirmasi dengan tekan `c` lagi)
- `?` — toggle help overlay
- `q` / `Ctrl+C` — quit

#### 3.8 — Pause behavior
Saat paused:
- Event tetap masuk ke Store di background
- TUI tidak otomatis update list, tapi indikator di header berubah (`⏸ paused (12 new)`)
- Saat resume, event baru flush ke list

#### 3.9 — Header dan footer
- Header: nama tool, status koneksi (connected/disconnected), DB target, total event, status pause
- Footer: help line keybinding utama (selalu visible, opsional di-toggle)

#### 3.10 — Non-TTY fallback
Di `cmd/dbwatch/main.go` handler `tail`:
- Cek `isatty.IsTerminal(os.Stdout.Fd())`
- Kalau true: jalankan Bubble Tea program
- Kalau false (di-pipe atau redirect ke file): jalankan loop print JSON seperti Phase 2
- Pakai library `github.com/mattn/go-isatty`

#### 3.11 — Flag untuk paksa mode output
Tambah flag `--output={tui,json}` untuk override deteksi otomatis. Default `auto`.

### Expected Outcome

Demo skenario lengkap:

```bash
$ ./dbwatch tail --db-url=postgres://test:test@localhost:5432/test
```

Terminal langsung berubah jadi tampilan TUI seperti mockup di ARCHITECTURE.md. Lalu di terminal lain:

```sql
INSERT INTO orders (user_id, total) VALUES (7, 150);
INSERT INTO order_items (order_id, product_id, qty) VALUES (last_value(), 99, 2);
UPDATE inventory SET stock = stock - 2 WHERE product_id = 99;
```

Di TUI muncul 3 event berurutan kurang dari 1 detik kemudian:
- Baris 1 hijau: `INSERT orders id=...`
- Baris 2 hijau: `INSERT order_items ...`
- Baris 3 kuning: `UPDATE inventory stock 50 → 48`

User bisa:
- Tekan `k` untuk pindah ke baris 3
- Tekan `enter`, lihat diff `stock: 50 → 48`
- Tekan `space`, lihat header berubah jadi paused
- Tekan `f`, fokus pindah ke sidebar, uncheck `inventory`, fokus balik
- Event UPDATE inventory selanjutnya tidak muncul lagi

Pipe-friendly mode tetap jalan:
```bash
$ ./dbwatch tail --db-url=... | head -5
# 5 baris JSON, lalu tool exit dengan rapi
```

---

## Phase 4 — Polish & Release

**Tujuan:** Tool siap untuk dibagikan ke developer lain. Setup distribusi otomatis.

### Tasks

#### 4.1 — Error handling yang ramah
Audit semua error path dan ganti dengan pesan yang clear untuk user akhir:
- Connection refused → "Cannot connect to Postgres at <host>. Is it running and accessible?"
- `wal_level != logical` → "Postgres is not configured for logical replication. Set wal_level=logical in postgresql.conf and restart."
- Permission denied → "User <user> does not have REPLICATION privilege. Run: ALTER USER <user> REPLICATION;"
- Publication tidak ada dan tidak bisa dibuat → guide untuk bikin manual

Setiap error punya:
- Pesan jelas (apa yang salah)
- Hint actionable (apa yang harus dilakukan)
- Optional: link ke section README

#### 4.2 — README lengkap
Sections:
- **What is dbwatch?** — one-paragraph pitch
- **Demo** — embed asciinema cast atau GIF
- **Quick Start** — 3 command yang langsung jalan
- **Installation** — go install, brew, Docker, manual download
- **Postgres Setup** — step by step set wal_level, create publication, replica identity
- **Usage** — flag dan env var
- **Keybindings** — table lengkap
- **Troubleshooting** — error umum + solusi
- **Architecture** — link ke ARCHITECTURE.md
- **Contributing** — link ke CONTRIBUTING.md (kalau ada)
- **License**

#### 4.3 — Buat demo asciinema atau GIF
- Record sesi 30-60 detik: start dbwatch, jalanin beberapa SQL, tunjukin diff dan filter
- Upload ke asciinema.org atau convert ke GIF dengan `agg`
- Embed di README

#### 4.4 — Setup GoReleaser
Buat `.goreleaser.yml` dengan:
- Build untuk: linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64
- Archive: tar.gz untuk Unix, zip untuk Windows
- Checksums file
- Changelog otomatis dari commit messages
- Docker image multi-arch ke ghcr.io
- Homebrew tap (opsional di v0.1)

#### 4.5 — Setup GitHub Actions
Workflow `release.yml`:
- Trigger: push tag `v*.*.*`
- Run GoReleaser

Workflow `ci.yml`:
- Trigger: push ke main, pull request
- Run `go test ./... -race`
- Run `go vet`
- Run linter (`golangci-lint`)
- Service container Postgres untuk integration test

#### 4.6 — Versioning
- Di `cmd/dbwatch/main.go`, baca versi dari `runtime/debug.ReadBuildInfo()` atau ldflags
- Set ldflags di build: `-X main.version={tag} -X main.commit={sha}`
- Command `dbwatch version` tampilkan: version, commit, build date, go version

#### 4.7 — Tag dan release pertama
- Pastikan README dan CHANGELOG ready
- Tag `v0.1.0`
- Push tag, biarin GoReleaser jalan
- Verify: binary di GitHub Releases, Docker image di ghcr.io
- Test instalasi dari user perspective:
  ```bash
  go install github.com/<user>/dbwatch/cmd/dbwatch@v0.1.0
  docker pull ghcr.io/<user>/dbwatch:v0.1.0
  ```

#### 4.8 — Announce
Sumber audiens yang relevan:
- Show HN
- r/PostgreSQL, r/golang
- Hacker News, Lobsters
- Twitter/X dev community
- Postgres weekly newsletter

Siapkan: 1-paragraph pitch, link demo, link repo.

### Expected Outcome

Versi v0.1.0 sudah ter-release dan dapat diinstall dengan:

```bash
# Lewat go install
go install github.com/<user>/dbwatch/cmd/dbwatch@latest

# Lewat Homebrew (kalau setup)
brew install <user>/tap/dbwatch

# Lewat Docker
docker run --rm -it ghcr.io/<user>/dbwatch:latest tail --db-url=...

# Lewat download manual dari GitHub Releases
curl -L https://github.com/<user>/dbwatch/releases/download/v0.1.0/dbwatch_linux_amd64.tar.gz | tar xz
./dbwatch version
```

README punya demo visual yang convincing. Error handling sudah ramah. Ada minimal 1 issue tracker activity dari user pertama (atau confirmation bahwa tool jalan di environment mereka).

---

## Phase 5 — Web UI (after MVP feedback)

**Tujuan:** Tambah dashboard web sebagai mode tambahan. CLI tetap default.

**Catatan:** Phase ini sebaiknya dimulai **setelah ada feedback dari minimal 5-10 user real** yang pakai CLI. Tanpa validasi, beresiko bikin web UI yang tidak dipakai.

### Tasks (high level)

#### 5.1 — Server package
- Tambah `internal/server/` dengan HTTP handler dan WebSocket
- Subscribe ke Store yang sama
- Endpoint: `GET /` (serve UI), `GET /api/events` (snapshot), `WS /ws` (realtime)

#### 5.2 — Frontend minimal
- HTML + Alpine.js atau Preact, bundled
- Layout mirip TUI: header, sidebar filter, feed, detail panel
- Embed via `go:embed` ke binary

#### 5.3 — Subcommand `serve`
- `dbwatch serve --port=7878 --db-url=...`
- TUI dan Web jadi mode terpisah, tidak bisa keduanya bersamaan di satu proses

#### 5.4 — Dockerize untuk mode serve
- Expose port 7878
- Update README dengan contoh `docker run -p 7878:7878 ...`

### Expected Outcome

```bash
$ dbwatch serve --db-url=...
DBWatch serving at http://localhost:7878
```

Buka browser → dashboard hidup → trigger SQL → event muncul realtime di browser.

CLI dan Web share inti yang sama. Tidak ada duplikasi logic Listener/Store.

---

## Recommended workflow with Claude Code

Saat menjalankan plan ini dengan Claude Code, beberapa praktik yang direkomendasikan:

1. **Kasih konteks via file ini.** Mulai sesi dengan: "Baca `ARCHITECTURE.md` dan `PLAN.md`. Sekarang kita di Phase X, task Y. Bantu aku implement."

2. **Satu task per sesi.** Jangan minta Claude bikin seluruh Phase 1 sekaligus. Pecah per task (1.1, 1.2, dst).

3. **Minta test dulu untuk logic kritis.** Untuk decoder dan store, mulai dengan: "Tulis test untuk fungsi X dengan kasus-kasus berikut...". Lalu minta implementation.

4. **Review sebelum lanjut.** Di akhir setiap phase, minta Claude review struktur dan code style. Lebih baik refactor dini daripada di Phase 4.

5. **Manual test setelah setiap task selesai.** Jangan trust kalau "code-nya jalan". Selalu jalanin sendiri, terutama untuk yang berhubungan dengan Postgres replication (banyak edge case).

6. **Commit per task selesai.** Commit message format: `phase-1: implement schema cache`, dll. Memudahkan kalau perlu rollback.

---

## Definition of "MVP Complete"

DBWatch v0.1.0 (akhir Phase 4) dinyatakan complete kalau:

- [ ] Bisa connect ke Postgres dan stream perubahan realtime
- [ ] TUI menampilkan INSERT/UPDATE/DELETE dengan color coding
- [ ] UPDATE event punya diff view (old → new)
- [ ] Filter by table berfungsi
- [ ] Pause/resume berfungsi
- [ ] Pipe-friendly mode (JSON output ke stdout saat non-TTY)
- [ ] Distribusi via `go install`, Docker, dan GitHub Releases
- [ ] README lengkap dengan demo dan troubleshooting
- [ ] Minimal 1 user di luar diri sendiri yang berhasil install dan pakai

Kalau semua di atas check, MVP shipped. Lanjut ke Phase 5 berdasarkan feedback.
