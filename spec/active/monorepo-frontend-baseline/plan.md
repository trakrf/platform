# Implementation Plan: Monorepo Frontend Baseline
Generated: 2025-10-16
Specification: spec.md

## Understanding

This plan migrates the standalone `trakrf-handheld` React application into the `platform` monorepo as `@trakrf/frontend`. The migration is split into two phases:

**Phase 1 (This Plan)**: Mechanical copy of all files with zero modifications
- Creates pnpm workspace infrastructure
- Copies all 200+ files verbatim from trakrf-handheld
- Zero changes to existing platform files
- Result: Files in place but not yet integrated

**Phase 2 (Separate PR)**: Integration and path fixes
- Updates configs for monorepo paths
- Updates justfile for workspace commands
- Fixes package.json naming
- Cleans up low-value docs
- Updates platform README

**Why This Split Works**:
- Phase 1 PR is additive-only → no need to review 200 file diffs
- Can verify with simple: `diff -r ../trakrf-handheld/src frontend/src`
- Phase 2 is small (~10 files) and reviewable
- If integration has issues, Phase 1 is already safely merged

## Phase 1 Scope

**This plan implements Phase 1 only**: Mechanical file copy with workspace setup.

## Relevant Files

**Reference Patterns**:
- `/home/mike/trakrf-handheld/pnpm-workspace.yaml` - Existing workspace config to reference
- `/home/mike/trakrf-handheld/package.json` - Source package.json structure

**Files to Create (Phase 1)**:
- `pnpm-workspace.yaml` - Root workspace configuration
- `package.json` - Root package.json (minimal)
- `frontend/` - Complete copy of trakrf-handheld structure
- `docs/frontend/` - Complete copy of docs/

**Files to Copy (Phase 1)**:

From `/home/mike/trakrf-handheld/` to `platform/frontend/`:
- Source: `src/` → `frontend/src/` (178 files)
- Tests: `tests/` → `frontend/tests/`
- Test utils: `test-utils/` → `frontend/test-utils/`
- Scripts: `scripts/` → `frontend/scripts/`
- Public: `public/` → `frontend/public/`
- Root files: `index.html`, `index.css` → `frontend/`
- Configs: `vite.config.ts`, `vite.config.test.ts`, `tsconfig*.json`, etc. → `frontend/`
- Styling: `tailwind.config.js`, `postcss.config.js` → `frontend/`
- Linting: `.eslintrc.json`, `.eslintignore` → `frontend/`
- Testing: `playwright.config.ts`, `vitest.config.ts` → `frontend/`
- Environment: `.nvmrc`, `.npmrc`, `.envrc`, `.env.local.example` → `frontend/`
- Package: `package.json`, `pnpm-lock.yaml` → `frontend/`
- Docs: `CLAUDE.md`, `README.md` → `frontend/`

From `/home/mike/trakrf-handheld/docs/` to `platform/docs/frontend/`:
- All 18 documentation files (4.1MB total)

**NOT Copied**:
- `node_modules/` - Will be installed fresh
- `dist/` - Build artifacts
- `.git/` - Git history stays in original repo
- `test-results/`, `tmp/` - Temporary files
- `examples/`, `prp/`, `spec/` - Project-specific, not needed in monorepo

**Files to Modify (Phase 2 - NOT THIS PLAN)**:
- `justfile` - Update for workspace commands
- `frontend/package.json` - Change name to `@trakrf/frontend`
- `frontend/vite.config.ts` - Path updates if needed
- `platform/README.md` - Reflect real frontend
- Various doc cross-references

## Architecture Impact

**Subsystems Affected**:
1. **Workspace Infrastructure**: Creating pnpm workspace
2. **Documentation Organization**: New `docs/frontend/` structure
3. **Frontend Codebase**: Location change (not functional change)

**New Dependencies**: None (workspace infrastructure only)

**Breaking Changes**: None (Phase 1 is additive only)

## Task Breakdown

