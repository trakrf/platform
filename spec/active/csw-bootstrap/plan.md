# Implementation Plan: Bootstrap Claude Spec Workflow (CSW)
Generated: 2025-10-12
Specification: spec.md

## Understanding

Set up CSW for a Go + React + TimescaleDB monorepo that will use Just task runner. Currently the project is documentation-only, so this bootstraps the minimal directory structure and validation framework for future development.

**User Decisions**:
- Create backend/frontend directories with minimal scaffolding
- Document intended validation commands (will validate when code exists)
- Use Just task runner for validation orchestration
- Keep Go commands minimal (basic test/build)
- Document workspace-level commands (cd into workspace) + Just commands from root

## Relevant Files

**Files to Create**:
- `backend/README.md` - Backend workspace marker and setup docs
- `frontend/README.md` - Frontend workspace marker and setup docs
- `justfile` - Task runner commands for validation
- Update `spec/stack.md` - Monorepo validation strategy

**Reference Documentation**:
- Just task runner: https://just.systems/
- Project structure from README.md
- Git workflow from CLAUDE.md

## Architecture Impact
- **Subsystems affected**: Project structure (creates workspace directories)
- **New dependencies**: Just task runner (external tool, not a package dependency)
- **Breaking changes**: None (new project)

## Task Breakdown

### Task 1: Create Backend Directory Structure
**Files**: `backend/README.md`
**Action**: CREATE

**Implementation**:
Create `backend/` directory with README documenting:
- Go workspace for API server
- Intended structure (cmd/, internal/, pkg/)
- Setup instructions (placeholder for now)
- Validation commands

**Validation**:
```bash
# Verify directory and file exist
test -d backend && test -f backend/README.md
```

### Task 2: Create Frontend Directory Structure
**Files**: `frontend/README.md`
**Action**: CREATE

**Implementation**:
Create `frontend/` directory with README documenting:
- React workspace for web app
- Package manager: pnpm (EXCLUSIVE - per CLAUDE.md)
- Intended structure (src/, public/, tests/)
- Setup instructions (placeholder for now)
- Validation commands

**Validation**:
```bash
# Verify directory and file exist
test -d frontend && test -f frontend/README.md
```

### Task 3: Create Just Task Runner Config
**Files**: `justfile`
**Action**: CREATE

**Implementation**:
Create justfile at project root with validation recipes:
```just
# Validate backend
backend-lint:
    cd backend && go fmt ./...
    cd backend && go vet ./...

backend-test:
    cd backend && go test ./...

backend-build:
    cd backend && go build ./...

backend: backend-lint backend-test backend-build

# Validate frontend
frontend-lint:
    cd frontend && pnpm run lint --fix

frontend-typecheck:
    cd frontend && pnpm run typecheck

frontend-test:
    cd frontend && pnpm test

frontend-build:
    cd frontend && pnpm run build

frontend: frontend-lint frontend-typecheck frontend-test frontend-build

# Validate all
lint: backend-lint frontend-lint

test: backend-test frontend-test

build: backend-build frontend-build

validate: lint test build

# CSW /check command uses this
check: validate
```

**Validation**:
```bash
# Verify justfile syntax
just --list
```

### Task 4: Update spec/stack.md for Monorepo
**Files**: `spec/stack.md`
**Action**: MODIFY

**Implementation**:
Replace entire contents with monorepo structure:

```markdown
# Stack: Go + React + TimescaleDB (Monorepo)

> **Package Manager**: pnpm (frontend only)
> **Task Runner**: Just (https://just.systems/)
> **Backend**: Go 1.21+
> **Frontend**: React + TypeScript + Vite
> **Database**: TimescaleDB (PostgreSQL extension)

## Quick Validation

From project root:
```bash
just validate
```

## Backend (Go)

### From backend/ directory:
```bash
# Lint
go fmt ./...
go vet ./...

# Test
go test ./...

# Build
go build ./...
```

### From project root (via Just):
```bash
just backend-lint
just backend-test
just backend-build
just backend  # All backend checks
```

## Frontend (React + TypeScript)

**IMPORTANT**: This project uses pnpm EXCLUSIVELY. Never use npm or npx.

### From frontend/ directory:
```bash
# Lint
pnpm run lint --fix

# Typecheck
pnpm run typecheck

# Test
pnpm test

