# Specification-Driven Development System

This system combines the best practices from Context Engineering (Cole Medin), 3-File PRD (Ryan Carson), and Linear workflow (Pedram Navid) to create an efficient, AI-friendly development process.

## Quick Start

### First Time: Ship the Bootstrap Spec

After installation, you'll find a bootstrap spec at `spec/bootstrap/`. This validates your setup:

```
/plan bootstrap
/build
/check
/ship
```

This proves CSW works and commits the infrastructure cleanly. You'll experience the full workflow and create your first SHIPPED.md entry.

### Creating New Features

1. **Create a specification**
   ```bash
   mkdir -p spec/my-feature
   cp spec/template.md spec/my-feature/spec.md
   # Edit spec.md with your requirements
   ```

2. **Generate implementation plan**
   ```
   /plan my-feature
   # or just: /plan (auto-detects if only one spec)
   ```

3. **Build the feature**
   ```
   /build
   # Auto-detects the spec with plan.md
   ```

4. **Validate readiness**
   ```
   /check
   ```

5. **Ship it**
   ```
   /ship
   # Auto-detects the spec ready to ship
   ```

## Directory Structure

```
spec/
├── README.md          # This file
├── template.md        # Specification template
├── stack.md           # Validation commands for your tech stack
├── active/            # Current work
│   └── {feature}/     # One directory per feature
│       ├── spec.md    # Feature specification
│       ├── plan.md    # Generated implementation plan
│       └── log.md     # Build progress log
└── SHIPPED.md         # Log of completed features
```

**Note**: The slash commands (`/plan`, `/build`, `/check`, `/ship`, `/spec`) are installed globally in Claude's commands directory, not in your project.

## Philosophy

This system is built on three core principles:

1. **Context is King** - Provide comprehensive context to enable autonomous execution
2. **Progressive Validation** - Validate continuously, fix immediately
3. **Clear Workflow** - Separate planning from execution for better results

## Workflow Overview

### 1. Specification (Human writes)
Define WHAT needs to be built, not HOW. Include:
- Clear outcome and success criteria
- Examples and references
- Constraints and context

### 2. Planning (AI generates)
AI analyzes the spec and creates detailed implementation plan:
- Task breakdown
- Risk assessment
- Validation steps

### 3. Building (AI executes)
AI implements based on plan with:
- Continuous validation
- Progress tracking
- Error recovery

### 4. Shipping (AI completes)
Final validation and PR preparation:
- Comprehensive checks
- Documentation updates
- Clean git history

## Command Reference

| Command | Purpose | Input |
|---------|---------|-------|
| `/plan` | Generate implementation plan | `spec/active/{feature}/spec.md` |
| `/build` | Execute implementation | `spec/active/{feature}/` |
| `/check` | Validate PR readiness | None |
| `/ship` | Complete and archive | `spec/active/{feature}/` |

## Best Practices

### DO:
- ✅ Write clear, specific requirements
- ✅ Include examples from the codebase
- ✅ Reference documentation
- ✅ Define validation criteria
- ✅ Use semantic commit messages

### DON'T:
- ❌ Mix multiple features in one spec
- ❌ Skip validation steps
- ❌ Ignore failing tests
- ❌ Ship without running /check
- ❌ Leave console.logs in code

## Validation Standards

All features must pass validation commands defined in `spec/stack.md`:
- **Lint** - No linting errors
- **Typecheck** - No type errors (if applicable to your stack)
- **Test** - All tests passing
- **Build** - Successful build

The specific commands depend on your tech stack. See `spec/stack.md` for your project's validation commands.

## Git Workflow

1. Features are developed on `feature/{name}` branches
2. Each feature gets semantic commits
3. Specs are archived by reference, not content
4. Clean history with meaningful commit messages

## Troubleshooting

**Build fails validation?**
- Check log.md for specific errors
- Fix the code, not the tests
- Re-run validation

**Can't ship?**
- Run `/check` for detailed report
- Fix all critical issues
- Try again

**Lost context?**
- Check log.md for progress
- Plan.md has the full strategy
- Resume from last completed task