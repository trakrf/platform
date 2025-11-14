# Implementation Log: Locations CRUD with ltree

## Status: Planning Complete ✅

**Branch**: `feature/locations-crud`
**Started**: 2025-11-10

---

## Phase 1: Database Layer
**Status**: Not Started
**Estimated**: 2-3 hours

### Tasks
- [ ] Create migration 000018_locations_add_ltree.up.sql
- [ ] Create down migration
- [ ] Test migration locally
- [ ] Write ltree storage tests
- [ ] Verify path generation works
- [ ] Verify trigger updates paths correctly

### Notes
- Using ltree extension for efficient hierarchy queries
- Keeping parent_location_id for backward compatibility
- Path format: `usa.california.warehouse_1.section_a`

---

## Phase 2: Backend API
**Status**: Not Started
**Estimated**: 4-5 hours

### Tasks
- [ ] Create models/location package
- [ ] Implement storage layer functions
- [ ] Create handlers package
- [ ] Register routes in main.go
- [ ] Write unit tests
- [ ] Write integration tests
- [ ] Update Swagger docs

### Notes
- Following assets CRUD pattern
- Org isolation via RLS policies
- Hierarchy endpoints: ancestors, descendants, children

---

## Phase 3: Frontend Foundation
**Status**: Not Started
**Estimated**: 3-4 hours

### Tasks
- [ ] Create TypeScript types
- [ ] Create API client
- [ ] Create Zustand store
- [ ] Create custom hooks
- [ ] Write store tests
- [ ] Write hook tests

### Notes
- Store similar to assetStore pattern
- Cache by ID in Map for O(1) lookup
- Tree building on demand from flat list

---

## Phase 4: Frontend UI
**Status**: Not Started
**Estimated**: 6-8 hours

### Tasks
- [ ] Create LocationsPage
- [ ] Create LocationTree component
- [ ] Create LocationForm component
- [ ] Create LocationCard component
- [ ] Create LocationBreadcrumb component
- [ ] Create LocationMoveDialog component
- [ ] Add route to App.tsx
- [ ] Write component tests
- [ ] Write E2E tests

### Notes
- Tree component with expand/collapse
- Drag-and-drop for move (nice to have)
- Mobile responsive
- Search/filter support

---

## Decisions Made

### 2025-11-10: Chose ltree over pure adjacency list
**Reason**: Better query performance for hierarchy operations, PostgreSQL native extension, proven at scale.

**Alternatives Considered**:
- Pure adjacency list (too slow for deep trees)
- Closure table (over-engineered for this use case)
- Nested sets (maintenance nightmare)
- Hybrid adjacency + materialized path (no extensions, but manual maintenance)

**Decision**: ltree - best balance of simplicity and performance.

---

## Open Questions

None at this time.

---

## Blockers

None at this time.

---

## Next Steps

1. Start Phase 1: Create migration and enable ltree
2. Test migration thoroughly with various hierarchy depths
3. Move to Phase 2 once database layer is solid
