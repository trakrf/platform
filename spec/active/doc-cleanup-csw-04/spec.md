# Feature: CSW 0.4.0 Documentation Cleanup

## Origin
This spec captures completed work on the `cleanup/merged` branch that aligns our documentation structure with CSW 0.4.0 standards.

## Outcome
Documentation structure fully aligned with CSW 0.4.0 - SHIPPED.md tracking removed, log.md files unblocked, shipped specs deleted (preserved in git history).

## User Story
**As a developer**
I want our spec workflow aligned with CSW 0.4.0
So that shipped features are tracked via GitHub PRs (canonical source) instead of SHIPPED.md files

## Context

### Discovery
- CSW 0.4.0 removed SHIPPED.md tracking entirely
- GitHub PRs are now the canonical source of truth (`gh pr list --state merged`)
- `log.md` presence is the real proof a spec is complete
- `spec/active/*/log.md` was incorrectly in gitignore, blocking this

### Current State (cleanup/merged branch)
**Commits ready for PR:**
1. `7c5a23d` - cleanup shipped features
2. `35dc363` - align spec workflow with CSW 0.4.0 (remove SHIPPED.md tracking)
3. `e7e5c12` - remove spec/active/*/log.md from gitignore (CSW 0.4.0 alignment)
4. `949001c` - remove SHIPPED.md from completed spec (CSW 0.4.0 cleanup)
5. `6d75397` - delete shipped spec (trust git history)
6. `8f4604a` - consolidate railway preview deployment specs

### Desired State
- PR created from `cleanup/merged` branch
- All commits merged to main
- Documentation aligned with CSW 0.4.0

## Technical Requirements

### Changes Already Completed (on cleanup/merged branch)
- ✅ Updated spec/README.md and spec/template.md to match CSW 0.4.0
- ✅ Removed spec/SHIPPED.md (no longer used in CSW 0.4.0)
- ✅ Removed `spec/active/*/log.md` pattern from .gitignore
- ✅ Deleted spec/completed/internal-structure/ (trust git history)
- ✅ Consolidated spec/TRA-81-railway-preview-deployment.md into spec/active/railway-preview/spec.md

### Task
Create PR from `cleanup/merged` branch to `main`

## Validation Criteria
- [ ] PR created from `cleanup/merged` branch
- [ ] PR title: "docs: align spec workflow with CSW 0.4.0"
- [ ] PR description includes summary of 6 commits
- [ ] All commits have conventional commit messages
- [ ] No merge conflicts with main
- [ ] Ready to merge

## Success Metrics
- Documentation aligned with CSW 0.4.0
- SHIPPED.md tracking removed (GitHub PRs are canonical)
- log.md files can be tracked properly
- Clean git history with semantic commits
