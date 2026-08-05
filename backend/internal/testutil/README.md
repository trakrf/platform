# Test Utilities

Utilities for integration testing with real database.

## Prerequisites

Install the `migrate` CLI tool (one-time setup):

```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

Make sure it's in your PATH or in `~/go/bin/`.

## Usage

### Integration Tests

```go
func TestSomething(t *testing.T) {
    if testing.Short() {
        t.Skip("Skipping integration test")
    }

    // Creates test database, runs migrations, returns storage
    store := testutil.SetupTestDatabase(t)

    // Test your code...
    // Database is automatically cleaned up after test
}
```

### Running Tests

```bash
# Run integration tests (requires database)
go test ./...

# Skip integration tests (fast)
go test ./... -short
```

## How It Works

1. **Creates test database**: Connects to postgres and creates `trakrf_test` database
2. **Runs migrations**: Shells out to `migrate` CLI (uses existing migration scripts)
3. **Returns storage**: Provides a `storage.Storage` instance connected to test DB
4. **Cleans up**: Truncates all tables after test completes

## Environment Variables

- `PG_ADMIN_URL` - **superuser** connection to the maintenance database (default:
  `postgresql://postgres:postgres@localhost:5432/postgres?sslmode=disable`). The harness
  drops and recreates `trakrf_test` and creates the `trakrf_test_app` role, so it needs
  privileges the application deliberately does not have. Not `PG_URL`: since TRA-1075 that
  is the non-superuser `trakrf-app` role on the `trakrf` database, and neither is any use
  here.
- `TEST_PG_URL` - Test database connection string (default: derived from `PG_ADMIN_URL` with
  the database name swapped for `trakrf_test`)

## Architecture

- **No Go migration library** - Uses existing `migrate` CLI via `exec.Command`
- **Real database** - Tests run against actual Postgres, not mocks
- **Isolated tests** - Each test gets clean tables via TRUNCATE
- **Docker-aware** - Automatically converts `timescaledb` hostname to `localhost` for local testing

## Troubleshooting

### "migrate binary not found"

Install it:
```bash
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest
```

### "failed to connect to postgres"

Make sure Postgres is running:
```bash
docker compose up -d timescaledb
```

### Tests are slow

Use `-short` flag to skip integration tests:
```bash
go test ./... -short
```
