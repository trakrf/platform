# Implementation Plan: Monorepo Frontend Baseline - Phase 2 (Integration)

## Complexity: 3/10 (LOW)
- File Impact: 5-10 files to modify, 8 files to delete
- Subsystems: Workspace config + documentation + validation
- Estimated Tasks: 12
- Dependencies: None (all packages already installed)
- Pattern: Following Phase 1 conventions

## Task Breakdown

### 1. Verify Current State
**Files**: Check Phase 1 completion
**Actions**:
- Verify frontend/ directory exists with all 281 files
- Verify pnpm-workspace.yaml exists
- Verify dependencies installed (904 packages)
**Validation**: `ls frontend/package.json` and `pnpm list` succeed
**Time**: 2 minutes

### 2. Update frontend/package.json
**File**: `frontend/package.json`
**Actions**:
- Read current package.json
- Change `"name": "trakrf-handheld"` → `"name": "@trakrf/frontend"`
- Remove `"workspaces"` field entirely
- Keep all other fields (dependencies, scripts, etc.)
**Validation**: Valid JSON, name matches monorepo convention
**Time**: 3 minutes

### 3. Verify Justfile Commands
**File**: `justfile` (read-only check)
**Actions**:
- Read justfile to verify frontend commands exist
- Confirm pattern: `cd frontend && pnpm ...`
- No changes needed (user preference from Phase 1)
**Validation**: Commands use correct pattern
**Time**: 2 minutes

### 4. Run Validation Gate: Lint
**Command**: `just frontend-lint`
**Expected**: ESLint passes with no errors
**On Failure**: Check vite.config.ts, tsconfig.json paths
**Blocking**: YES
**Time**: 2 minutes

### 5. Run Validation Gate: Typecheck
**Command**: `just frontend-typecheck`
**Expected**: TypeScript compiles with no errors
**On Failure**: Check tsconfig.json paths, workspace references
**Blocking**: YES
**Time**: 3 minutes

### 6. Run Validation Gate: Test
**Command**: `just frontend-test`
**Expected**: All unit tests pass
**On Failure**: Check test paths, mocks, workspace dependencies
**Blocking**: YES
**Time**: 5 minutes

### 7. Run Validation Gate: Build
**Command**: `just frontend-build`
**Expected**: Vite build succeeds, dist/ created
**On Failure**: Check vite.config.ts paths, public assets
**Blocking**: YES
**Time**: 3 minutes

### 8. Run Validation Gate: Full Stack
**Command**: `just validate`
**Expected**: Full validation passes
**On Failure**: Review all validation outputs
**Blocking**: YES
**Time**: 3 minutes

### 9. Delete Low-Value Documentation
**Directory**: `docs/frontend/`
**Actions**:
- Delete `ARCHAEOLOGY.md` (historical context)
- Delete `DEPLOYMENT.md` (Railway-specific)
- Delete `OPENREPLAY-TROUBLESHOOTING.md` (specific integration)
- Delete `LOCATE_PACKET_ISSUE.md` (feature-specific)
- Delete `TRIGGER-HANDLING-PLAN.md` (feature-specific)
- Delete `URL-PARAMETER-PATTERN.md` (feature-specific)
- Delete `TEST-COMMANDS.md` (feature-specific)
- Delete `SHIPPING-HISTORY.md` (if exists - standalone history)
**Keep**:
- `ARCHITECTURE.md` (core system architecture)
- `MOCK_USAGE_GUIDE.md` (testing guide)
- `TROUBLESHOOTING.md` (operational guide)
- `cs108/` directory (vendor specs - 6,708 lines)
- `README.md` (documentation index)
- `cs108-hardware-setup.jpg` (hardware reference)
**Validation**: Only essential docs remain
**Time**: 3 minutes

### 10. Update Platform README.md
**File**: `README.md` (root)
**Actions**:
- Read current README.md
- Update frontend/ section from placeholder to real description
- Add link to docs/frontend/ documentation
- Update quick start to include frontend commands
- Reference `just frontend` for frontend validation
**Validation**: README accurately reflects monorepo state
**Time**: 5 minutes

### 11. Update Frontend README.md
**File**: `frontend/README.md`
**Actions**:
- Read current frontend README
- Check if monorepo references make sense
- Update "From project root" section to emphasize Just commands
- Ensure pnpm workspace context is clear
**Validation**: README makes sense in monorepo context
**Time**: 3 minutes

### 12. Final Validation Pass
**Commands**: All validation gates again
**Actions**:
- `just frontend-lint`
- `just frontend-typecheck`
- `just frontend-test`
- `just frontend-build`
- `just validate`
**Expected**: 100% pass rate
**Blocking**: YES
**Time**: 5 minutes

### 13. Create Phase 2 Commit
**Actions**:
- Stage all modified files (package.json, READMEs)
- Stage deleted documentation files
- Create commit with message:
  ```
  feat: integrate frontend into monorepo (phase 2: integration)

  Phase 2 of monorepo frontend baseline migration.

  Integration changes:
  - Update package name to @trakrf/frontend
  - Remove workspace config from frontend/package.json
  - Clean up low-value documentation (8 files)
  - Update platform and frontend READMEs

  Validation:
  - ✅ just frontend-lint passes
  - ✅ just frontend-typecheck passes
  - ✅ just frontend-test passes
  - ✅ just frontend-build passes
  - ✅ just validate passes

  Phase 1: PR #4 (mechanical copy)
  Phase 2: This commit (integration)

  🤖 Generated with [Claude Code](https://claude.com/claude-code)

  Co-Authored-By: Claude <noreply@anthropic.com>
  ```
**Validation**: Commit succeeds
**Time**: 3 minutes

## Total Estimated Time
**45 minutes** (includes validation runs and potential fix iterations)

## Risk Assessment

**Low Risk**:
- Package.json changes are minimal and well-defined
- Documentation deletion is safe (low-value files only)
- Justfile already correct (no changes needed)

**Medium Risk**:
- Validation gates may fail due to path issues
- Mitigation: Spec explicitly says "update only if validation fails"
- If paths need fixing: vite.config.ts, tsconfig.json most likely

**High Risk**:
- None identified

## Success Criteria

**All BLOCKING gates must pass**:
- ✅ `just frontend-lint`
- ✅ `just frontend-typecheck`
- ✅ `just frontend-test`
- ✅ `just frontend-build`
- ✅ `just validate`

**Integration Quality**:
- ✅ Package name: `@trakrf/frontend`
- ✅ No workspace config in frontend/package.json
- ✅ Documentation clean and relevant
- ✅ READMEs reflect monorepo reality

**Overall Success**: 100% validation pass rate + clean integration

## Dependencies
- Phase 1 complete (PR #4 merged)
- Working directory: `/home/mike/platform`
- Branch: Will create `feature/active-monorepo-frontend-baseline-integration`

## Notes
- Unlike Phase 1, this phase MUST pass all validation gates before shipping
- Path fixes are conditional: only if validation fails
- Follow user preference: keep justfile `cd frontend && pnpm` pattern
- Aggressive documentation cleanup: keep only architecture + vendor specs
