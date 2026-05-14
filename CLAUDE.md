# CLAUDE.md

Context for Claude Code when working on this project. Read this file at the start of every session before starting any task.

## Project context

DBWatch is a CLI tool for monitoring Postgres database changes in realtime from the terminal. Target user: developers who are testing or debugging. **This is not a production tool.**

For full context:
- `ARCHITECTURE.md` — technical design, folder structure, tech stack
- `CHANGELOG.md` — what has shipped (`[0.1.0]`) and what is in flight (`[Unreleased]`)
- `PLAN.md` — phase plan, task list, expected outcome (local only, gitignored — exists on the user's machine, not the public repo)

At the start of every session, ask the user: "Which phase and task are we on?" before writing any code.

## Working principles

### 1. One task at a time
Don't work on multiple tasks at once, even if they look simple. Finish one task until its expected outcome is met, then move on.

### 2. Test-first for critical logic
For the following components, **write tests before implementation**:
- `internal/listener/decoder.go` — binary protocol parser, many edge cases
- `internal/listener/schema_cache.go` — cache invalidation
- `internal/store/store.go` — concurrency, ring buffer, pub/sub

For UI (TUI components), test-first is not required — visual iteration is faster.

### 3. Small commits
Commit after each task in PLAN.md is done. Commit message format:
```
phase-N: <task summary>
```
Examples: `phase-1: implement schema cache`, `phase-1: add decoder unit tests`.

### 4. No premature abstraction
Don't create an interface if there's only one implementor. Wait until you genuinely need polymorphism (e.g. when multi-DB or a second Listener implementation lands).

### 5. Don't expand scope
If an interesting feature comes to mind mid-task and it isn't in PLAN.md, **write it down in `IDEAS.md`**, don't implement it directly. Discuss with the user first.

## Tech constraints

- **Go version:** 1.22 or newer (for `slog`, mature generics)
- **Dependencies:** minimal. Before adding a new dependency, justify why the standard library is not enough.
- **Approved dependencies:**
  - `github.com/jackc/pglogrepl` — logical replication
  - `github.com/jackc/pgx/v5` — Postgres driver
  - `github.com/spf13/cobra` — CLI parsing
  - `github.com/charmbracelet/bubbletea` — TUI
  - `github.com/charmbracelet/lipgloss` — styling
  - `github.com/charmbracelet/bubbles` — TUI components
  - `github.com/mattn/go-isatty` — TTY detection
  - `github.com/stretchr/testify` — test assertion helper

For anything else, ask before running `go get`.

## Code style

### Naming
- Package: lowercase, singular (`listener`, not `listeners`)
- Exported type / function: PascalCase
- Error variable: `ErrSomething`
- Test function: `TestXxx`, table-driven tests named `TestXxx_Scenario`

### Error handling
- Wrap errors with context: `fmt.Errorf("decode insert message: %w", err)`
- User-facing errors (shown on the CLI) must be actionable. See task 4.1 in PLAN.md for the format.
- Internal errors (in library code) just need wrapping with context — don't reformat.

### Concurrency
- Always run `go test -race ./...` before committing
- Use `context.Context` in every function that can block (network call, long channel receive)
- Every goroutine must have a clear exit path. There must be no goroutine that cannot be stopped.
- Buffered channels for pub/sub (capacity 100 default), unbuffered for synchronization signals.

### Logging
- Use `log/slog` (stdlib)
- Level guidance:
  - `Debug` — internal detail useful during investigation (LSN, message type, etc.)
  - `Info` — normal milestones (connected, slot created, etc.). **Avoid spam in hot paths.**
  - `Warn` — something is not ideal but the program can continue (TOAST value, replica identity not FULL)
  - `Error` — operation failed
- End-user CLI output is **not** slog — use readable formatting.

## File organization

See `ARCHITECTURE.md` section "Folder structure" for the full layout.

Additional rules:
- `cmd/` is for entry point and wiring only. Logic lives in `internal/`.
- An `internal/` package must not import another `internal/` package that sits "higher" in the layer hierarchy. Current layers (lowest first):
  - `store` — imports nothing from `internal/`
  - `config` — standalone
  - `listener` — may import `store`
  - `ipc` — may import `store` (transport for events)
  - `daemon` — standalone (PID/lifecycle utilities only, no domain types)
  - `core` — may import `store`, `listener`, `config` (orchestrates them)
  - `tui` — may import `store` and `ipc` (must NOT import `listener` directly — go through `core` or `ipc`)
- Test files live in the same package (`store_test.go` in package `store`), except integration tests that need a test database.

## Testing strategy

### Unit test
- Every public function in `internal/listener/decoder.go`, `schema_cache.go`, and all of `internal/store/` must have unit tests
- Use table-driven tests for functions with many cases
- Mock minimally — if you can use the real struct, use the real struct

### Integration test
- `internal/listener/integration_test.go` with build tag `//go:build integration`
- Requires a running Postgres, skipped in normal unit-test runs
- Run with: `go test -tags=integration ./...`

### Manual test
- At the end of every phase, run the "Expected Outcome" scenario from PLAN.md
- Record results in `TESTING.md` under "Verified scenarios"

## Anti-patterns to avoid

1. **Goroutine leak.** Every `go func()` must have a way to stop via context or channel close.
2. **Unbounded channel.** Pub/sub `chan Event` must be buffered, and the handler must drop when full.
3. **Print in library code.** Library code in `internal/` must not `fmt.Println`. Use slog or return an error.
4. **Hardcoded value.** Capacity, timeout, retry — all in config or a clearly named constant.
5. **Big bang refactor.** If the structure needs to change, do it incrementally. Don't rewrite an entire package at once.

## When stuck

If a task feels too large or ambiguous:
1. Break it into smaller sub-tasks, write them in chat first
2. Ask the user: "Should I work on A first, or B first?"
3. If a design decision isn't clear from ARCHITECTURE.md, **ask the user, don't assume**

If a test fails and is hard to debug:
1. Add Debug logs
2. Reproduce with a minimal example
3. If you're stuck for more than 30 minutes, surface it to the user with a summary of what you've tried