### Task 1: Create Root Workspace Configuration
**File**: `pnpm-workspace.yaml`
**Action**: CREATE

**Implementation**:
Create minimal workspace config referencing frontend package:

```yaml
packages:
  - 'frontend'
```

**Validation**:
- File exists at root
- Valid YAML syntax

### Task 2: Create Root Package.json
**File**: `package.json`
**Action**: CREATE

**Implementation**:
Create minimal root package.json for workspace:

```json
{
  "name": "trakrf-platform",
  "version": "0.1.0",
  "private": true,
  "description": "TrakRF Platform - RFID/BLE asset tracking",
  "repository": {
    "type": "git",
    "url": "https://github.com/trakrf/platform"
  },
  "engines": {
    "node": ">=24.0.0",
    "pnpm": ">=9.0.0"
  },
  "packageManager": "pnpm@9.12.3"
}
```

**Validation**:
- File exists at root
- Valid JSON syntax
- No scripts (keeping minimal per clarifying question)

### Task 3: Copy Source Code
**Directory**: `frontend/src/`
**Action**: CREATE (copy from /home/mike/trakrf-handheld/src/)

**Implementation**:
```bash
cp -r /home/mike/trakrf-handheld/src frontend/src
```

Copy entire src/ directory verbatim. No modifications.

**Validation**:
- Directory exists: `frontend/src/`
- File count matches source (178 files)
- Spot check: `diff -r /home/mike/trakrf-handheld/src frontend/src` shows no differences

### Task 4: Copy Test Infrastructure
**Directories**: `frontend/tests/`, `frontend/test-utils/`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp -r /home/mike/trakrf-handheld/tests frontend/tests
cp -r /home/mike/trakrf-handheld/test-utils frontend/test-utils
```

Copy entire test directories verbatim.

**Validation**:
- Directories exist: `frontend/tests/`, `frontend/test-utils/`
- Spot check: `diff -r` shows no differences

### Task 5: Copy Scripts
**Directory**: `frontend/scripts/`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp -r /home/mike/trakrf-handheld/scripts frontend/scripts
```

Copy dev scripts including `dev-mock.js`.

**Validation**:
- Directory exists: `frontend/scripts/`
- Files present: `dev-mock.js`, `setup-https.sh`, etc.

### Task 6: Copy Public Assets
**Directory**: `frontend/public/`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp -r /home/mike/trakrf-handheld/public frontend/public
```

Copy public assets verbatim.

**Validation**:
- Directory exists: `frontend/public/`
- Assets present

### Task 7: Copy Root HTML and CSS
**Files**: `frontend/index.html`, `frontend/index.css`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/index.html frontend/index.html
cp /home/mike/trakrf-handheld/index.css frontend/index.css
```

**Validation**:
- Files exist
- Identical to source

### Task 8: Copy Build Configuration
**Files**: `frontend/vite.config.ts`, `frontend/vite.config.test.ts`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/vite.config.ts frontend/vite.config.ts
cp /home/mike/trakrf-handheld/vite.config.test.ts frontend/vite.config.test.ts
```

Copy verbatim. Path updates happen in Phase 2.

**Validation**:
- Files exist
- Identical to source

### Task 9: Copy TypeScript Configuration
**Files**: `frontend/tsconfig.json`, `frontend/tsconfig.build.json`, `frontend/tsconfig.node.json`, `frontend/tsconfig.src-only.json`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/tsconfig.json frontend/tsconfig.json
cp /home/mike/trakrf-handheld/tsconfig.build.json frontend/tsconfig.build.json
cp /home/mike/trakrf-handheld/tsconfig.node.json frontend/tsconfig.node.json
cp /home/mike/trakrf-handheld/tsconfig.src-only.json frontend/tsconfig.src-only.json
```

**Validation**:
- All 4 tsconfig files exist
- Identical to source

