# CLAUDE.md

This file provides guidance to Claude when working with code in this repository.

## 📄 Project Awareness & Context
- **Always read `PLANNING.md`** at the start of a new conversation to understand the project's architecture, goals, and constraints
- **Use consistent patterns** across the entire stack (Go backend, React frontend, TimescaleDB)

## ⚠️ MANDATORY: Package Manager Rules

**Backend**: Use standard Go tooling (`go mod`, `go get`)
**Frontend**: Use pnpm EXCLUSIVELY
- Replace ALL instances of `npm` with `pnpm`
- Replace ALL instances of `npx` with `pnpm dlx`

## 🚨 CRITICAL: Git Workflow Rules

**NEVER PUSH DIRECTLY TO MAIN BRANCH**
1. **ALL changes must go through a Pull Request** - no exceptions
2. **Always create a feature/fix branch** for your work
3. **Use conventional commits**: `feat:`, `fix:`, `docs:`, `chore:`
4. **Branch naming**: `feature/add-xyz`, `fix/broken-xyz`, `docs/update-xyz`

## Architecture Principles

### Core Philosophy
- **Clean Architecture** - Separate API, business logic, and data layers
- **Type Safety** - TypeScript frontend, strongly-typed Go backend
- **Real-time First** - Design for streaming data and live updates
- **Multi-tenant** - Always consider data isolation
- **Constants Over Magic Numbers** - Use enums and named constants

### Code Structure
- **Never create files longer than 500 lines** - Split into modules
- **Organize by feature/domain** not by file type
- **Clear module boundaries** with explicit interfaces

### Backend (Go)
```go
// Use clear service interfaces
type AssetService interface {
    GetLocation(ctx context.Context, assetID string) (*Location, error)
}

// Proper error handling
if err := validate(input); err != nil {
    return fmt.Errorf("validation failed: %w", err)
}

// Context-aware operations
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
```

### Frontend (React/TypeScript)
```typescript
// Type all function parameters and returns
function processAsset(data: AssetData): ProcessedAsset {
  // implementation
}

// Use type imports
import type { AssetLocation } from '@/types';

// Prefer interfaces for shapes
interface DeviceConfig {
  name: string;
  timeout: number;
}
```

### Database (TimescaleDB)
```sql
-- Use meaningful table/column names
-- Always include created_at, updated_at
-- Consider hypertables for time-series data
-- Use proper indexes for tenant isolation
```

## 🧪 Testing Philosophy

### Test Structure
- **Colocate unit tests** with source files (`file.go` → `file_test.go`)
- **Integration tests** in `tests/integration/`
- **E2E tests** in `tests/e2e/`

### Test Requirements
- **Always write tests** for new features
- **Test actual behavior**, not implementation details
- **Mock only external dependencies** (databases, APIs, hardware)
- **Report exact test results**: "X passing, Y failing"

### Backend Testing
```bash
go test ./...              # Unit tests
go test -race ./...        # Race detection
go test ./tests/integration -tags=integration
```

### Frontend Testing
```bash
pnpm test                  # Unit tests
pnpm test:e2e             # E2E tests (headless only)
pnpm validate             # Full validation
```

## Critical Implementation Rules

### ✅ ALWAYS:
1. Use dependency injection for testability
2. Handle errors explicitly at boundaries
3. Use structured logging with context
4. Validate input at API boundaries
5. Use transactions for data consistency
6. Consider rate limiting and backpressure

### 🚫 NEVER:
1. Hardcode credentials or secrets
2. Ignore error returns
3. Use `panic()` except in truly unrecoverable cases
4. Mix business logic with HTTP handlers
5. Store sensitive data in logs
6. Trust client-provided IDs without validation

## API Design

- **RESTful conventions** with clear resource names
- **Consistent error responses** with problem details
- **API versioning** via URL path (`/api/v1/`)
- **Authentication** via JWT in Authorization header
- **Multi-tenancy** via header or subdomain

## Documentation Standards

- **API docs** via OpenAPI/Swagger
- **Code comments** explain "why" not "what"
- **README** includes setup and common tasks
- **Architecture decisions** documented in ADRs

## 🧠 AI Behavior Rules

- **Ask questions** when requirements are unclear
- **Never delete code** without explicit instruction
- **Run tests** before claiming completion
- **Report actual status** - no false optimism
- **Check file existence** before referencing
- **Use only verified packages** - no hallucinated imports

## Performance Considerations

### Backend
- Use connection pooling
- Implement caching strategically
- Profile with pprof
- Use bulk operations for batch processing

### Frontend
- Debounce rapid updates
- Virtualize long lists
- Use React.memo judiciously
- Lazy load heavy components

### Database
- Use TimescaleDB continuous aggregates
- Implement data retention policies
- Index foreign keys and query predicates
- Monitor slow queries

## Security First

- **Input validation** on all external data
- **SQL injection prevention** via parameterized queries
- **XSS protection** in React (automatic with JSX)
- **CORS configuration** explicit and minimal
- **Rate limiting** on all endpoints
- **Audit logging** for sensitive operations
