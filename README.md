# DBWatch

> `tail -f` for your Postgres database. Watch inserts, updates, and deletes in realtime while you develop.

**Status:** Work in progress. See [`PLAN.md`](./PLAN.md) for roadmap.

## What is this?

DBWatch is a CLI tool for developers. When you're testing or debugging code that touches Postgres, DBWatch shows you every change as it happens — directly in your terminal, with diff view for updates.

Think of it as `tail -f` for your database.

**This is a dev tool**, not a production observability solution. For production use cases, look at Debezium, pgaudit, or similar.

## Quick Start

> **Note:** Quick start instructions will be finalized at v0.1.0 release. For now, run from source:

```bash
git clone https://github.com/<user>/dbwatch.git
cd dbwatch
make build
./bin/dbwatch tail --db-url=postgres://user:pass@localhost:5432/mydb
```

## Postgres Setup

DBWatch uses Postgres logical replication. Your database needs:

1. `wal_level = logical` in `postgresql.conf` (requires restart)
2. A user with `REPLICATION` privilege
3. A publication for the tables you want to watch

For local dev with Docker:

```bash
docker run -d \
  --name pg-dev \
  -e POSTGRES_PASSWORD=dev \
  -p 5432:5432 \
  postgres:16 \
  -c wal_level=logical
```

Detailed setup guide will be added at v0.1.0.

## Documentation

- [`ARCHITECTURE.md`](./ARCHITECTURE.md) — technical design
- [`PLAN.md`](./PLAN.md) — development roadmap
- [`CONTRIBUTING.md`](./CONTRIBUTING.md) — how to contribute (coming soon)

## License

MIT — see [`LICENSE`](./LICENSE).
