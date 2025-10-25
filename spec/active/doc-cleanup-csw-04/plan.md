# Implementation Plan: CSW 0.4.0 Documentation Cleanup
Generated: 2025-10-25
Specification: spec.md

## Understanding

This work is **already complete** on the `cleanup/merged` branch. This plan documents the completed work and guides PR creation.

**What was completed**:
1. Aligned spec/README.md and spec/template.md with CSW 0.4.0
2. Removed spec/SHIPPED.md (CSW 0.4.0 no longer uses this)
3. Removed `spec/active/*/log.md` from .gitignore (was blocking log.md tracking)
4. Deleted spec/completed/ directory (trust git history, not archiving)
5. Consolidated duplicate railway preview specs into single source of truth

**CSW 0.4.0 Changes**:
- SHIPPED.md tracking removed entirely
- GitHub PRs are canonical source of truth (`gh pr list --state merged`)
- log.md files are proof a spec is complete (must be tracked in git)

## Relevant Files

**Files Modified** (already done):
- `.gitignore` - Removed `spec/active/*/log.md` pattern
- `spec/README.md` - Updated to CSW 0.4.0 standards
- `spec/template.md` - Updated to CSW 0.4.0 standards
- `spec/active/railway-preview/spec.md` - Consolidated from TRA-81 spec

**Files Deleted** (already done):
- `spec/SHIPPED.md` - No longer used in CSW 0.4.0
- `spec/completed/internal-structure/` - Trust git history
- `spec/TRA-81-railway-preview-deployment.md` - Consolidated into railway-preview/spec.md

## Commits Ready for PR

Branch: `cleanup/merged`

1. **7c5a23d** - `chore: cleanup shipped features`
2. **35dc363** - `chore: align spec workflow with CSW 0.4.0 (remove SHIPPED.md tracking)`
3. **e7e5c12** - `chore: remove spec/active/*/log.md from gitignore (CSW 0.4.0 alignment)`
4. **949001c** - `chore: remove SHIPPED.md from completed spec (CSW 0.4.0 cleanup)`
5. **6d75397** - `chore: delete shipped spec (trust git history)`
6. **8f4604a** - `docs: consolidate railway preview deployment specs`
7. **f73d119** - `docs: split housekeeping spec into two focused specs`

## Task Breakdown

### Task 1: Verify Branch State
**Action**: VERIFY
**Command**: Check we're on cleanup/merged with all commits

**Implementation**:
```bash
git branch --show-current  # Should show: cleanup/merged
git log --oneline origin/main..HEAD  # Should show 7 commits
git status  # Should be clean
```

**Validation**:
```bash
[[ $(git branch --show-current) == "cleanup/merged" ]] && echo "✅ On cleanup/merged"
[[ $(git log --oneline origin/main..HEAD | wc -l) -ge 6 ]] && echo "✅ Has commits"
[[ -z $(git status --porcelain) ]] && echo "✅ Working tree clean"
```

---

### Task 2: Create Pull Request
**Action**: CREATE PR
**Command**: Use `gh pr create`

**Implementation**:
```bash
gh pr create \
  --base main \
  --head cleanup/merged \
  --title "docs: align spec workflow with CSW 0.4.0" \
  --body "$(cat <<'EOF'
## Summary

Align specification workflow with CSW 0.4.0 standards. This removes SHIPPED.md tracking in favor of using GitHub PRs as the canonical source of truth.

## Changes

### CSW 0.4.0 Alignment
- ✅ Remove SHIPPED.md tracking (deprecated in CSW 0.4.0)
- ✅ Remove `spec/active/*/log.md` from gitignore (log.md must be tracked)
- ✅ Update spec/README.md to match CSW 0.4.0 standards
- ✅ Update spec/template.md to match CSW 0.4.0 standards

### Cleanup
- ✅ Delete spec/completed/ directory (trust git history, no archiving)
- ✅ Consolidate railway preview specs (TRA-81 → railway-preview/spec.md)
- ✅ Split remaining work into focused specs (doc-cleanup + justfile-delegation)

## Rationale

**CSW 0.4.0 Changelog** (https://github.com/trakrf/claude-spec-workflow/blob/main/CHANGELOG.md):
- SHIPPED.md became problematic (commit SHAs invalid after squash/rebase)
- GitHub PRs are canonical: `gh pr list --state merged`
- log.md presence proves a spec is complete
- /cleanup automatically removes leftover SHIPPED.md files

## Testing

- [x] All specs readable and follow CSW 0.4.0 format
- [x] No SHIPPED.md files remain
- [x] log.md files no longer ignored
- [x] Git history preserves all deleted content

## Breaking Changes

None - this is documentation and tooling alignment only.

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

**Validation**:
```bash
gh pr list --state open --head cleanup/merged  # Should show the new PR
```

---

### Task 3: Verify PR Details
**Action**: VERIFY
**Command**: Check PR was created successfully

**Implementation**:
```bash
gh pr view cleanup/merged
```

**Expected**:
- Title: "docs: align spec workflow with CSW 0.4.0"
- Base: main
- Head: cleanup/merged
- State: OPEN
- No merge conflicts

**Validation**:
```bash
gh pr view cleanup/merged --json state,title,baseRefName | \
  jq -e '.state == "OPEN" and .baseRefName == "main"' && \
  echo "✅ PR created successfully"
```

---

## Risk Assessment

**Risk**: PR merge conflicts with main
**Mitigation**: Branch was rebased on main before planning (commit e7e5c12 shows rebase)

**Risk**: Breaking CSW workflow for other specs
**Mitigation**: Changes are additive (unblock log.md) and cleanup (remove deprecated SHIPPED.md)

**Risk**: Losing historical spec data
**Mitigation**: All deletions preserved in git history, recoverable via `git log --all -- spec/`

## Integration Points
- CSW tooling: Aligned with CSW 0.4.0 (no tool changes needed)
- Spec workflow: SHIPPED.md → GitHub PRs (automatic via /ship)
- Documentation: README.md and template.md updated

## VALIDATION GATES (MANDATORY)

This is a **documentation PR** - no code validation needed.

**After Task 2 (PR creation)**:
```bash
# Gate 1: PR exists and is open
gh pr view cleanup/merged --json state --jq '.state' | grep -q "OPEN"

# Gate 2: No merge conflicts
gh pr view cleanup/merged --json mergeable --jq '.mergeable' | grep -q "MERGEABLE"

# Gate 3: Commits are present
gh pr view cleanup/merged --json commits --jq '.commits | length' | grep -qE "[6-9]|[1-9][0-9]+"
```

**Enforcement Rules**:
- If PR creation fails → Check gh authentication
- If merge conflicts → Rebase on latest main
- All gates must pass before requesting review

## Validation Sequence

**After Task 2**:
```bash
# Verify PR created
gh pr view cleanup/merged

# Check for conflicts
gh pr checks cleanup/merged  # If any checks configured

# View diff summary
gh pr diff cleanup/merged --stat
```

**Final validation**:
```bash
echo "✅ All validation passed - PR ready for review"
```

## Plan Quality Assessment

**Complexity Score**: 1/10 (TRIVIAL)
**Confidence Score**: 10/10 (CERTAIN)

**Confidence Factors**:
✅ All work already completed
✅ Just need to create PR
✅ No code changes
✅ Documentation only
✅ Already rebased on main
✅ Clean working tree

**Assessment**: Trivial execution - work is done, just documenting and creating PR.

**Estimated one-pass success probability**: 100%

**Reasoning**: All work completed and committed. Task is simply creating a GitHub PR via `gh` CLI, which is straightforward and has clear validation.