### Task 10: Copy Styling Configuration
**Files**: `frontend/tailwind.config.js`, `frontend/postcss.config.js`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/tailwind.config.js frontend/tailwind.config.js
cp /home/mike/trakrf-handheld/postcss.config.js frontend/postcss.config.js
```

**Validation**:
- Files exist
- Identical to source

### Task 11: Copy Linting Configuration
**Files**: `frontend/.eslintrc.json`, `frontend/.eslintignore`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/.eslintrc.json frontend/.eslintrc.json
cp /home/mike/trakrf-handheld/.eslintignore frontend/.eslintignore
```

**Validation**:
- Files exist
- Identical to source

### Task 12: Copy Testing Configuration
**Files**: `frontend/playwright.config.ts`, `frontend/vitest.config.ts`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/playwright.config.ts frontend/playwright.config.ts
cp /home/mike/trakrf-handheld/vitest.config.ts frontend/vitest.config.ts
```

**Validation**:
- Files exist
- Identical to source

### Task 13: Copy Environment Configuration
**Files**: `frontend/.nvmrc`, `frontend/.npmrc`, `frontend/.envrc`, `frontend/.env.local.example`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/.nvmrc frontend/.nvmrc
cp /home/mike/trakrf-handheld/.npmrc frontend/.npmrc
cp /home/mike/trakrf-handheld/.envrc frontend/.envrc
cp /home/mike/trakrf-handheld/.env.local.example frontend/.env.local.example
```

**Validation**:
- Files exist
- Identical to source

### Task 14: Copy Package Files
**Files**: `frontend/package.json`, `frontend/pnpm-lock.yaml`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/package.json frontend/package.json
cp /home/mike/trakrf-handheld/pnpm-lock.yaml frontend/pnpm-lock.yaml
```

Copy verbatim. Name change happens in Phase 2.

**Validation**:
- Files exist
- Identical to source

### Task 15: Copy Frontend Documentation
**Files**: `frontend/CLAUDE.md`, `frontend/README.md`
**Action**: CREATE (copy from trakrf-handheld)

**Implementation**:
```bash
cp /home/mike/trakrf-handheld/CLAUDE.md frontend/CLAUDE.md
cp /home/mike/trakrf-handheld/README.md frontend/README.md
```

**Validation**:
- Files exist
- Identical to source

### Task 16: Copy All Documentation
**Directory**: `docs/frontend/`
**Action**: CREATE (copy from trakrf-handheld/docs/)

**Implementation**:
```bash
cp -r /home/mike/trakrf-handheld/docs docs/frontend
```

Copy entire docs directory (18 files, 4.1MB). Cleanup happens in Phase 2.

**Validation**:
- Directory exists: `docs/frontend/`
- All files present (including cs108/ subdirectory)
- Spot check: `diff -r /home/mike/trakrf-handheld/docs docs/frontend` shows no differences

### Task 17: Verify Workspace Installation
**Action**: Test workspace setup

**Implementation**:
```bash
pnpm install
```

Run from project root. This validates workspace config and installs all dependencies.

**Expected Behavior**:
- pnpm recognizes workspace
- Installs frontend dependencies
- No errors about workspace structure

**Validation**:
Use validation commands from `spec/stack.md`:
- Installation completes successfully
- `node_modules/` created in both root and frontend/
- Workspace linking works

### Task 18: Create Phase 1 Commit
**Action**: Commit all copied files

**Implementation**:
```bash
git add pnpm-workspace.yaml package.json frontend/ docs/frontend/
git commit -m "feat: baseline frontend from trakrf-handheld (phase 1: mechanical copy)

Copies all files verbatim from trakrf-handheld standalone repo:
- 178 source files (src/)
- Full test suite (tests/, test-utils/)
- All configurations (vite, typescript, eslint, etc.)
- Complete documentation (18 files, 4.1MB)
- Scripts and assets

NO modifications made - files copied exactly as-is.
Integration and path fixes will happen in Phase 2.

