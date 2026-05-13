#!/usr/bin/env bash
set -euo pipefail

CONTAINER=dbwatch-test-pg
IMAGE=postgres:16
PORT=5432
DB_URL="postgres://test:test@localhost:${PORT}/test"

echo "==> Stopping existing container (if any)..."
docker rm -f "$CONTAINER" 2>/dev/null || true

echo "==> Starting Postgres with wal_level=logical..."
docker run -d \
  --name "$CONTAINER" \
  -e POSTGRES_USER=test \
  -e POSTGRES_PASSWORD=test \
  -e POSTGRES_DB=test \
  -p "${PORT}:5432" \
  "$IMAGE" \
  -c wal_level=logical \
  -c max_replication_slots=10 \
  -c max_wal_senders=10

echo -n "==> Waiting for Postgres to be ready"
for i in $(seq 1 30); do
  if docker exec "$CONTAINER" pg_isready -U test -d test -q 2>/dev/null; then
    echo " ready."
    break
  fi
  echo -n "."
  sleep 1
done

echo "==> Seeding schema..."
docker exec -i "$CONTAINER" psql -U test -d test < "$(dirname "$0")/seed.sql"

echo ""
echo "Test database ready."
echo "Connection string: $DB_URL"
