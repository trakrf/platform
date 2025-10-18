# Phase 6A: Justfile Monorepo Structure

**Status**: Planned
**Created**: 2025-10-18
**Depends on**: Phase 6 (Serve Embedded React Frontend)

## Problem Statement

The current justfile structure is flat and doesn't leverage Just's monorepo capabilities:

### Current Pain Points
1. **All recipes in root** - 42 recipes in single 142-line justfile
2. **No workspace context** - Must specify `backend-*` or `frontend-*` from root
3. **Not using Just's path magic** - Can't run `just dev` from `frontend/` directory
4. **Missed discoverability** - Developers in `backend/` don't see backend-specific recipes
5. **Command inconsistency** - Had `backend-run` but no `frontend-dev` (recently added)

### Current Structure
```
platform/
├── justfile                 # 42 recipes (backend + frontend + docker + db)
├── backend/                 # (no justfile)
└── frontend/                # (no justfile)
```

## Desired State

Leverage Just's monorepo patterns for better developer experience:

### Target Structure
```
platform/
├── justfile                 # Shared/orchestration recipes, imports from subdirs
├── backend/justfile         # Backend-specific recipes (import ../justfile)
└── frontend/justfile        # Frontend-specific recipes (import ../justfile)
```

### Developer Experience Goals
1. **Context-aware commands** - `cd backend && just dev` starts Go server
2. **Root orchestration** - `just dev` (from root) starts full stack
3. **Shared recipes** - Import common patterns (lint, test, build)
4. **Better discoverability** - `just --list` shows relevant commands for current directory
5. **Consistent naming** - `just dev` works in any workspace, does the right thing

## Reference Implementation

Following Stuart Ellis's pattern:
https://www.stuartellis.name/articles/just-task-runner/#multiple-justfiles-in-a-directory-structure

### Key Patterns
1. **Subdirectory justfiles** - Each workspace has its own justfile
2. **Import for sharing** - `import '../justfile'` to reuse recipes
3. **Override vs extend** - Local recipes can override or extend root recipes
4. **Automatic discovery** - Just finds nearest justfile in parent directories

## Proposed Recipe Organization

### Root `justfile` (Orchestration)
```just
# Full stack orchestration
dev: db-up
    @just backend-dev &
    @just frontend-dev &
    wait

# Combined validation
validate: lint test build

# Database commands (shared)
db-up:
    docker compose up -d timescaledb

# Import patterns for child justfiles
```

### `backend/justfile` (Backend-specific)
```just
import '../justfile'

# Local dev server (context: already in backend/)
dev:
    go run .

run: dev

# Backend validation
lint:
    go fmt ./...
    go vet ./...

test:
    go test -v ./...

build:
    go build -o bin/trakrf .
```

### `frontend/justfile` (Frontend-specific)
```just
import '../justfile'

# Local dev server (context: already in frontend/)
dev:
    pnpm dev

# Frontend validation
lint:
    pnpm run lint --fix

typecheck:
    pnpm run typecheck

test:
    pnpm test

build:
    pnpm run build
```

## Success Metrics

### Functional Requirements
- [ ] `cd backend && just dev` starts Go server
- [ ] `cd frontend && just dev` starts Vite dev server
- [ ] `just dev` (from root) starts full stack (db + backend + frontend)
- [ ] `cd backend && just --list` shows backend-relevant commands
- [ ] `cd frontend && just --list` shows frontend-relevant commands
- [ ] All existing `just` commands continue to work from root

### Developer Experience
- [ ] Commands are discoverable from workspace directories
- [ ] Consistent naming (`dev`, `lint`, `test`, `build`) across workspaces
- [ ] No workflow regressions (Docker commands, db migrations still work)
- [ ] Documentation updated (README.md, backend/README.md)

### Code Quality
- [ ] No duplication of recipe logic
- [ ] Imports working correctly
- [ ] Comments explain import pattern for future maintainers

## Testing Plan

### Command Matrix
Test each command from multiple contexts:

| Command | From Root | From backend/ | From frontend/ |
|---------|-----------|---------------|----------------|
| `just dev` | Start full stack | Start Go server | Start Vite server |
| `just lint` | Lint all | Lint backend | Lint frontend |
| `just test` | Test all | Test backend | Test frontend |
| `just build` | Build all | Build backend | Build frontend |
| `just --list` | All commands | Backend commands | Frontend commands |

### Docker Commands
- [ ] `just db-up` works from any directory
- [ ] `just db-migrate-up` works from root
- [ ] `just backend-dev` (Docker) works from root

### Backward Compatibility
- [ ] All existing workflows documented in READMEs still work
- [ ] No breaking changes for developers who use root-level commands

## Implementation Approach

### Phase 1: Create Subdirectory Justfiles
1. Create `backend/justfile` with import
2. Create `frontend/justfile` with import
3. Move workspace-specific recipes to subdirectories

### Phase 2: Refactor Root Justfile
1. Keep orchestration recipes (`dev`, `validate`, `db-*`)
2. Add `backend-*` and `frontend-*` aliases that call subdirectory recipes
3. Maintain backward compatibility

### Phase 3: Documentation
1. Update root README.md with new command patterns
2. Update backend/README.md with workspace-specific commands
3. Add comments explaining import pattern

### Phase 4: Testing
1. Test command matrix (root, backend/, frontend/)
2. Verify Docker workflows
3. Verify db migration commands
4. Test on clean checkout

## Files to Modify

### New Files
- `backend/justfile` (~30 lines)
- `frontend/justfile` (~30 lines)

### Modified Files
- `justfile` (refactor, possibly reduce from 142 to ~80 lines)
- `README.md` (update Quick Start with new patterns)
- `backend/README.md` (show workspace-specific commands)
- Possibly `CLAUDE.md` (document justfile pattern for AI)

## Risks and Mitigations

### Risk: Breaking existing workflows
**Mitigation**: Maintain all root-level `backend-*` and `frontend-*` commands as aliases

### Risk: Import pattern confusion
**Mitigation**: Add clear comments and documentation, test from multiple directories

### Risk: Docker commands fail from subdirectories
**Mitigation**: Keep Docker orchestration in root justfile, ensure paths work

### Risk: More files to maintain
**Mitigation**: Better organization offsets maintenance cost, imports reduce duplication

## Out of Scope

- Justfile for `database/` directory (not needed, db commands are orchestration)
- Migration from Just to other task runners
- Advanced Just features (parameters, shell selection, etc.)
- CI/CD integration changes

## Future Enhancements

After Phase 6A stabilizes:
- Add `just watch` commands for auto-reload workflows
- Add `just clean` commands for cleaning build artifacts
- Consider workspace-specific environment variables
- Explore Just modules/functions for shared logic

## References

- **Stuart Ellis - Just Task Runner**: https://www.stuartellis.name/articles/just-task-runner/#multiple-justfiles-in-a-directory-structure
- **Just Manual - Imports**: https://just.systems/man/en/chapter_50.html
- **Current justfile**: `/justfile` (142 lines, 42 recipes)

## Notes

This spec was written during Phase 6 PR review when the gap was identified. Context is fresh:
- Missing `frontend-dev` command discovered and fixed
- Stuart Ellis article reviewed
- Current justfile structure analyzed
- Developer workflow patterns identified

Execute this phase after Phase 6 merges to avoid scope creep and maintain clean PR boundaries.
