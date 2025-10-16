# Feature: Monorepo Frontend Baseline

## Origin
This specification emerged from planning the migration of the standalone `trakrf-handheld` React application into the `platform` monorepo's frontend workspace.

## Outcome
The `trakrf-handheld` application becomes `@trakrf/frontend` - a fully-functional pnpm workspace package within the platform monorepo, with all source code, tests, configurations, and essential documentation preserved and working.

## User Story
As a platform developer
I want the handheld React app integrated as the frontend workspace package
So that I can develop the full stack (Go backend + React frontend) in a unified monorepo with shared tooling and validation

## Context

**Current State**:
- **Standalone**: `trakrf-handheld` is a separate repo (270MB, 178 src files)
  - React + TypeScript + Vite
  - Web Bluetooth + CS108 RFID hardware integration
  - Full test suite: Vitest unit tests + Playwright E2E
  - Comprehensive documentation including vendor specs
  - pnpm package manager with workspace config (but no packages/)

- **Monorepo**: `platform` has placeholder frontend/ and backend/ directories
  - Go + React + TimescaleDB stack
  - Just task runner for validation
  - No pnpm workspace configured yet
  - Using spec-driven workflow (CSW)

**Desired State**:
- `platform/frontend/` contains the complete handheld app as a workspace package
- Root `pnpm-workspace.yaml` manages workspace structure
- Essential docs organized under `docs/frontend/` with CS108 vendor specs preserved
- All tests, dev modes, and validation commands work in monorepo context
- Just task runner orchestrates workspace commands

**Why This Matters**:
- Unified development environment for full stack
- Shared validation and CI/CD workflows
- Preparation for future packages (shared libs, backend tooling)
- Maintains production-ready frontend while building out backend

## Technical Requirements

### 1. Workspace Infrastructure
- **Root pnpm config**: Create `pnpm-workspace.yaml` defining `frontend/` as workspace package
- **Root package.json**: Workspace scripts and shared tooling configuration
- **Package naming**: Frontend package named `@trakrf/frontend` (monorepo convention)

### 2. Source Code Migration
- **Complete copy**: All 178+ files from `trakrf-handheld/src/` → `platform/frontend/src/`
- **Assets**: Copy `public/`, `index.html`, `index.css` to frontend/
- **Preserve structure**: Maintain all subdirectories (components/, stores/, worker/, etc.)

### 3. Configuration Files
Copy all configs to `frontend/`:
- Build: `vite.config.ts`, `vite.config.test.ts`
- TypeScript: `tsconfig.json`, `tsconfig.build.json`, `tsconfig.node.json`, `tsconfig.src-only.json`
- Styling: `tailwind.config.js`, `postcss.config.js`
- Linting: `.eslintrc.json`, `.eslintignore`
- Testing: `playwright.config.ts`, `vitest.config.ts`
- Environment: `.nvmrc`, `.npmrc`, `.envrc`, `.env.local.example`

Update path references in configs to work from `frontend/` subdirectory.

### 4. Test Infrastructure
- **Copy test suite**: `tests/` directory with all unit, integration, and E2E tests
- **Test utilities**: `test-utils/` if exists
- **Scripts**: Copy dev scripts (e.g., `scripts/dev-mock.js`)
- **Preserve test modes**: Hardware tests, mock tests, E2E test tags

### 5. Documentation Cherry-Picking

**Essential CS108 Vendor Docs** (must preserve - irreplaceable):
- `docs/frontend/cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.md` (6,708 lines)
- `docs/frontend/cs108/CS108_and_CS463_Bluetooth_and_USB_Byte_Stream_API_Specifications.pdf`
- `docs/frontend/cs108/Serial_Programming_Command_Manual_V1.3.0-2.md` + `.pdf`
- `docs/frontend/cs108/inventory-parsing.md` (tag parsing quick reference)
- `docs/frontend/cs108/README.md` (protocol overview)
- `docs/frontend/cs108/CS108-PROTOCOL-QUIRKS.md` (protocol gotchas)

**Core Architecture Docs**:
- `docs/frontend/ARCHITECTURE.md` (system architecture - 161 lines)
- `docs/frontend/MOCK_USAGE_GUIDE.md` (BLE mock testing setup - 17KB)
- `docs/frontend/TROUBLESHOOTING.md` (operational guide)
- `docs/frontend/cs108-hardware-setup.jpg` (4MB hardware reference image)

**Updated Index**:
- Create new `docs/frontend/README.md` with correct paths for monorepo
- Update cross-references to work in new structure

