# Feature: Optional Asset ID with Auto-Generation

**Linear**: [TRA-260](https://linear.app/trakrf/issue/TRA-260/make-asset-id-optional-with-auto-generation)
**Priority**: Urgent
**Labels**: Launch, frontend, backend

## Origin

NADA customer feedback - they don't have a mature fixed asset tracking system and don't want to manually specify IDs for ~100 assets.

## Outcome

Users can create assets without specifying an identifier. The system auto-generates a unique ID when none is provided, removing friction while preserving data integrity.

## User Story

As an **asset manager without a fixed asset tracking system**
I want to **create assets without specifying an ID**
So that I can **quickly onboard assets without worrying about naming conventions**

## Context

**Discovery**: Customer interview revealed friction in asset creation workflow. Users without mature asset management systems find the required ID field blocking.

**Current State**: Asset ID (identifier) is required. Users must think of a naming scheme before they can create assets.

**Desired State**: Asset ID is optional. Auto-generated IDs allow fast asset creation while preserving the create-then-tag workflow.

## Technical Requirements

### Frontend Changes

1. **Asset ID field**
   - Remove required asterisk/indicator
   - Update placeholder text to "Leave blank to auto-generate"
   - Allow form submission with empty Asset ID field
   - Validation should no longer require this field

### Backend Changes

1. **Auto-generation logic**
   - If `identifier` is blank/empty on asset create, auto-generate
   - Format: `ASSET-{4_digit_sequential}`
   - Example: `ASSET-0001`, `ASSET-0002`, etc.
   - Zero-padded to 4 digits
   - Sequential counter per organization

2. **Sequence tracking**
   - Track next sequence number per organization
   - Atomic increment to prevent duplicates under concurrency
   - Start at 0001 for new organizations

3. **Data integrity**
   - Identifier field always has a value (no nulls)
   - Auto-generated IDs guaranteed unique within organization
   - Sequential ensures predictable, human-friendly IDs

4. **Key design**
   - Asset ID (`identifier`) is alternate/natural key - editable by customer
   - Surrogate key (UUID) is true primary key - never changes
   - Customer can replace auto-generated ID with their own at any time
   - DB should have unique index + NOT NULL on `(org_id, identifier)` - verify during implementation
   - Backend handles constraint violation → friendly error message

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| ID format | `ASSET-{sequential}` | Tim's preference - predictable, human-readable |
| Length | 4+ digits | Minimum 4, grows naturally beyond 9999 |
| Padding | Zero-padded (min 4) | Consistent length, sorts correctly |
| Scope | Per-org sequence | Multi-tenant isolation |
| Overflow | Natural rollover | 10000+ just grows to 5 digits |

## Out of Scope

- Org-prefixed IDs (e.g., `NADA-0001`) - future enhancement if requested
- Custom auto-generation patterns - future enhancement if requested

## Validation Criteria

- [ ] User can create asset with empty Asset ID field
- [ ] Auto-generated ID appears in format `ASSET-0001`
- [ ] Auto-generated ID is unique within organization
- [ ] User can edit auto-generated ID to custom value (alternate key, not PK)
- [ ] Duplicate identifier returns validation error
- [ ] Export/import handles auto-generated IDs correctly
- [ ] Existing assets with custom IDs unaffected

## Edge Cases

1. **Concurrent creates**: Atomic increment prevents duplicate sequence numbers
2. **Edit to blank**: Require non-empty value on update (auto-gen only on create)
3. **Import with blank ID**: Auto-generate during import (each gets next sequence)
4. **Bulk create**: Each asset gets next sequential ID in order
5. **Deleted assets**: Sequence numbers not reused (gaps are acceptable)
6. **Beyond 9999**: Naturally rolls to 5 digits (ASSET-10000) - customers at this scale should have their own asset management system
7. **Duplicate on edit**: Return validation error "Asset ID already in use"

## Files Likely Affected

### Frontend
- Asset creation form component
- Form validation logic
- Asset ID field placeholder/label

### Backend
- Asset create endpoint/service
- Sequence counter storage (org settings or dedicated table)
- ID generation utility function
- Asset model/validation

## Testing Requirements

### Unit Tests
- ID generation produces valid format (`ASSET-0001`)
- Zero-padding works correctly (1 → 0001, 99 → 0099)
- Empty identifier triggers auto-generation
- Non-empty identifier preserved as-is

### Integration Tests
- Create asset without ID returns auto-generated ID
- Create asset with ID preserves provided ID
- Sequential creates produce incrementing IDs (0001, 0002, 0003)
- Concurrent creates produce unique IDs (no duplicates)
- Different orgs have independent sequences
- Edit to duplicate identifier returns error
- Same identifier allowed in different orgs
