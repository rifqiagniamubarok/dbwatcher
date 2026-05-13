# DBWatch — Architecture

## Overview

DBWatch adalah CLI tool untuk memantau perubahan database Postgres secara realtime. Tool ini ditujukan untuk **development environment**, bukan production observability. Filosofi utamanya: `tail -f` untuk Postgres.

Saat developer melakukan testing atau debugging, DBWatch menampilkan setiap INSERT, UPDATE, dan DELETE yang terjadi di database secara live di terminal, dengan diff view untuk UPDATE. Ini memberi developer (dan AI agent seperti Claude Code) visibilitas langsung terhadap side-effect dari kode yang sedang dikerjakan.

## Goals & non-goals

### Goals
- Realtime: latency event dari Postgres ke terminal < 1 detik
- Zero friction: developer cukup `docker run` atau `dbwatch tail`, langsung jalan
- Minimal dependency: satu binary, embedded UI, tidak butuh service eksternal
- Reusable core: Listener dan Store didesain agar bisa dipakai ulang untuk Web UI di phase berikutnya

### Non-goals
- Production audit trail (sudah ada Debezium, pgaudit, dll)
- Persistent storage / long-term retention
- Authentication, RBAC, multi-tenant
- High availability, clustering
- Multi-database support di MVP (Postgres only)

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

Semua komponen di dalam DBWatch binary hidup di satu proses, berkomunikasi via Go channels (in-memory). Tidak ada network call antar komponen, tidak ada serialisasi internal.

## Component breakdown

### Listener

**Tanggung jawab:** Connect ke Postgres sebagai logical replication consumer, baca stream WAL, decode binary pgoutput menjadi struct `Event` yang bermakna.

**Cara kerja:**
1. Saat startup, query `information_schema.columns` untuk semua table di public schema. Cache hasilnya di map per table OID.
2. Buat replication slot temporary (auto-cleanup saat disconnect) atau pakai slot yang sudah ada.
3. Mulai streaming dari LSN terakhir.
4. Untuk setiap message pgoutput:
   - `RelationMessage`: update schema cache jika belum ada
   - `InsertMessage`, `UpdateMessage`, `DeleteMessage`: decode jadi `Event`, push ke Store
   - `BeginMessage`, `CommitMessage`: track transaction context (untuk grouping di v0.2)
5. Acknowledge LSN secara berkala agar Postgres tidak menumpuk WAL.

**Dependency:** `github.com/jackc/pglogrepl`, `github.com/jackc/pgx/v5`.

**Edge case yang harus di-handle:**
- TOAST values (kolom besar yang tidak ikut di-ship saat UPDATE jika tidak berubah) → tampilkan sebagai "[unchanged]"
- Replica identity bukan FULL → old values tidak tersedia di UPDATE, tampilkan informasi seadanya dengan warning
- Schema change saat runtime (ALTER TABLE) → refresh cache saat dapat RelationMessage baru
- Koneksi putus → retry dengan exponential backoff

### Store

**Tanggung jawab:** Menyimpan event terbaru di memori dan mendistribusikan ke semua subscriber yang aktif.

**Cara kerja:**
1. Maintain slice circular dengan kapasitas tetap (default 1000). Event terlama otomatis ter-overwrite.
2. Maintain list of subscriber channels. Saat ada event baru:
   - Append ke ring buffer
   - Broadcast ke semua subscriber channel (non-blocking, drop jika channel penuh)
3. Expose method:
   - `Push(event Event)` — dipanggil Listener
   - `Subscribe() <-chan Event` — dipanggil Renderer
   - `Unsubscribe(ch <-chan Event)` — saat subscriber selesai
   - `Snapshot() []Event` — ambil semua event yang masih ada (untuk initial render)

**Concurrency:** Pakai `sync.Mutex` untuk akses ke slice dan subscriber list. Bukan bottleneck karena volume event di dev environment rendah.

**Catatan desain:** Store sengaja tidak tahu apa-apa tentang format event maupun siapa yang subscribe. Ini yang bikin reusable saat Web UI ditambahkan — tinggal subscribe channel baru.

### Renderer (TUI)

**Tanggung jawab:** Render UI di terminal, handle interaksi user, subscribe ke Store untuk update realtime.

**Arsitektur:** Mengikuti pola Elm/Bubble Tea — `Model`, `Update`, `View`.

