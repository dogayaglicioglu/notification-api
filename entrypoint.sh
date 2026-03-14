#!/bin/sh
set -e

APP_MODE=${APP_MODE:-api}

if [ "$APP_MODE" = "worker" ] || [ "$APP_MODE" = "queueWorker" ]; then
  echo "Starting worker mode..."
  exec ./notif-queueWorker
fi

if [ "$APP_MODE" = "outbox-worker" ]; then
  echo "Starting outbox worker mode..."
  exec ./notif-outbox-worker
fi

echo "Waiting for PostgreSQL to be ready..."
until PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -c '\q' 2>/dev/null; do
  echo "PostgreSQL is unavailable - sleeping"
  sleep 1
done

echo "PostgreSQL is up - running migrations"

# Run migrations
for migration in /migrations/*.up.sql; do
  if [ -f "$migration" ]; then
    echo "Running migration: $migration"
    PGPASSWORD=$DB_PASSWORD psql -h "$DB_HOST" -U "$DB_USER" -d "$DB_NAME" -f "$migration"
  fi
done

echo "Starting API mode..."
exec ./notif-api
