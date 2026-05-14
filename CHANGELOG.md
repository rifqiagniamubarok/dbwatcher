# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project follows [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

## [0.1.0] - 2026-05-14

### Added

- **Listener** — connects to Postgres via logical replication, streams INSERT/UPDATE/DELETE events in realtime
- **Decoder** — decodes pgoutput WAL messages into typed `Event` structs, handles TOAST (unchanged) values and NULL
- **Schema cache** — caches table column metadata per relation OID, auto-updates on `RelationMessage`
- **Store** — in-memory ring buffer (default 1000 events) with pub/sub fan-out and table-based filtering
- **Bubble Tea TUI** — interactive terminal UI with:
  - Live event feed with color coding (INSERT=green, UPDATE=yellow, DELETE=red)
  - Diff view on `enter` — shows old → new for UPDATE events
  - Filter sidebar (`f`) to show/hide specific tables
  - Pause/resume (`space`) — buffers events while paused
  - Full keybinding set: `j/k` navigate, `g/G` jump, `c` clear, `?` help, `q` quit
- **JSON mode** — auto-detects non-TTY and outputs one JSON event per line (pipe-friendly)
- **`--output` flag** — override output mode: `auto`, `tui`, `json`
- **Config** — all settings via flags or environment variables (`DBWATCH_DB_URL`, etc.)
- **Friendly error messages** — actionable hints for common failures (TLS, wal_level, REPLICATION privilege, slot conflict)
- **Versioning** — `dbwatch version` shows version, commit hash, and build date via ldflags
- **GoReleaser** — automated multi-platform builds (Linux/macOS/Windows × amd64/arm64) and Docker multi-arch images to ghcr.io
- **GitHub Actions** — CI workflow (test + vet on push/PR) and release workflow (triggered by version tags)
- **Test scripts** — `scripts/start-postgres.sh` and `scripts/seed.sql` for local development

[Unreleased]: https://github.com/rifqiagniamubarok/dbwatcher/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rifqiagniamubarok/dbwatcher/releases/tag/v0.1.0