**State (Model):**
```go
type Model struct {
    events       []Event              // snapshot lokal dari Store
    cursor       int                  // event yang sedang di-highlight
    expanded     bool                 // detail view terbuka?
    paused       bool                 // freeze incoming events?
    filter       map[string]bool      // table aktif (true = ditampilkan)
    tables       []string             // semua table yang pernah muncul
    focus        Focus                // FOCUS_FEED atau FOCUS_FILTER
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
- `j` / `↓` — pindah ke event di bawah
- `k` / `↑` — pindah ke event di atas
- `enter` — toggle expand detail
- `space` — pause / resume feed
- `f` — toggle focus ke sidebar filter
- `c` — clear feed
- `q` / `Ctrl+C` — quit

**Color coding (lipgloss):**
- INSERT: hijau
- UPDATE: kuning
- DELETE: merah
- Table name: cyan, bold
- Timestamp: gray, dim
- Diff old value: merah/striked
- Diff new value: hijau

**Non-TTY mode:** Jika `stdout` bukan terminal (di-pipe ke `jq`, `grep`, atau file), Renderer tidak start TUI. Sebagai gantinya, print setiap event sebagai JSON line ke stdout. Ini membuat tool pipe-friendly:

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
    Timestamp time.Time         // saat event diterima
    LSN       string            // Postgres LSN, untuk debugging
    Type      EventType
    Schema    string            // misal "public"
    Table     string            // misal "orders"
    Columns   []Column          // metadata kolom
    NewValues map[string]any    // untuk INSERT dan UPDATE
    OldValues map[string]any    // untuk UPDATE dan DELETE (jika REPLICA IDENTITY FULL)
    TxID      uint32            // transaction ID, untuk grouping nanti
}

type Column struct {
    Name     string
    DataType string
    IsKey    bool
}
```

### Diff (computed at render time)

Untuk event tipe UPDATE, Renderer membandingkan `OldValues` dan `NewValues` di runtime — tidak disimpan di Event. Ini menghemat memori dan fleksibel jika strategi diff berubah.

## Configuration

Semua config via environment variable dan command-line flag. Flag override env var. Tidak ada config file untuk MVP.

| Setting | Env var | Flag | Default | Required |
|---|---|---|---|---|
| Database URL | `DBWATCH_DB_URL` | `--db-url` | — | yes |
| Publication name | `DBWATCH_PUBLICATION` | `--publication` | `dbwatch_pub` | no |
| Replication slot | `DBWATCH_SLOT` | `--slot` | `dbwatch_slot` | no |
| Buffer size | `DBWATCH_BUFFER` | `--buffer` | `1000` | no |
| Log level | `DBWATCH_LOG_LEVEL` | `--log-level` | `warn` | no |
| Output format | — | `--output` | `tui` (atau `json` jika non-TTY) | no |

## Postgres setup requirements

Tool ini butuh konfigurasi Postgres berikut. Akan didokumentasikan di README dengan jelas:

1. `wal_level = logical` di `postgresql.conf` (butuh restart)
2. `max_replication_slots >= 1` (default biasanya cukup)
3. User dengan privilege `REPLICATION` dan `SELECT` di table yang ingin di-watch
4. Publication mencakup table yang ingin di-watch:
   ```sql
   CREATE PUBLICATION dbwatch_pub FOR ALL TABLES;
   ```
5. (Opsional, untuk diff lengkap di UPDATE/DELETE) `ALTER TABLE foo REPLICA IDENTITY FULL;` per table

Untuk dev environment dengan Postgres di Docker, contoh command:
```bash
docker run -d \
  -e POSTGRES_PASSWORD=test \
  -p 5432:5432 \
  postgres:16 \
  -c wal_level=logical
```

## Distribution

Tiga channel distribusi:

1. **Single binary** via `go install github.com/<user>/dbwatch/cmd/dbwatch@latest`
2. **Docker image** via `docker run -it ghcr.io/<user>/dbwatch tail --db-url=...`
3. **GitHub Releases** dengan pre-built binary untuk Linux, macOS, Windows × amd64, arm64

Semua di-build otomatis oleh GoReleaser yang ter-trigger dari git tag.

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
│   │   └── event.go             # struct Event, Column
│   ├── tui/
│   │   ├── app.go               # Bubble Tea Model + Update
│   │   ├── feed.go              # komponen list event
│   │   ├── filter.go            # komponen sidebar filter
│   │   ├── detail.go            # komponen detail/diff
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

## Future extension points

Arsitektur ini dirancang agar fitur di bawah bisa ditambahkan **tanpa mengubah Listener atau Store**:

- **Web UI:** Tambah package `internal/server/` dengan HTTP + WebSocket. Subscribe ke Store yang sama.
- **Session correlation:** Tambah field `ApplicationName` dan `Pid` di Event. Listener decode dari Postgres metadata.
- **MCP server:** Tambah package `internal/mcp/` yang expose Store sebagai MCP resource untuk AI agent.
- **Persistent storage:** Tambah subscriber baru di Store yang nulis ke SQLite. Tidak menggantikan ring buffer.
- **Multi-database:** Refactor Listener jadi interface, implement `postgres_listener.go` dan `mysql_listener.go` terpisah.

Semua ekstensi di atas adalah **additive**. Inti aplikasi tidak perlu diubah.
