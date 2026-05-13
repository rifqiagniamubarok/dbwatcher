# CLAUDE.md

Konteks untuk Claude Code saat bekerja di project ini. Baca file ini di awal setiap sesi sebelum mulai task apapun.

## Project context

DBWatch adalah CLI tool untuk memantau perubahan Postgres database secara realtime di terminal. Target user: developer yang sedang testing/debugging. **Ini bukan production tool.**

Untuk konteks lengkap:
- `ARCHITECTURE.md` — desain teknis, struktur folder, tech stack
- `PLAN.md` — phase plan, task list, expected outcome

Setiap sesi, tanyakan ke user: "Kita di phase berapa, task mana?" sebelum mulai coding.

## Working principles

### 1. One task at a time
Jangan kerjakan multiple task sekaligus, walaupun kelihatannya gampang. Selesaikan satu task sampai expected outcome-nya tercapai, baru lanjut.

### 2. Test-first untuk logic kritis
Untuk komponen berikut, **tulis test dulu sebelum implementation**:
- `internal/listener/decoder.go` — parser binary protocol, banyak edge case
- `internal/listener/schema_cache.go` — cache invalidation
- `internal/store/store.go` — concurrency, ring buffer, pub/sub

Untuk UI (TUI components), test-first nggak wajib — iterasi visual lebih cepat.

### 3. Small commits
Commit setelah setiap task di PLAN.md selesai. Format pesan commit:
```
phase-N: <ringkasan task>
```
Contoh: `phase-1: implement schema cache`, `phase-1: add decoder unit tests`.

### 4. No premature abstraction
Jangan bikin interface kalau cuma ada satu implementor. Tunggu sampai benar-benar butuh polymorphism (paling cepat saat Phase 5 atau saat multi-DB).

### 5. Don't expand scope
Kalau di tengah jalan kepikiran fitur menarik yang nggak ada di PLAN.md, **catat di file `IDEAS.md`**, jangan langsung implement. Diskusikan dengan user dulu.

## Tech constraints

- **Go version:** 1.22 atau lebih baru (untuk `slog`, generics matang)
- **Dependencies:** minimal. Sebelum tambah dependency baru, justifikasi kenapa standard library nggak cukup.
- **Approved dependencies:**
  - `github.com/jackc/pglogrepl` — logical replication
  - `github.com/jackc/pgx/v5` — Postgres driver
  - `github.com/spf13/cobra` — CLI parsing
  - `github.com/charmbracelet/bubbletea` — TUI
  - `github.com/charmbracelet/lipgloss` — styling
  - `github.com/charmbracelet/bubbles` — komponen TUI
  - `github.com/mattn/go-isatty` — deteksi TTY
  - `github.com/stretchr/testify` — assertion helper di test

Selain di atas, tanya dulu sebelum `go get`.

## Code style

### Naming
- Package: lowercase, singular (`listener`, bukan `listeners`)
- Exported type/function: PascalCase
- Error variable: `ErrSomething`
- Test function: `TestXxx`, table-driven test diberi nama `TestXxx_Scenario`

### Error handling
- Bungkus error dengan konteks: `fmt.Errorf("decode insert message: %w", err)`
- User-facing error (yang muncul di CLI) harus actionable. Lihat task 4.1 di PLAN.md untuk contoh format.
- Internal error (di library code) cukup bungkus dengan konteks, jangan format ulang.

### Concurrency
- Selalu jalankan `go test -race ./...` sebelum commit
- Gunakan `context.Context` di setiap fungsi yang bisa block (network call, channel receive yang lama)
- Goroutine harus punya exit path yang jelas. Tidak boleh ada goroutine yang nggak bisa di-stop.
- Channel buffered untuk pub/sub (capacity 100 default), unbuffered untuk synchronization signal.

### Logging
- Pakai `log/slog` (stdlib)
- Level guidance:
  - `Debug` — detail internal yang berguna saat investigasi (LSN, message type, dll)
  - `Info` — milestone normal (connected, slot created, dll). **Hindari spam di hot path.**
  - `Warn` — sesuatu nggak ideal tapi bisa lanjut (TOAST value, replica identity bukan FULL)
  - `Error` — operasi gagal
- Format log untuk user akhir (di CLI) **bukan** pakai slog — pakai output yang readable.

## File organization

Lihat `ARCHITECTURE.md` section "Folder structure" untuk layout lengkap.

Rules tambahan:
- `cmd/` cuma boleh berisi entry point dan wiring. Logic ada di `internal/`.
- `internal/` package tidak boleh import package lain di `internal/` yang levelnya "lebih tinggi". Hirarki:
  - `store` tidak import apa-apa dari internal
  - `listener` boleh import `store`
  - `tui` boleh import `store` (tidak import `listener` langsung)
  - `config` standalone
- Test file di package yang sama (`store_test.go` di package `store`), kecuali integration test yang butuh test database.

## Testing strategy

### Unit test
- Setiap fungsi public di `internal/listener/decoder.go`, `schema_cache.go`, dan seluruh `internal/store/` wajib punya unit test
- Pakai table-driven test untuk function yang banyak case
- Mock minimal — kalau bisa pakai struct asli, pakai struct asli

### Integration test
- Folder `internal/listener/integration_test.go` dengan build tag `//go:build integration`
- Butuh Postgres running, di-skip di unit test biasa
- Jalankan dengan: `go test -tags=integration ./...`

### Manual test
- Setiap akhir phase, jalankan skenario di section "Expected Outcome" PLAN.md
- Catat hasilnya di `TESTING.md` di section "Verified scenarios"

## Anti-patterns to avoid

1. **Goroutine leak.** Setiap `go func()` harus punya cara berhenti via context atau channel close.
2. **Unbounded channel.** Channel `chan Event` di pub/sub harus buffered, dan handler harus drop kalau penuh.
3. **Print di library code.** Library code di `internal/` jangan `fmt.Println`. Pakai slog atau return error.
4. **Hardcoded value.** Capacity, timeout, retry — semua di config atau constant yang jelas.
5. **Big bang refactor.** Kalau struktur perlu berubah, kerjakan bertahap. Bukan rewrite seluruh package.

## When stuck

Kalau task terasa terlalu besar atau ambigu:
1. Pecah jadi sub-task lebih kecil, tulis di chat dulu
2. Tanya user: "Mau aku kerjakan A dulu atau B dulu?"
3. Kalau ada keputusan desain yang nggak jelas di ARCHITECTURE.md, **tanya user, jangan asumsikan**

Kalau test gagal dan susah debug:
1. Tambah log Debug
2. Reproduce dengan minimal example
3. Kalau lebih dari 30 menit stuck, surface ke user dengan ringkasan apa yang udah dicoba
