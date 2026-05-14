# Contributing to DBWatch

Internal development guide. Read this before writing any code.

---

## Branch strategy

```
main         ← production-ready, protected. Only accepts merges from release/*
development  ← integration branch. Base for all feature branches
feature/*    ← one branch per feature or task
fix/*        ← bug fixes
release/*    ← release preparation (version bump, CHANGELOG update)
```

### Workflow

```
feature/your-feature
        ↓  PR
   development
        ↓  PR (once all features are ready)
  release/v0.x.x
        ↓  PR
        main  ← tag here → GitHub Actions auto-release
```

### Branch rules

- **Never commit directly to `main` or `development`**
- All changes go through a Pull Request
- Branch names: lowercase, words separated by `-`
  - ✅ `feature/filter-sidebar`
  - ✅ `fix/tui-cursor-overflow`
  - ❌ `FeatureFilterSidebar`, `my-branch`
- Delete branches after the PR is merged

### Creating a new branch

```bash
# Always branch off development
git checkout development
git pull origin development
git checkout -b feature/your-feature-name
```

---

## Commit messages

Required format: **`<type>(<scope>): <short description>`**

```
<type>(<scope>): <short description>

[optional body — explain WHY, not WHAT]

[optional footer — breaking change, closes issue]
```

### Types

| Type | When to use |
| --- | --- |
| `feat` | New feature |
| `fix` | Bug fix |
| `refactor` | Code change with no new feature or bug fix |
| `test` | Add or update tests |
| `docs` | Documentation changes only |
| `chore` | Maintenance: deps, config, CI, etc. |
| `perf` | Performance improvement |

### Scopes (optional but recommended)

`listener`, `store`, `tui`, `config`, `cmd`, `ci`, `deps`

### Good commit message examples

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

### Commit rules

- Description: **lowercase**, **imperative mood** ("add" not "added"), **no trailing period**
- Max 72 characters on the first line
- Explain *why* in the body, not *what* — the code already shows what changed
- One commit = one logical change. Don't mix a bug fix with a refactor
- Small commits are better than one large commit

---

## Pull Requests

### Before opening a PR

```bash
# All tests must pass
go test -race ./...

# No vet warnings
go vet ./...

# Code must be formatted
gofmt -l .
```

### PR title

Follow the commit message format: `feat(scope): short description`

### PR description template

```
## What
[What changed — one paragraph]

## Why
[Why this change is needed]

## How to test
- [ ] Step 1
- [ ] Step 2
```

### Review rules

- At least 1 approval before merging
- All CI checks must be green
- Resolve all comments before merging
- Use **Squash and merge** for feature branches into development
- Use **Merge commit** for release branches into main

---

## Release process

1. Create a `release/vX.Y.Z` branch from `development`
2. Update `CHANGELOG.md` — move items from `[Unreleased]` to `[X.Y.Z] - YYYY-MM-DD`
3. Open a PR to `main`
4. After merge, tag on `main`:

```bash
git checkout main
git pull origin main
git tag v0.2.0
git push origin v0.2.0
```

GitHub Actions will automatically build and publish the release.

---

## Versioning

DBWatch follows [Semantic Versioning](https://semver.org/):

- **PATCH** `v0.1.1` — bug fix, no API changes
- **MINOR** `v0.2.0` — new features, backward compatible
- **MAJOR** `v1.0.0` — breaking changes

---

## Development setup

```bash
# Clone and setup
git clone https://github.com/rifqiagniamubarok/dbwatcher.git
cd dbwatcher
git checkout development

# Build
make build

# Run tests
make test

# Start test database (requires Docker)
./scripts/start-postgres.sh

# Run
./bin/dbwatch tail \
  --db-url="postgres://test:test@localhost:5433/test?sslmode=disable&replication=database"
```

---

## Folder structure

```
cmd/dbwatch/        ← entry point and wiring only, no business logic
internal/
  listener/         ← WAL streaming and decoding
  store/            ← ring buffer and pub/sub
  tui/              ← Bubble Tea TUI
  config/           ← config loader
scripts/            ← dev helper scripts
.github/workflows/  ← CI/CD
```

### Package import rules

- `store` → must not import any other internal package
- `listener` → may import `store`
- `tui` → may import `store`, must not import `listener`
- `config` → standalone

---

## If you get stuck

1. Check `ARCHITECTURE.md` for system design context
2. Open an issue on GitHub with the `question` label
