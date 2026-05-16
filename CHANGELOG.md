# Changelog

All notable changes to DBWatch are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] — 2026-05-15

Marker HTTP API. External tools (test runners, deploy scripts, ad-hoc curl) can now push markers (separator lines) and log entries into the DBWatch feed over a tiny localhost HTTP API. Markers make the live feed read like a timeline instead of a firehose.

### Added

- **Marker HTTP API** (`internal/markerapi/`) — `POST /marker` (text or JSON, with an optional color), `POST /log` (free-form line), `GET /health` (status + uptime). Default bind `127.0.0.1:6677`. Body capped at 4 KiB, label / message capped at 200 chars, color validated against the allow-list (`default`, `yellow`, `green`, `red`, `blue`, `dim`).
- **Auto-start in both modes.** `dbwatch tail` and `dbwatch daemon start` both bring up the marker server by default. The detached child inherits the parent's `--marker-*` flags so the port stays consistent across `start --detach`.
- **TUI rendering for markers and logs.** Markers render as a separator line spanning the feed width (`──── label ────`) in the chosen color; log entries render inline with a `[log]` tag. Both bypass the table filter — they were pushed deliberately. Detail view (`enter`) shows label/color/timestamp for markers and message/timestamp for logs.
- **New TUI keybindings.** `[` and `]` jump the cursor to the previous / next marker; `M` drops every item that arrived before the most recent marker (useful between test runs). Footer hint and `?` help overlay updated.
- **Extended `Event` type.** New `Kind` discriminator (`event` default, `marker`, `log`) and optional `Label` / `Color` / `Message` fields. Backward compatible: a legacy `Event` JSON payload without `kind` decodes as a database event. Helper constructors `store.NewMarker(label, color)` and `store.NewLog(message)`.
- **Flags:** `--marker-port` (default `6677`), `--marker-bind` (default `127.0.0.1`), `--no-marker`, all shared by `tail` and `daemon start`.

### Documentation

- README: new "Test Integration (Marker HTTP API)" section with curl, Go, Node.js (Jest), Python (pytest), and GitHub Actions examples. Keybindings table updated.

## [0.2.0] — 2026-05-14

Daemon mode. DBWatch can now run as a long-lived background process while one or more TUI / JSON clients attach and detach freely. `dbwatch tail` continues to work as a foreground all-in-one.

### Added

- **Daemon mode:** `dbwatch daemon {start,stop,status,list,logs}` and `dbwatch attach`. The daemon keeps a single Listener+Store process alive and serves clients over a Unix domain socket with NDJSON envelopes (`hello`, `snapshot`, `event`, `stats`, `pong`).
- **Core runner extraction:** shared `internal/core/runner.go` wires the Listener into the Store for both `tail` and `daemon` modes — no duplication.
- **IPC transport package** (`internal/ipc/`): protocol envelope, socket / PID / log path resolution (`ResolveSocketPath`, `ResolvePIDPath`, `ResolveLogPath`), server with periodic `stats` broadcasts (5s ticker), client exposing `Events()`, `Stats()`, `Errors()`, `Dropped()`, and `RequestStats(ctx)`. Includes roundtrip and path-resolution tests.
- **Daemon lifecycle helpers** (`internal/daemon/`): PID file I/O, signal-based stop with SIGTERM → SIGKILL escalation after a timeout, stale-file cleanup, log truncation when the file exceeds a size cap.
- **Detach support:** `--detach` forks via an internal `--daemon-child` flag, redirects stdio to the per-name log file, and returns to the shell immediately.
- **Service templates:** `docs/dbwatch.service` (systemd) and `docs/dbwatch.plist` (launchd).
- **Configuration:** `DBWATCH_SOCKET_DIR` env var to override the runtime directory; `--name` flag for multi-daemon hosts; `--follow` for `daemon logs`; local-only `--buffer` for `attach`.
- **IPC idle-client reaper.** The server applies a 60s read deadline that resets on every inbound `ping` / `subscribe`. Hung clients no longer occupy a Store subscription indefinitely.
- **Observable IPC client drops.** When a slow `attach` consumer overflows the local 100-event channel, the client records the count (exposed via `Client.Dropped()`) and emits a rate-limited `Debug` log instead of silently swallowing the event.

### Changed

- **README** rewritten around a single coherent example database (`user=local`, `password=local`, `database=test`, `port=5432`), with a new "Adapting the connection URL" section that spells out every placeholder.
- **ARCHITECTURE / CLAUDE** updated to reflect the shipped daemon mode rather than treating it as a "planned phase". Package import hierarchy in `CLAUDE.md` now covers `core/`, `ipc/`, and `daemon/`.
- **CONTRIBUTING.md** added, covering branch strategy, commit conventions, PR rules, and the release process.

### Fixed

- **Windows cross-compilation.** `internal/daemon/` and `cmd/dbwatch/` split POSIX-specific bits (`syscall.Kill`, `Setsid`) into `_unix.go` files; Windows stubs return `daemon.ErrUnsupportedPlatform`. `go build` and `go vet` now succeed for `GOOS=windows`; the binary still refuses `daemon start --detach` on Windows with a clear message.

### Known limitations

- Daemon process management (`daemon start --detach`, `daemon stop`) is Linux / macOS only. Windows builds compile cleanly but the daemon subcommands return `ErrUnsupportedPlatform`; use `dbwatch tail` on Windows.
- The IPC `subscribe` envelope is a protocol placeholder — every attached client currently receives all events. Per-client table filtering is deferred. The server logs at Debug level when it ignores a subscribe filter.

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

[Unreleased]: https://github.com/rifqiagniamubarok/dbwatcher/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/rifqiagniamubarok/dbwatcher/releases/tag/v0.3.0
[0.2.0]: https://github.com/rifqiagniamubarok/dbwatcher/releases/tag/v0.2.0
[0.1.0]: https://github.com/rifqiagniamubarok/dbwatcher/releases/tag/v0.1.0