Source: https://github.com/trakrf/trakrf-handheld
Ref: trakrf-handheld@$(cd /home/mike/trakrf-handheld && git rev-parse --short HEAD)"
```

**Validation**:
- All files staged
- Commit message follows convention
- No changes to existing platform files (except additions)

## Risk Assessment

**Phase 1 Risks** (Low):
- **Risk**: Workspace config syntax error
  **Mitigation**: Simple YAML/JSON, validate with `pnpm install`

- **Risk**: Missing files during copy
  **Mitigation**: Use `diff -r` to verify completeness

- **Risk**: File permissions lost
  **Mitigation**: Use `cp -r` which preserves permissions

**Phase 2 Risks** (Deferred):
- Path resolution issues → Will be addressed in Phase 2
- Justfile syntax → Will be addressed in Phase 2
- Package name conflicts → Will be addressed in Phase 2

## Integration Points

**Phase 1** (This Plan):
- Workspace setup: pnpm recognizes frontend package
- No other integration (intentional)

**Phase 2** (Future):
- Just task runner: Update commands for workspace
- Path references: Fix configs for monorepo structure
- Documentation: Update cross-references and clean up low-value docs
- Platform README: Reflect real frontend

## VALIDATION GATES (MANDATORY)

**Phase 1 Gates**:

After workspace setup (Tasks 1-2):
```bash
# Validate workspace config
pnpm install
```
**Must pass**: Installation succeeds, workspace recognized

After file copy (Tasks 3-16):
```bash
# Verify file completeness
diff -r /home/mike/trakrf-handheld/src frontend/src
diff -r /home/mike/trakrf-handheld/docs docs/frontend
```
**Must pass**: No differences found

After final installation (Task 17):
```bash
pnpm install
```
**Must pass**: Dependencies install without errors

**Note**: We are NOT running lint/typecheck/test/build in Phase 1. Those validations happen in Phase 2 when paths are fixed.

## Validation Sequence

**After Tasks 1-2** (Workspace setup):
```bash
pnpm install
```
Expected: Workspace structure recognized, no errors

**After Tasks 3-16** (File copy):
```bash
# Spot checks
ls -la frontend/src
ls -la docs/frontend
diff -r /home/mike/trakrf-handheld/src frontend/src | head -20
```
Expected: Directories exist, no differences

**After Task 17** (Final installation):
```bash
pnpm install
```
Expected: All dependencies installed successfully

**No lint/typecheck/test/build in Phase 1** - Those are Phase 2 validations.

## Plan Quality Assessment

**Complexity Score**: 2/10 (LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Mechanical copy - no logic, minimal risk
✅ Clear source directory structure at `/home/mike/trakrf-handheld/`
✅ All clarifying questions answered
✅ Simple validation: `diff -r` proves correctness
✅ Zero modifications to existing platform code
✅ Phase split reduces scope significantly
✅ pnpm workspace pattern is well-established

⚠️ Minor uncertainty: Exact file count (will verify during execution)

**Assessment**: Very high confidence. This is a straightforward mechanical copy operation with clear validation criteria. The workspace setup follows standard pnpm patterns. Phase split eliminates integration complexity from this phase.

**Estimated one-pass success probability**: 95%

**Reasoning**: Mechanical copy operations are low-risk. The only potential issues are missing files or workspace config syntax, both easily caught by validation gates. High confidence because no logic or integration complexity in this phase.

## Phase 2 Preview

**Not in this plan**, but for context:

Phase 2 will handle:
1. Update `justfile` for workspace commands (keep `cd frontend &&` pattern)
2. Update `frontend/package.json` name to `@trakrf/frontend`
3. Remove `workspaces` field from `frontend/package.json`
4. Fix any path references in configs if needed
5. Delete low-value docs (ARCHAEOLOGY.md, DEPLOYMENT.md, etc.)
6. Update `platform/README.md` to reflect real frontend
7. Run full validation suite: `just validate`
8. Commit: `feat: baseline frontend from trakrf-handheld (phase 2: integration)`

Phase 2 complexity: 4/10 (manageable, ~10 files to modify)
