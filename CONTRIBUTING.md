# Contributing to DBWatch

Panduan kerja internal untuk development DBWatch. Baca ini sebelum mulai coding.

---

## Branch strategy

```
main         ← production-ready, protected. Hanya menerima merge dari release/*
development  ← integrasi semua fitur. Base branch untuk semua feature branch
feature/*    ← satu branch per fitur / task
fix/*        ← untuk bug fix
release/*    ← persiapan release (bump version, update CHANGELOG)
```

### Alur kerja

```
feature/your-feature
        ↓  PR
   development
        ↓  PR (setelah semua fitur siap)
  release/v0.x.x
        ↓  PR
        main  ← tag di sini → GitHub Actions auto-release
```

### Aturan branch

- **Jangan commit langsung ke `main` atau `development`**
- Semua perubahan masuk lewat Pull Request
- Branch name: lowercase, kata dipisah dengan `-`
  - ✅ `feature/filter-sidebar`
  - ✅ `fix/tui-cursor-overflow`
  - ❌ `FeatureFilterSidebar`, `my-branch`
- Hapus branch setelah PR di-merge

### Membuat branch baru

```bash
# Selalu branching dari development
git checkout development
git pull origin development
git checkout -b feature/nama-fitur
```

---

## Commit messages

Format wajib: **`<type>(<scope>): <deskripsi singkat>`**

```
<type>(<scope>): <deskripsi singkat>

[opsional: body — jelaskan WHY, bukan WHAT]

[opsional: footer — breaking change, closes issue]
```

### Types

| Type | Kapan dipakai |
| --- | --- |
| `feat` | Fitur baru |
| `fix` | Bug fix |
| `refactor` | Ubah code tanpa tambah fitur atau fix bug |
| `test` | Tambah atau update test |
| `docs` | Perubahan dokumentasi saja |
| `chore` | Maintenance: update deps, config, CI, dll |
| `perf` | Optimasi performa |

### Scopes (opsional tapi dianjurkan)

`listener`, `store`, `tui`, `config`, `cmd`, `ci`, `deps`

### Contoh commit message yang benar

```
feat(tui): add pause/resume with pending event count

Paused state buffers incoming events without displaying them.
Header shows "⏸ paused (12 new)" so user knows events are queued.
```

```
fix(listener): handle replication slot already exists error

Show actionable message with exact psql command to drop the slot.
Closes #12
```

```
chore(ci): fix goreleaser docker build context

Use Dockerfile.goreleaser which copies pre-built binary instead of
building from source — GoReleaser provides the binary in build context.
```

### Aturan commit

- Deskripsi: **lowercase**, **imperative mood** ("add" bukan "added"), **tanpa titik di akhir**
- Maksimal 72 karakter di baris pertama
- Jelaskan *mengapa* di body, bukan *apa* — code sudah menjelaskan *apa*
- Satu commit = satu perubahan logis. Jangan campur fix bug dengan refactor
- Commit kecil lebih baik daripada satu commit besar

---

## Pull Request

### Sebelum buat PR

```bash
# Pastikan semua test lulus
go test -race ./...

# Pastikan tidak ada warning
go vet ./...

# Pastikan code ter-format
gofmt -l .
```

### Judul PR

Ikuti format commit message: `feat(scope): deskripsi singkat`

### Isi PR description

```
## What
[Apa yang berubah — satu paragraf]

## Why
[Kenapa perubahan ini diperlukan]

## How to test
- [ ] Step 1
- [ ] Step 2
```

### Review rules

- Minimal 1 approval sebelum merge
- Semua CI checks harus hijau
- Resolve semua comment sebelum merge
- Gunakan **Squash and merge** untuk feature branch ke development
- Gunakan **Merge commit** untuk release branch ke main

---

## Release process

1. Buat branch `release/vX.Y.Z` dari `development`
2. Update `CHANGELOG.md` — pindahkan dari `[Unreleased]` ke `[X.Y.Z] - YYYY-MM-DD`
3. PR ke `main`
4. Setelah merge, tag di `main`:

```bash
git checkout main
git pull origin main
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions akan otomatis build dan publish release.

---

## Versioning

DBWatch mengikuti [Semantic Versioning](https://semver.org/):

- **PATCH** `v0.1.1` — bug fix, tidak ada perubahan API
- **MINOR** `v0.2.0` — fitur baru, backward compatible
- **MAJOR** `v1.0.0` — breaking change

---

## Development setup

```bash
# Clone dan setup
git clone https://github.com/rifqiagniamubarok/dbwatcher.git
cd dbwatcher
git checkout development

# Build
make build

# Test
make test

# Start test database (butuh Docker)
./scripts/start-postgres.sh

# Run
./bin/dbwatch tail \
  --db-url="postgres://test:test@localhost:5433/test?sslmode=disable&replication=database"
```

---

## Folder structure

```
cmd/dbwatch/        ← entry point dan wiring saja, tidak ada logic
internal/
  listener/         ← WAL streaming dan decoding
  store/            ← ring buffer dan pub/sub
  tui/              ← Bubble Tea TUI
  config/           ← config loader
scripts/            ← dev helper scripts
.github/workflows/  ← CI/CD
```

**Aturan import antar package:**

- `store` → tidak boleh import package internal lain
- `listener` → boleh import `store`
- `tui` → boleh import `store`, tidak boleh import `listener`
- `config` → standalone

---

## Jika stuck

1. Cek `PLAN.md` untuk konteks phase dan task
2. Cek `ARCHITECTURE.md` untuk desain sistem
3. Buka issue di GitHub dengan label `question`