**Excluded** (to avoid bloat):
- `ARCHAEOLOGY.md` (historical context)
- `DEPLOYMENT.md` (Railway-specific, may not apply)
- `OPENREPLAY-TROUBLESHOOTING.md` (specific integration)
- Feature-specific docs (LOCATE_PACKET_ISSUE, TRIGGER-HANDLING-PLAN, etc.)

### 6. Just Task Runner Integration
Update `justfile` to use pnpm workspace commands:
- Change `cd frontend && pnpm run lint` → `pnpm -F frontend run lint`
- Ensure all validation commands work from root
- Maintain existing command structure (frontend-lint, frontend-test, etc.)

### 7. Package.json Updates
- Move `handheld/package.json` → `frontend/package.json`
- Change `name` to `@trakrf/frontend`
- Remove `workspaces` field (now at root)
- Keep all dependencies and scripts intact
- Preserve special scripts: `dev:mock`, `dev:https`, `test:hardware`

## Validation Criteria

**Workspace Setup**:
- [ ] `pnpm install` succeeds from root
- [ ] Workspace structure shows `@trakrf/frontend` package
- [ ] Dependencies resolve correctly

**Build & Dev**:
- [ ] `cd frontend && pnpm dev` - dev server starts
- [ ] `cd frontend && pnpm dev:mock` - mock mode works
- [ ] `cd frontend && pnpm build` - build succeeds

**Validation Commands** (via Just):
- [ ] `just frontend-lint` - ESLint passes
- [ ] `just frontend-typecheck` - TypeScript compiles
- [ ] `just frontend-test` - Unit tests pass
- [ ] `just frontend-build` - Build succeeds
- [ ] `just validate` - Full stack validation works

**Tests**:
- [ ] `cd frontend && pnpm test` - Unit tests run
- [ ] `cd frontend && pnpm test:e2e --list` - E2E tests discoverable
- [ ] `cd frontend && pnpm test:hardware` - Hardware test runs (with hardware)

**Documentation**:
- [ ] CS108 vendor specs accessible at `docs/frontend/cs108/`
- [ ] Cross-references in docs resolve correctly
- [ ] README links to frontend docs

## Migration Strategy

### Phase 1: Infrastructure
1. Create root `pnpm-workspace.yaml`
2. Create root `package.json` with workspace config
3. Update `justfile` for workspace commands

### Phase 2: Code Migration
4. Copy all source code to `frontend/src/`
5. Copy all config files to `frontend/`
6. Copy test infrastructure to `frontend/tests/`
7. Update `frontend/package.json`

### Phase 3: Documentation
8. Create `docs/frontend/` structure
9. Copy CS108 vendor specs to `docs/frontend/cs108/`
10. Copy architecture docs to `docs/frontend/`
11. Create updated `docs/frontend/README.md`
12. Update root documentation

### Phase 4: Path Fixes
13. Update vite.config.ts paths
14. Update tsconfig paths if needed
15. Update playwright paths
16. Fix doc cross-references

### Phase 5: Validation
17. Install dependencies: `pnpm install`
18. Run full validation: `just validate`
19. Test dev modes: `pnpm dev`, `pnpm dev:mock`
20. Verify E2E tests: `pnpm test:e2e --list`

### Phase 6: Cleanup
21. Remove handheld-specific references
22. Update naming to `@trakrf/frontend`
23. Clean build artifacts

### Phase 7: Commit
24. Create migration commit: `feat: baseline frontend from trakrf-handheld`
25. Reference original repo in commit body

## Key Decisions

**Documentation Strategy**:
- Cherry-pick essential docs vs full migration (chose cherry-pick)
- Rationale: Vendor specs are irreplaceable and frequently referenced; skip ephemeral docs

**Workspace Structure**:
- Organize under `docs/frontend/` vs flat structure (chose frontend/ subdirectory)
- Rationale: Keeps frontend-specific docs separate, room for backend docs later

**Git History**:
- Document migration in commit message
- Reference original repo for historical context
- Option to use git subtree if full history needed later

**Task Runner**:
- Update Just to use pnpm workspace filters
- Maintains existing command names for consistency
- Enables root-level orchestration

## Conversation References

**Key Insight**: "That CS108 vendor spec (6,708 lines) is something I frequently reference - it's a PDF-to-MD conversion we can't regenerate"

**Decision**: Cherry-pick docs rather than migrate everything - preserve vendor specs and core architecture docs, skip deployment/feature-specific docs

**Context**: The monorepo uses Just for task orchestration and follows a spec-driven workflow (CSW) with pnpm as the exclusive package manager

## Estimated Scope
- **Files to migrate**: ~180 source files + ~10 config files + ~10 doc files
- **Documentation**: ~7,000 lines of vendor specs + architecture docs
- **Risk areas**: Path resolution, workspace deps, test configs
- **Success metric**: All existing tests pass in new location
