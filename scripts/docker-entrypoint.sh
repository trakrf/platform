#!/bin/sh
set -e

echo "🗄️  Running database migrations..."

# Run migrations (path must match Dockerfile COPY)
migrate -path /app/database/migrations -database "$PG_URL" up

echo "✅ Migrations complete"
echo "🚀 Starting server..."

# exec replaces shell process with server (proper signal handling)
exec "$@"
