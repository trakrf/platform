# Implementation Plan: Optional Asset ID with Auto-Generation

Generated: 2026-01-12
Specification: spec.md
Linear: [TRA-260](https://linear.app/trakrf/issue/TRA-260/make-asset-id-optional-with-auto-generation)

## Understanding

Make the Asset ID field optional during asset creation. When left blank, the backend auto-generates a sequential ID in format `ASSET-0001`, `ASSET-0002`, etc. The sequence is derived from existing assets (no stored counter), making it self-healing and resilient to manually entered IDs.

**Key decisions from planning**:
- Derive sequence from `MAX(identifier)` query - no metadata storage
- Simple uniqueness per org (ignore `valid_from` for now)
- Frontend shows inline hint when field empty

## Relevant Files

**Reference Patterns**:
- `backend/internal/storage/assets.go` (lines 14-42) - existing CreateAsset with constraint handling
- `backend/migrations/000001_prereqs.up.sql` (lines 26-59) - sequence generation patterns
- `frontend/src/components/assets/AssetForm.tsx` (lines 78-82) - current validation logic

**Files to Modify**:
- `backend/internal/models/asset/asset.go` (line 30) - remove `required` from Identifier validation
- `backend/internal/storage/assets.go` (lines 14-42) - add auto-generation logic
- `frontend/src/components/assets/AssetForm.tsx` (lines 78-82, 163) - make field optional, add hint
- `frontend/src/types/assets/index.ts` (line 46) - make identifier optional in CreateAssetRequest

**Files to Create**:
- None - all changes fit in existing files

## Architecture Impact

- **Subsystems affected**: Frontend form, Backend API, Database queries
- **New dependencies**: None
- **Breaking changes**: None - existing assets unaffected, identifier still required in DB

## Task Breakdown

### Task 1: Backend - Add sequence derivation query

**File**: `backend/internal/storage/assets.go`
**Action**: MODIFY
**Pattern**: Reference existing query patterns in same file

**Implementation**:
```go
// Add new function to derive next sequence number
func (s *AssetStorage) GetNextAssetSequence(ctx context.Context, orgID int) (int, error) {
    var maxSeq sql.NullInt64
    query := `
        SELECT MAX(CAST(SUBSTRING(identifier FROM 'ASSET-([0-9]+)') AS INT))
        FROM trakrf.assets
        WHERE org_id = $1
          AND identifier ~ '^ASSET-[0-9]+$'
          AND deleted_at IS NULL
    `
    err := s.db.QueryRowContext(ctx, query, orgID).Scan(&maxSeq)
    if err != nil {
        return 0, fmt.Errorf("failed to get max sequence: %w", err)
    }
    if !maxSeq.Valid {
        return 1, nil // Start at 1 if no existing ASSET-XXXX
    }
    return int(maxSeq.Int64) + 1, nil
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 2: Backend - Add identifier generation helper

**File**: `backend/internal/storage/assets.go`
**Action**: MODIFY

**Implementation**:
```go
// Add helper to generate identifier
func GenerateAssetIdentifier(seq int) string {
    return fmt.Sprintf("ASSET-%04d", seq) // Zero-pad to 4 digits, grows naturally beyond 9999
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 3: Backend - Modify CreateAsset to auto-generate when empty

**File**: `backend/internal/storage/assets.go`
**Action**: MODIFY
**Pattern**: Reference lines 14-42 for existing create logic

**Implementation**:
```go
// In CreateAsset function, before insert:
if request.Identifier == "" {
    seq, err := s.GetNextAssetSequence(ctx, request.OrgID)
    if err != nil {
        return nil, fmt.Errorf("failed to generate identifier: %w", err)
    }
    request.Identifier = GenerateAssetIdentifier(seq)
}
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 4: Backend - Remove required validation from Identifier

**File**: `backend/internal/models/asset/asset.go`
**Action**: MODIFY (line 30)

**Change**:
```go
// Before:
Identifier string `json:"identifier" validate:"required,min=1,max=255"`

// After:
Identifier string `json:"identifier" validate:"omitempty,max=255"`
```

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 5: Backend - Update CreateAssetWithIdentifiers for auto-generation

**File**: `backend/internal/storage/assets.go`
**Action**: MODIFY
**Pattern**: Reference CreateAssetWithIdentifiers function (lines 309-347)

**Implementation**:
Same auto-generation logic needs to apply in the `CreateAssetWithIdentifiers` path.

**Validation**:
```bash
cd backend && just lint && just test
```

---

### Task 6: Backend - Add unit tests for sequence derivation

**File**: `backend/internal/storage/assets_test.go`
**Action**: MODIFY or CREATE

**Test cases**:
- `TestGetNextAssetSequence_Empty` - returns 1 when no ASSET-XXXX exist
- `TestGetNextAssetSequence_WithExisting` - returns max+1
- `TestGetNextAssetSequence_WithGaps` - handles gaps correctly
- `TestGenerateAssetIdentifier_Formatting` - zero-padding works
- `TestGenerateAssetIdentifier_Beyond9999` - grows to 5 digits

**Validation**:
```bash
cd backend && just test
```

---

### Task 7: Frontend - Make identifier optional in types

**File**: `frontend/src/types/assets/index.ts`
**Action**: MODIFY (line 46)

**Change**:
```typescript
// Before:
identifier: string;

// After:
identifier?: string;
```

**Validation**:
```bash
cd frontend && just typecheck
```

---

### Task 8: Frontend - Update form validation

**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY (lines 78-82)

**Change**:
```typescript
// Before:
if (!formData.identifier.trim()) {
  errors.identifier = 'Asset ID is required';
} else if (!/^[a-zA-Z0-9-_]+$/.test(formData.identifier)) {
  errors.identifier = 'Asset ID must contain only letters, numbers, hyphens, and underscores';
}

// After:
if (formData.identifier.trim() && !/^[a-zA-Z0-9-_]+$/.test(formData.identifier)) {
  errors.identifier = 'Asset ID must contain only letters, numbers, hyphens, and underscores';
}
// Empty is now valid - backend will auto-generate
```

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 9: Frontend - Update field placeholder and hint

**File**: `frontend/src/components/assets/AssetForm.tsx`
**Action**: MODIFY

**Implementation**:
- Update placeholder to "Leave blank to auto-generate"
- Remove required asterisk/indicator
- Add hint text below field when empty: "Will be auto-generated as ASSET-XXXX"

**Validation**:
```bash
cd frontend && just lint && just typecheck
```

---

### Task 10: Integration test - End-to-end auto-generation

**File**: `backend/internal/handlers/assets/assets_test.go` or integration test file
**Action**: MODIFY

**Test cases**:
- Create asset without identifier → returns ASSET-0001
- Create second asset without identifier → returns ASSET-0002
- Create asset with identifier → preserves provided identifier
- Create asset with duplicate identifier → returns error

**Validation**:
```bash
cd backend && just test
```

## Risk Assessment

| Risk | Likelihood | Impact | Mitigation |
|------|------------|--------|------------|
| Race condition on concurrent creates | Low | Medium | DB unique constraint catches duplicates; retry with next sequence |
| Regex mismatch between Go/Postgres | Low | High | Test regex pattern matches exactly |
| Existing ASSET-XXXX identifiers | Low | Low | Query handles existing data correctly |

## Integration Points

- **API contract**: Identifier becomes optional in request, always present in response
- **Database**: No schema changes - constraint unchanged
- **Frontend**: Form validation relaxed, hint added

## VALIDATION GATES (MANDATORY)

After EVERY code change:
```bash
# Backend
cd backend && just lint && just test

# Frontend
cd frontend && just lint && just typecheck && just test

# Full validation
just validate
```

**Enforcement Rules**:
- If ANY gate fails → Fix immediately
- Re-run validation after fix
- Do not proceed to next task until current task passes all gates

## Validation Sequence

After each task: Run relevant lint/typecheck/test commands

Final validation:
```bash
just validate
just build
```

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 8/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec and planning discussion
✅ Similar patterns found in codebase (sequence generation, constraint handling)
✅ All clarifying questions answered
✅ Existing test patterns to follow
✅ No new dependencies
✅ No schema changes required
⚠️ Regex must match between Go and Postgres - needs careful testing

**Assessment**: Well-defined feature with clear patterns to follow. Main complexity is ensuring the sequence derivation query works correctly across edge cases.

**Estimated one-pass success probability**: 85%

**Reasoning**: Clear scope, existing patterns, no schema changes. Minor risk around regex pattern matching between languages.