# Build
pnpm run build
```

### From project root (via Just):
```bash
just frontend-lint
just frontend-typecheck
just frontend-test
just frontend-build
just frontend  # All frontend checks
```

## Full Stack Validation

From project root:
```bash
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend
just validate    # All checks (used by /check)
```

## CSW Integration

The `/check` command runs:
```bash
just check
```

This validates the entire stack is ready to ship.

## Database

TimescaleDB validation happens via backend tests. No separate validation commands needed.
```

**Validation**:
```bash
# Verify file is valid markdown and readable
test -f spec/stack.md && wc -l spec/stack.md
```

### Task 5: Document Just in README.md
**Files**: `README.md`
**Action**: MODIFY

**Implementation**:
Add Just to Prerequisites section (after line 31):
```markdown
### Prerequisites
- Go 1.21+
- Node.js 18+
- Docker & Docker Compose
- TimescaleDB (via Docker or TigerData cloud)
- Just (command runner) - https://just.systems/
```

Also add a new section after Quick Start explaining validation:
```markdown
### Validation

Run validation checks:
```bash
# Full validation (lint, test, build)
just validate

# Individual checks
just lint        # Lint backend + frontend
just test        # Test backend + frontend
just build       # Build backend + frontend
```

See `justfile` for all available commands.
```

**Validation**:
```bash
# Verify Just is documented
grep -q "Just" README.md && grep -q "just validate" README.md
```

### Task 6: Verify Directory Structure
**Action**: VERIFY

**Implementation**:
Confirm the directory structure matches CSW expectations:
```
platform/
├── backend/          ✓ (new)
│   └── README.md     ✓ (new)
├── frontend/         ✓ (new)
│   └── README.md     ✓ (new)
├── spec/
│   ├── README.md     ✓ (existing)
│   ├── template.md   ✓ (existing)
│   ├── stack.md      ✓ (updated)
│   ├── SHIPPED.md    ✓ (existing)
│   └── active/       ✓ (existing)
│       └── csw-bootstrap/
│           ├── spec.md   ✓
│           └── plan.md   ✓
├── justfile          ✓ (new)
└── README.md         ✓ (updated)
```

**Validation**:
```bash
# Verify all expected files/dirs exist
test -d backend && \
test -d frontend && \
test -d spec/active && \
test -f spec/stack.md && \
test -f justfile && \
test -f README.md && \
grep -q "Just" README.md && \
echo "✓ Structure verified"
```

## Risk Assessment

**Risk**: Just task runner not installed
**Mitigation**: Document installation in README.md, provide fallback commands in stack.md

**Risk**: Validation commands will fail until actual code exists
**Mitigation**: This is expected - stack.md documents *intended* commands. Will validate once code is written.

**Risk**: Go/pnpm commands might need adjustment for specific project setup
**Mitigation**: Keeping commands generic and minimal. Easy to extend later.

## Integration Points
- **CSW /check command**: Will run `just check` (via stack.md)
- **Git workflow**: Respects CLAUDE.md rules (feature branches, no push to main)
- **Documentation**: README.md already references backend/frontend structure

## VALIDATION GATES (MANDATORY)

Since this is infrastructure setup (no actual code), validation is:

After EVERY file creation:
```bash
# Gate 1: File exists and is readable
test -f {filepath} && cat {filepath} > /dev/null

# Gate 2: Syntax check (where applicable)
# - Markdown: No check needed
# - justfile: just --list
```

**Enforcement Rules**:
- If file creation fails → Fix immediately
- If syntax check fails → Fix and re-check
- All files must be readable and syntactically valid

## Validation Sequence

After each task:
```bash
# Verify file/directory created
ls -la {path}

# For justfile specifically:
just --list
```

Final validation:
```bash
# Verify complete structure
ls -la backend/ frontend/ spec/active/ justfile

# Verify justfile is valid
just --list

# Verify stack.md updated
grep "Go + React" spec/stack.md
```

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
- Creating 4 files (2 READMEs, 1 justfile), updating 2 files (stack.md, README.md)
- No code logic, just structure and documentation
- Clear, simple tasks

**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec
✅ No code dependencies - pure structure
✅ User provided clear answers to all questions
✅ Just task runner is well-documented
✅ Simple file creation and documentation tasks

**Assessment**: High confidence - straightforward infrastructure setup with clear requirements and no code dependencies.

**Estimated one-pass success probability**: 95%

**Reasoning**: This is primarily documentation and directory structure with one task runner config. The only potential issue is if Just isn't installed, but that's documented. No complex logic or integration points to fail.
