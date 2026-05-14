# Changelog

All notable changes to DBWatch are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Planned

- **Phase 5 — Daemon mode:** run `dbwatch` as a background process and attach a TUI from any terminal. New subcommands `dbwatch daemon {start,stop,status,list}` and `dbwatch attach`. Uses a Unix domain socket and newline-delimited JSON. Multiple clients can attach to the same daemon. See `PLAN.md` and `ARCHITECTURE.md`.
- **Phase 6 — Web UI:** optional HTTP + WebSocket dashboard via `dbwatch serve`. Embedded frontend, shares the same Store as CLI and daemon.

## [0.1.0] — 2026-05-14

First public release. MVP CLI that streams Postgres logical replication events to a terminal TUI with diff view.

### Added

- **Listener** (`internal/listener/`) — Postgres logical replication consumer using `jackc/pglogrepl` and `jackc/pgx/v5`. Decodes `pgoutput` INSERT / UPDATE / DELETE messages into structured `Event` values. Maintains a schema cache keyed by relation OID, refreshed on `RelationMessage`. Auto-creates the publication and a temporary replication slot. Periodic LSN acknowledgement.
- **Store** (`internal/store/`) — ring buffer (configurable, default 1000) plus pub/sub. Non-blocking broadcast — slow subscribers get events dropped on their channel rather than blocking the listener. Supports filter-aware subscriptions (`AllowAllFilter`, `TableFilter`).
- **TUI** (`internal/tui/`) — Bubble Tea renderer with live feed, sidebar table filter, expandable detail view, diff view for UPDATE (old → new with color), pause/resume, clear-with-confirm, help overlay. Adaptive color via lipgloss. Keybindings: `j/k/g/G` navigation, `enter` expand, `space` pause, `f` filter focus, `c` clear, `?` help, `q` / `Ctrl+C` quit.
- **CLI** (`cmd/dbwatch/`) — Cobra-based with `tail` and `version` subcommands. Flags / env vars for `--db-url`, `--publication`, `--slot`, `--buffer`, `--log-level`, `--output`.
- **Non-TTY mode** — when stdout is not a terminal (piped to `jq`, `grep`, or a file), the TUI is skipped and events are emitted as newline-delimited JSON. Detected via `mattn/go-isatty`. Can be forced with `--output=json` or `--output=tui`.
- **Friendly error messages** for the common Postgres setup mistakes: connection refused, `wal_level != logical`, missing `REPLICATION` privilege, stale replication slot, TLS misconfiguration.
- **Periodic stats** on stderr (does not pollute stdout JSON): `received`, `buffered`, `subscribers`.
- **Versioning** via ldflags — `dbwatch version` reports version, commit, build date, and Go version.

### Distribution

- **GoReleaser** — multi-platform builds (linux / darwin / windows × amd64 / arm64, excluding windows-arm64), tar.gz / zip archives, checksums file, auto-generated changelog from conventional commit messages.
- **Docker** — multi-arch image published to `ghcr.io/rifqiagniamubarok/dbwatcher`. Uses a slim `Dockerfile.goreleaser` that copies the pre-built binary.
- **GitHub Actions** — `ci.yml` runs `go vet` and `go test -race` on push and PR against a Postgres service container. `release.yml` triggers GoReleaser on `v*.*.*` tags.
- **`go install`** — installable via `go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@latest`.

### Documentation

- `README.md` — pitch, demo mockup, quick start, installation paths, Postgres setup, flag reference, keybindings, troubleshooting.
- `ARCHITECTURE.md` — component breakdown (Listener, Store, TUI), data model, configuration, future extension points.
- `TESTING.md` — test layers (unit / integration / manual), per-phase manual test checklist, edge cases (TOAST, REPLICA IDENTITY, schema change at runtime, truncate, rollback).
- `CONTRIBUTING.md` — branch strategy, commit-message format, PR rules, release process.
- `CLAUDE.md` — working principles for AI-assisted development on this repo.

### Known limitations

- Postgres only. MySQL / other databases out of scope for v0.1.
- No persistent storage — events live in memory only. Ring buffer overwrites the oldest events.
- Single-process — TUI and Listener run together. Background-daemon / multi-client attach is planned for Phase 5.
- No authentication or remote access. Intended for local development.
- `TRUNCATE` events are currently ignored (definitive behavior TBD).

[Unreleased]: https://github.com/rifqiagniamubarok/dbwatcher/compare/v0.1.0...HEAD
[0.1.0]: https://github.com/rifqiagniamubarok/dbwatcher/releases/tag/v0.1.0
