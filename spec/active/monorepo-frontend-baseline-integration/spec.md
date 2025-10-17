# Feature: Monorepo Frontend Baseline - Phase 2 (Integration)

## Origin
Phase 2 of the monorepo frontend baseline migration. Phase 1 (PR #4) copied all files verbatim. This phase integrates them into the monorepo.

## Outcome
The frontend is fully integrated and functional in the monorepo with all validation gates passing.

## User Story
As a platform developer
I want the frontend fully integrated into the monorepo
So that I can develop and validate the full stack with `just validate`

## Context

**Phase 1 Complete** (PR #4, merged):
- ✅ pnpm workspace infrastructure created
- ✅ 281 files copied verbatim (178 source + configs + tests + docs)
- ✅ Workspace validated, dependencies installed (904 packages)
- ✅ Files verified identical to source

**Phase 2 Scope** (This Spec):
- Integrate files with monorepo structure
- Update configurations for workspace
- Run full validation suite
- Clean up documentation

**Why Phase 2**:
- Phase 1 was mechanical copy with zero modifications (easy merge)
- Phase 2 makes it actually work (small, reviewable changes)

## Technical Requirements

### 1. Update justfile for Workspace Commands
**Current**: Justfile expects to `cd frontend && pnpm ...`
**Action**: Verify commands work, update if needed

### 2. Update frontend/package.json
**Changes**:
- Update `name` field from `trakrf-handheld` → `@trakrf/frontend`
- Remove `workspaces` field (now at root)
- Keep all other fields (dependencies, scripts, etc.)

### 3. Fix Path References (If Needed)
**Check**: vite.config.ts, tsconfig.json paths
**Action**: Update only if validation fails

### 4. Clean Up Low-Value Documentation
**Delete from docs/frontend/**:
- `ARCHAEOLOGY.md` - Historical context (not needed)
- `DEPLOYMENT.md` - Railway-specific (doesn't apply to monorepo)
- `OPENREPLAY-TROUBLESHOOTING.md` - Specific integration (optional)
- Feature-specific: `LOCATE_PACKET_ISSUE.md`, `TRIGGER-HANDLING-PLAN.md`, `URL-PARAMETER-PATTERN.md`, `TEST-COMMANDS.md`

**Keep**:
- `ARCHITECTURE.md` - Core system architecture
- `MOCK_USAGE_GUIDE.md` - Testing guide
- `TROUBLESHOOTING.md` - Operational guide
- `cs108/` directory - All vendor specs and protocol docs
- `README.md` - Documentation index
- `cs108-hardware-setup.jpg` - Hardware reference

### 5. Update Platform README.md
**Changes**:
- Update frontend/ section to reflect real frontend (not placeholder)
- Link to frontend docs
- Update quick start instructions

### 6. Update Frontend README.md
**Check**: Does it make sense in monorepo context?
**Action**: Update references to reflect monorepo structure if needed

## Validation Criteria

**BLOCKING GATES** (Must Pass):
- [ ] `just frontend-lint` - ESLint passes
- [ ] `just frontend-typecheck` - TypeScript compiles with no errors
- [ ] `just frontend-test` - All unit tests pass
- [ ] `just frontend-build` - Vite build succeeds
- [ ] `just validate` - Full stack validation works

**Documentation**:
- [ ] Platform README.md reflects real frontend
- [ ] Frontend README.md makes sense in monorepo
- [ ] Low-value docs deleted
- [ ] Essential docs (vendor specs, architecture) preserved

**Integration**:
- [ ] Workspace package name is `@trakrf/frontend`
- [ ] No workspace config in frontend/package.json
- [ ] justfile commands work from root

## Success Metrics

**Validation Gates**:
- All 5 validation commands pass (lint, typecheck, test, build, validate)

**Integration Quality**:
- justfile commands work without errors
- Package naming follows monorepo conventions
- Documentation is clean and relevant

**Overall Success**: 100% validation gates + clean integration

## Migration Strategy

1. Update frontend/package.json (name, remove workspaces)
2. Verify justfile commands work
3. Run validation gates (fix any issues)
4. Clean up documentation
5. Update platform README.md
6. Final validation pass
7. Commit and create PR

## Key Decisions

**Keep justfile pattern**: Use `cd frontend && pnpm ...` (user preference from Phase 1)

**Delete docs aggressively**: Keep only essential architecture and vendor specs

**Full validation required**: Unlike Phase 1, this phase must pass all validation gates

## Phase 1 Reference

- **PR**: https://github.com/trakrf/platform/pull/4
- **Commit**: 05051c6
- **Source**: trakrf-handheld@cf584e4
- **SHIPPED**: 2025-10-17

## Estimated Scope

- **Files to modify**: ~5-10 files
- **Files to delete**: ~8 documentation files
- **Validation commands**: 5 (lint, typecheck, test, build, validate)
- **Complexity**: 4/10 (straightforward integration)
