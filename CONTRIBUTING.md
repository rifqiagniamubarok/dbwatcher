# Contributing to DBWatch

Thanks for your interest in DBWatch. This guide covers how the repo is organized, how we expect changes to land, and the conventions we follow.

For technical design, see [ARCHITECTURE.md](ARCHITECTURE.md). For the testing playbook, see [TESTING.md](TESTING.md). For the in-repo agent guide, see [CLAUDE.md](CLAUDE.md).

## Code of conduct

Be respectful. Disagree with ideas, not with people. Assume good faith.

## Getting started

### Prerequisites

- **Go 1.22+** — `slog` and mature generics
- **Docker** — to run the test Postgres
- **make** — task runner
- **git** — obviously

### Local setup

```bash
git clone https://github.com/rifqiagniamubarok/dbwatcher.git
cd dbwatcher
go mod download
make build
./bin/dbwatch version
```

### Test database

```bash
./scripts/start-postgres.sh
# Postgres 16 running on localhost:5433, db `test`, user/password test/test
```

### Run tests

```bash
make test                            # unit tests with -race
go test -tags=integration ./...      # integration tests (needs Postgres up)
```

## Branch strategy

We follow a lightweight trunk-with-feature-branches model:

- **`master`** — protected. Always releasable. Only updated via PR.
- **`feature/<short-name>`** — new functionality. Branched from `master`.
- **`fix/<short-name>`** — bug fixes. Branched from `master`.
- **`docs/<short-name>`** — documentation-only changes.
- **`release/v<x.y.z>`** — release prep (changelog, version bump). Optional, used when a release needs more than a tag.

Keep branches short-lived. Rebase onto `master` instead of merging if your branch falls behind.

## Commit messages

