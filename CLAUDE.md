# CLAUDE.md

Context for Claude Code when working on this project. Read this at the start of every session before doing anything.

## Project context

DBWatch is a CLI tool for monitoring Postgres database changes in realtime in the terminal. Target users: developers doing testing/debugging. **This is not a production tool.**

For full context:
- `ARCHITECTURE.md` — technical design, folder structure, tech stack
- `CONTRIBUTING.md` — branching, commit format, release process

## Working principles

### 1. One task at a time

Do not work on multiple tasks simultaneously, even if they seem simple. Finish one task and confirm its expected outcome before moving on.

### 2. Test-first for critical logic

For the following components, **write tests before the implementation**:
- `internal/listener/decoder.go` — binary protocol parser, many edge cases
- `internal/listener/schema_cache.go` — cache invalidation
- `internal/store/store.go` — concurrency, ring buffer, pub/sub

For UI (TUI components), test-first is not required — visual iteration is faster.

### 3. Small commits

Commit after each task is complete. Follow the commit format in `CONTRIBUTING.md`:
```
<type>(<scope>): <short description>
```

### 4. No premature abstraction

Do not create an interface if there is only one implementor. Wait until polymorphism is genuinely needed (at the earliest, Phase 5 or when multi-DB support is added).

### 5. Don't expand scope

If an interesting feature idea comes up that is not in the current plan, **add it to `IDEAS.md`** and do not implement it. Discuss with the user first.

## Tech constraints

- **Go version:** 1.22 or newer (for `slog`, mature generics)
- **Dependencies:** minimal. Before adding a new dependency, justify why the standard library is not sufficient.
- **Approved dependencies:**
  - `github.com/jackc/pglogrepl` — logical replication
  - `github.com/jackc/pgx/v5` — Postgres driver
  - `github.com/spf13/cobra` — CLI parsing
  - `github.com/charmbracelet/bubbletea` — TUI
  - `github.com/charmbracelet/lipgloss` — styling
  - `github.com/charmbracelet/bubbles` — TUI components
  - `github.com/mattn/go-isatty` — TTY detection
  - `github.com/stretchr/testify` — assertion helpers in tests

Ask before running `go get` for anything not listed above.

## Code style

### Naming

- Package: lowercase, singular (`listener` not `listeners`)
- Exported type/function: PascalCase
- Error variable: `ErrSomething`
- Test function: `TestXxx`, table-driven tests named `TestXxx_Scenario`

### Error handling

- Wrap errors with context: `fmt.Errorf("decode insert message: %w", err)`
- User-facing errors (shown in CLI output) must be actionable — include a hint on what to do
- Internal errors (in library code) just wrap with context, do not reformat

### Concurrency

- Always run `go test -race ./...` before committing
- Use `context.Context` in every function that can block (network calls, long channel receives)
- Every goroutine must have a clear exit path — no goroutine should be unstoppable
- Buffered channels for pub/sub (default capacity 100), unbuffered for synchronization signals

### Logging

- Use `log/slog` (stdlib)
- Level guidance:
  - `Debug` — internal details useful during investigation (LSN, message type, etc.)
  - `Info` — normal milestones (connected, slot created, etc.). **Avoid in hot paths.**
  - `Warn` — something is not ideal but can continue (TOAST value, replica identity not FULL)
  - `Error` — operation failed
- CLI-facing output (what the end user sees) should **not** use slog — use plain readable output

## File organization

See `ARCHITECTURE.md` "Folder structure" for the full layout.

Additional rules:
- `cmd/` may only contain entry points and wiring. All logic goes in `internal/`
- `internal/` packages must not import other `internal/` packages at a higher level. Hierarchy:
  - `store` must not import anything from internal
  - `listener` may import `store`
  - `tui` may import `store`, must not import `listener`
  - `config` is standalone
- Test files go in the same package (`store_test.go` in package `store`), except integration tests that require a test database

## Testing strategy

### Unit tests

- Every public function in `internal/listener/decoder.go`, `schema_cache.go`, and all of `internal/store/` must have unit tests
- Use table-driven tests for functions with many cases
- Minimize mocks — if you can use the real struct, use it

### Integration tests

- File: `internal/listener/integration_test.go` with build tag `//go:build integration`
- Requires a running Postgres instance, skipped in normal unit test runs
- Run with: `go test -tags=integration ./...`

### Manual tests

- At the end of each phase, run the scenarios in `TESTING.md`
- Record results in the "Verified scenarios log" section

## Anti-patterns to avoid

1. **Goroutine leak.** Every `go func()` must have a way to stop via context cancellation or channel close.
2. **Unbounded channel.** `chan Event` channels in pub/sub must be buffered, and the handler must drop events when full rather than block.
3. **Print in library code.** `internal/` packages must not call `fmt.Println`. Use slog or return an error.
4. **Hardcoded values.** Capacity, timeout, retry counts — all must be in config or named constants.
5. **Big bang refactor.** If structure needs to change, do it incrementally. Do not rewrite an entire package at once.

## When stuck

If a task feels too large or ambiguous:
1. Break it into smaller sub-tasks and write them out in chat first
2. Ask the user: "Should I work on A or B first?"
3. If a design decision is not clear from `ARCHITECTURE.md`, **ask the user — do not assume**

If a test fails and is hard to debug:
1. Add Debug logs
2. Reproduce with a minimal example
3. If stuck for more than 30 minutes, surface it to the user with a summary of what has been tried