We use a relaxed [Conventional Commits](https://www.conventionalcommits.org/) style:

```
<type>(<scope>): <short description>

<optional body>

<optional footer>
```

**Types we use:**

- `feat` — new feature
- `fix` — bug fix
- `docs` — documentation only
- `refactor` — code change that doesn't add a feature or fix a bug
- `test` — adding or fixing tests
- `chore` — tooling, build, CI changes
- `perf` — performance improvement

**Scopes** are loose — usually a package name (`listener`, `store`, `tui`, `ipc`, `config`) or `release`, `ci`, `docs`.

**Examples:**

```
feat(listener): handle TOAST unchanged columns in UPDATE
fix(store): drop slow-subscriber events instead of blocking
docs: clarify wal_level setup in README
chore(ci): bump golangci-lint to v1.55
```

During the early phase walkthrough, you'll also see commits like `phase-3: implement Bubble Tea TUI` — that's an internal convention for the initial phased build, not required for ongoing contributions.

**Don't:**

- Squash unrelated changes into one commit
- Use generic messages like "fix bug" or "update code"
- Bypass hooks (`--no-verify`) without a clear reason
- Amend commits that have already been pushed and reviewed

## Pull requests

### Before opening

- [ ] `make test` passes locally (with `-race`)
- [ ] `go vet ./...` is clean
- [ ] New public APIs have at least one unit test
- [ ] Manual smoke test for behavior that touches Postgres or the TUI (see [TESTING.md](TESTING.md))
- [ ] No unrelated formatting/whitespace changes
- [ ] CHANGELOG entry under `[Unreleased]` if user-visible

### PR description

Keep it short, but include:

1. **What** changed — one sentence
2. **Why** — the motivation or the issue this fixes
3. **How** — non-obvious design choices, if any
4. **Test plan** — what you ran to convince yourself it works

Link related issues with `Fixes #123` or `Refs #45`.

### Review expectations

- One reviewer is enough for small PRs
- Larger PRs (new package, behavior change) get two
- Address all comments before merging — either with code changes or a reply explaining why not
- Reviewer merges. Use **squash merge** unless the branch has a coherent multi-commit history worth preserving

## Release process

DBWatch follows [Semantic Versioning](https://semver.org/).

### Cutting a release

1. Update `CHANGELOG.md`:
   - Move items from `[Unreleased]` into a new `[X.Y.Z] — YYYY-MM-DD` section
   - Add a fresh empty `[Unreleased]` block
   - Update the comparison links at the bottom
2. Open a PR titled `release: vX.Y.Z` and merge it
3. Tag the merge commit:

   ```bash
   git checkout master && git pull
   git tag -a vX.Y.Z -m "Release vX.Y.Z"
   git push origin vX.Y.Z
   ```

4. GitHub Actions (`release.yml`) picks up the tag and runs GoReleaser:
   - Builds binaries for linux/darwin/windows × amd64/arm64
   - Publishes the GitHub Release with checksums
   - Pushes multi-arch Docker images to `ghcr.io/rifqiagniamubarok/dbwatcher`
5. Verify post-release:
   - `go install github.com/rifqiagniamubarok/dbwatcher/cmd/dbwatch@vX.Y.Z`
   - `docker run --rm ghcr.io/rifqiagniamubarok/dbwatcher:vX.Y.Z version`

### Version bumps

- **PATCH** (`0.1.0 → 0.1.1`) — backward-compatible bug fixes
- **MINOR** (`0.1.0 → 0.2.0`) — new features, no breaking changes
- **MAJOR** (`0.1.0 → 1.0.0`) — breaking changes. While we're pre-1.0, breaking changes can also land in MINOR — call them out clearly in the changelog

## Code style

See [CLAUDE.md](CLAUDE.md) for the full style guide. Highlights:

- **Naming:** packages lowercase singular (`listener`, not `listeners`); exports are PascalCase; errors are `ErrSomething`
- **Errors:** wrap with context using `fmt.Errorf("decode insert message: %w", err)`. User-facing CLI errors must be actionable.
- **Concurrency:** every goroutine has a clear exit path. `context.Context` on every blocking call. Always run `go test -race`.
- **Logging:** `log/slog` only. Don't spam `Info` in hot paths. CLI user output is not slog.
- **Dependencies:** minimal. Anything outside the approved list in [CLAUDE.md](CLAUDE.md#tech-constraints) needs justification before `go get`.

### Package layout rules

- `cmd/` — entry points and Cobra wiring only. No business logic.
- `internal/store/` — imports nothing from `internal/`
- `internal/listener/` — may import `internal/store/`
- `internal/tui/` — may import `internal/store/`. Must not import `internal/listener/` directly.
- `internal/config/` — standalone

## Testing

Three tiers (full detail in [TESTING.md](TESTING.md)):

1. **Unit tests** — required for `internal/listener/decoder.go`, `schema_cache.go`, and everything in `internal/store/`. Use table-driven tests for functions with many cases. Mock minimally.
2. **Integration tests** — gated by `//go:build integration`. Require Postgres. Run with `go test -tags=integration ./...`.
3. **Manual tests** — checklists per phase in [TESTING.md](TESTING.md). Run them when changes touch the Listener or the TUI.

Coverage targets: `internal/store/` ≥ 80%, decoder and schema cache ≥ 70%, rest best-effort.

## Reporting bugs

Open an issue with:

- **DBWatch version** — `dbwatch version`
- **Postgres version** — `SELECT version();`
- **OS / arch** — `uname -a` or Windows version
- **Steps to reproduce** — minimal SQL or commands
- **Expected vs. actual** — what should happen, what actually happens
- **Logs** — set `--log-level=debug` and include the output

For Postgres-replication oddities, the LSN and the raw pgoutput message (debug logs) are gold.

## Suggesting features

Open a discussion or an issue tagged `enhancement` first — don't open a PR cold for a new feature. Big features may belong in a future phase rather than master immediately; check [ARCHITECTURE.md](ARCHITECTURE.md#future-extension-points).

## License

By contributing, you agree your contributions are licensed under the project's [MIT License](LICENSE).
