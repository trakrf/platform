# TRA-503 Pagination Envelope Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make every public-API list endpoint return the standard `{data, limit, offset, total_count}` envelope, marshaled from typed structs, with real `LIMIT`/`OFFSET` pagination on the four endpoints that don't have it today.

**Architecture:** Add new paginated storage methods (`ListXPaginated`, `CountX`) alongside the existing unbounded ones — internal callers stay on the old methods, handlers move to the new ones. Each handler gets a per-endpoint named response struct (no shared generics) that handler returns directly instead of `map[string]any`. Standardize all list endpoints on `default=50, max=200`.

**Tech Stack:** Go (chi router), pgx + ltree for hierarchy queries, swaggo for OpenAPI generation, pgxmock for storage unit tests, real-DB integration tests behind the `integration` build tag, testify (`assert`/`require`) for assertions.

**Spec:** `docs/superpowers/specs/2026-04-25-tra-503-pagination-envelope-design.md`

---

## Task 1: Storage — `ListAncestorsPaginated` + `CountAncestors`

**Files:**
- Modify: `backend/internal/storage/locations.go` (add two methods near existing `GetAncestors` at line 326)
- Test: `backend/internal/storage/locations_test.go` (add tests near existing `TestGetAncestors` at line 567)

The existing `GetAncestors` query already orders `BY l.depth`. The paginated version needs `ORDER BY l.depth ASC, l.id ASC LIMIT $3 OFFSET $4` for stable paging. `scanHierarchyRows` accepts variadic args, so we can pass `(orgID, orgID, id, limit, offset)`.

`CountAncestors` runs a `SELECT COUNT(*)` with the same `WHERE` clause — no JOIN, no projection — wrapped in `WithOrgTx` for org scoping.

- [ ] **Step 1: Write the failing test for `ListAncestorsPaginated`**

Add to `backend/internal/storage/locations_test.go` (after `TestGetAncestors_RootLocation` at line 650):

```go
func TestListAncestorsPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}

	now := time.Now()
	orgID := 1
	locationID := 3
	limit := 1
	offset := 1

	parent1 := 1
	usaIdent := "usa"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_identifier",
	}).
		AddRow(2, 1, "California", "california", &parent1, "usa.california", 2, "California State", now, nil, true, now, &now, nil, &usaIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY l.depth ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, locationID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value, is_active`).
		WithArgs([]int{2}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value", "is_active"}))
	mock.ExpectCommit()

	results, err := storage.ListAncestorsPaginated(context.Background(), orgID, locationID, limit, offset)

	assert.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "usa.california", results[0].Path)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountAncestors(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	locationID := 3

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM trakrf\.locations`).
		WithArgs(orgID, locationID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectCommit()

	n, err := storage.CountAncestors(context.Background(), orgID, locationID)
	assert.NoError(t, err)
	assert.Equal(t, 2, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
just backend test ./internal/storage/... -run 'TestListAncestorsPaginated|TestCountAncestors'
```

Expected: FAIL with "undefined: ListAncestorsPaginated" / "undefined: CountAncestors".

- [ ] **Step 3: Add the two storage methods**

Insert into `backend/internal/storage/locations.go` immediately after the existing `GetAncestors` function (after line 341):

```go
// ListAncestorsPaginated returns the ancestors of a location ordered by depth
// (root first), with LIMIT/OFFSET applied. The id ASC tiebreaker ensures
// fully-deterministic paging across requests with the same offset.
func (s *Storage) ListAncestorsPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path @> (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.depth ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "ancestor", orgID, orgID, id, limit, offset)
}

// CountAncestors returns the total number of ancestors of the given location,
// matching the WHERE clause used by ListAncestorsPaginated.
func (s *Storage) CountAncestors(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND path @> (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND id != $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
just backend test ./internal/storage/... -run 'TestListAncestorsPaginated|TestCountAncestors'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go
git commit -m "feat(tra-503): add ListAncestorsPaginated and CountAncestors storage methods"
```

---

## Task 2: Storage — `ListChildrenPaginated` + `CountChildren`

**Files:**
- Modify: `backend/internal/storage/locations.go` (add two methods near existing `GetChildren` at line 373)
- Test: `backend/internal/storage/locations_test.go` (add tests near existing `TestGetChildren` at line 744)

Existing `GetChildren` orders `BY l.name`. Paginated version uses `ORDER BY l.name ASC, l.id ASC LIMIT $3 OFFSET $4`.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/storage/locations_test.go` (after `TestGetChildren_NoChildren`):

```go
func TestListChildrenPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	now := time.Now()
	orgID := 1
	parentID := 1
	limit := 2
	offset := 0

	parentRef := 1
	parentIdent := "parent"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_identifier",
	}).
		AddRow(2, 1, "Aisle A", "aisle-a", &parentRef, "parent.aisle-a", 2, "", now, nil, true, now, &now, nil, &parentIdent).
		AddRow(3, 1, "Aisle B", "aisle-b", &parentRef, "parent.aisle-b", 2, "", now, nil, true, now, &now, nil, &parentIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY l.name ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, parentID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value, is_active`).
		WithArgs([]int{2, 3}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value", "is_active"}))
	mock.ExpectCommit()

	results, err := storage.ListChildrenPaginated(context.Background(), orgID, parentID, limit, offset)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountChildren(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	parentID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM trakrf\.locations`).
		WithArgs(orgID, parentID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(5))
	mock.ExpectCommit()

	n, err := storage.CountChildren(context.Background(), orgID, parentID)
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
just backend test ./internal/storage/... -run 'TestListChildrenPaginated|TestCountChildren'
```

Expected: FAIL with "undefined: ListChildrenPaginated" / "undefined: CountChildren".

- [ ] **Step 3: Add the two storage methods**

Insert into `backend/internal/storage/locations.go` immediately after the existing `GetChildren` function (after line 387):

```go
// ListChildrenPaginated returns immediate children (depth = parent_depth + 1)
// of a location ordered alphabetically by name, with LIMIT/OFFSET applied.
// The id ASC tiebreaker keeps paging deterministic when sibling names collide.
func (s *Storage) ListChildrenPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.parent_location_id = $2
		  AND l.deleted_at IS NULL
		ORDER BY l.name ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "child", orgID, orgID, id, limit, offset)
}

// CountChildren returns the total number of immediate children of the given
// location, matching the WHERE clause used by ListChildrenPaginated.
func (s *Storage) CountChildren(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND parent_location_id = $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
just backend test ./internal/storage/... -run 'TestListChildrenPaginated|TestCountChildren'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go
git commit -m "feat(tra-503): add ListChildrenPaginated and CountChildren storage methods"
```

---

## Task 3: Storage — `ListDescendantsPaginated` + `CountDescendants`

**Files:**
- Modify: `backend/internal/storage/locations.go` (add two methods near existing `GetDescendants` at line 350)
- Test: `backend/internal/storage/locations_test.go` (add tests near existing `TestGetDescendants` at line 652)

Existing `GetDescendants` orders `BY l.path`. Paginated version uses `ORDER BY l.path ASC, l.id ASC LIMIT $3 OFFSET $4`.

- [ ] **Step 1: Write the failing tests**

Add to `backend/internal/storage/locations_test.go` (after `TestGetDescendants_LeafLocation`):

```go
func TestListDescendantsPaginated(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	now := time.Now()
	orgID := 1
	rootID := 1
	limit := 2
	offset := 1

	parentRef := 1
	rootIdent := "root"
	rows := pgxmock.NewRows([]string{
		"id", "org_id", "name", "identifier", "parent_location_id", "path", "depth",
		"description", "valid_from", "valid_to", "is_active",
		"created_at", "updated_at", "deleted_at",
		"parent_identifier",
	}).
		AddRow(3, 1, "B", "b", &parentRef, "root.b", 2, "", now, nil, true, now, &now, nil, &rootIdent).
		AddRow(4, 1, "C", "c", &parentRef, "root.c", 2, "", now, nil, true, now, &now, nil, &rootIdent)

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`ORDER BY l.path ASC, l.id ASC\s+LIMIT \$3 OFFSET \$4`).
		WithArgs(orgID, rootID, limit, offset).
		WillReturnRows(rows)
	mock.ExpectCommit()

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT location_id, id, type, value, is_active`).
		WithArgs([]int{3, 4}, orgID).
		WillReturnRows(pgxmock.NewRows([]string{"location_id", "id", "type", "value", "is_active"}))
	mock.ExpectCommit()

	results, err := storage.ListDescendantsPaginated(context.Background(), orgID, rootID, limit, offset)
	assert.NoError(t, err)
	assert.Len(t, results, 2)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestCountDescendants(t *testing.T) {
	mock, err := pgxmock.NewPool()
	require.NoError(t, err)
	defer mock.Close()

	storage := &Storage{pool: mock}
	orgID := 1
	rootID := 1

	mock.ExpectBegin()
	mock.ExpectExec(`SET LOCAL app.current_org_id = 1`).WillReturnResult(pgxmock.NewResult("SET", 0))
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM trakrf\.locations`).
		WithArgs(orgID, rootID).
		WillReturnRows(pgxmock.NewRows([]string{"count"}).AddRow(7))
	mock.ExpectCommit()

	n, err := storage.CountDescendants(context.Background(), orgID, rootID)
	assert.NoError(t, err)
	assert.Equal(t, 7, n)
	assert.NoError(t, mock.ExpectationsWereMet())
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
just backend test ./internal/storage/... -run 'TestListDescendantsPaginated|TestCountDescendants'
```

Expected: FAIL.

- [ ] **Step 3: Add the two storage methods**

Insert into `backend/internal/storage/locations.go` immediately after `GetDescendants` (after line 365):

```go
// ListDescendantsPaginated returns all descendants of a location ordered
// depth-first by ltree path, with LIMIT/OFFSET applied. The id ASC tiebreaker
// keeps paging deterministic across calls.
func (s *Storage) ListDescendantsPaginated(ctx context.Context, orgID, id, limit, offset int) ([]location.LocationWithParent, error) {
	query := `
		SELECT l.id, l.org_id, l.name, l.identifier, l.parent_location_id, l.path, l.depth,
		       l.description, l.valid_from, l.valid_to, l.is_active, l.created_at, l.updated_at, l.deleted_at,
		       p.identifier
		FROM trakrf.locations l
		LEFT JOIN trakrf.locations p
			ON p.id = l.parent_location_id AND p.org_id = l.org_id AND p.deleted_at IS NULL
		WHERE l.org_id = $1
		  AND l.path <@ (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND l.id != $2
		  AND l.deleted_at IS NULL
		ORDER BY l.path ASC, l.id ASC
		LIMIT $3 OFFSET $4
	`
	return s.scanHierarchyRows(ctx, query, "descendant", orgID, orgID, id, limit, offset)
}

// CountDescendants returns the total number of descendants of the given
// location, matching the WHERE clause used by ListDescendantsPaginated.
func (s *Storage) CountDescendants(ctx context.Context, orgID, id int) (int, error) {
	query := `
		SELECT COUNT(*) FROM trakrf.locations
		WHERE org_id = $1
		  AND path <@ (SELECT path FROM trakrf.locations WHERE id = $2 AND org_id = $1 AND deleted_at IS NULL)
		  AND id != $2
		  AND deleted_at IS NULL
	`
	var n int
	err := s.WithOrgTx(ctx, orgID, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, orgID, id).Scan(&n)
	})
	return n, err
}
```

- [ ] **Step 4: Run tests to verify they pass**

```
just backend test ./internal/storage/... -run 'TestListDescendantsPaginated|TestCountDescendants'
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add backend/internal/storage/locations.go backend/internal/storage/locations_test.go
git commit -m "feat(tra-503): add ListDescendantsPaginated and CountDescendants storage methods"
```

---

## Task 4: Storage — `ListActiveAPIKeysPaginated`

**Files:**
- Modify: `backend/internal/storage/apikeys.go` (add method near existing `ListActiveAPIKeys` at line 48)
- Test: `backend/internal/storage/apikeys_integration_test.go` (this file uses real DB; mirror existing test pattern there)

`CountActiveAPIKeys` already exists at line 77 — only the paginated list is new. Existing `ListActiveAPIKeys` has no explicit `ORDER BY`; the new method adds `ORDER BY created_at DESC, id ASC LIMIT $2 OFFSET $3`.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/storage/apikeys_integration_test.go`. Mirrors the existing pattern (e.g., `TestAPIKeyStorage_CountActive`): `testutil.SetupTestDB`, `testutil.CreateTestAccount`, file-local `createTestUser`, direct `store.CreateAPIKey` calls.

```go
func TestAPIKeyStorage_ListActivePaginated(t *testing.T) {
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID := testutil.CreateTestAccount(t, pool)
	userID := createTestUser(t, pool)
	ctx := context.Background()

	// Seed three keys at distinct timestamps so created_at DESC ordering is observable.
	k1, err := store.CreateAPIKey(ctx, orgID, "first", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	k2, err := store.CreateAPIKey(ctx, orgID, "second", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)
	time.Sleep(2 * time.Millisecond)
	k3, err := store.CreateAPIKey(ctx, orgID, "third", []string{"assets:read"}, apikey.Creator{UserID: &userID}, nil)
	require.NoError(t, err)

	page1, err := store.ListActiveAPIKeysPaginated(ctx, orgID, 2, 0)
	require.NoError(t, err)
	require.Len(t, page1, 2)
	assert.Equal(t, k3.ID, page1[0].ID, "newest first")
	assert.Equal(t, k2.ID, page1[1].ID)

	page2, err := store.ListActiveAPIKeysPaginated(ctx, orgID, 2, 2)
	require.NoError(t, err)
	require.Len(t, page2, 1)
	assert.Equal(t, k1.ID, page2[0].ID)
}
```

- [ ] **Step 2: Run test to verify it fails**

```
just backend test -tags=integration ./internal/storage/... -run TestAPIKeyStorage_ListActivePaginated
```

Expected: FAIL with "undefined: ListActiveAPIKeysPaginated".

- [ ] **Step 3: Add the storage method**

Insert into `backend/internal/storage/apikeys.go` immediately after `ListActiveAPIKeys` (after line 74, before `CountActiveAPIKeys`). Match the existing `ListActiveAPIKeys` style exactly: direct `s.pool.Query` (no `WithOrgTx` wrap, since this file uses pool-level queries with explicit `WHERE org_id =`), same column order in SELECT and Scan.

```go
// ListActiveAPIKeysPaginated returns non-revoked keys for the given org
// ordered by creation time descending (newest first) with LIMIT/OFFSET
// applied. The id ASC tiebreaker keeps paging deterministic for keys
// created in the same instant.
func (s *Storage) ListActiveAPIKeysPaginated(ctx context.Context, orgID, limit, offset int) ([]apikey.APIKey, error) {
	rows, err := s.pool.Query(ctx, `
        SELECT id, jti, org_id, name, scopes, created_by, created_by_key_id,
               created_at, expires_at, last_used_at, revoked_at
        FROM trakrf.api_keys
        WHERE org_id = $1 AND revoked_at IS NULL
        ORDER BY created_at DESC, id ASC
        LIMIT $2 OFFSET $3
    `, orgID, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list api_keys paginated: %w", err)
	}
	defer rows.Close()

	out := []apikey.APIKey{}
	for rows.Next() {
		var k apikey.APIKey
		if err := rows.Scan(
			&k.ID, &k.JTI, &k.OrgID, &k.Name, &k.Scopes,
			&k.CreatedBy, &k.CreatedByKeyID,
			&k.CreatedAt, &k.ExpiresAt, &k.LastUsedAt, &k.RevokedAt,
		); err != nil {
			return nil, fmt.Errorf("scan api_key row: %w", err)
		}
		out = append(out, k)
	}
	return out, rows.Err()
}
```

- [ ] **Step 4: Run test to verify it passes**

```
just backend test -tags=integration ./internal/storage/... -run TestAPIKeyStorage_ListActivePaginated
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add backend/internal/storage/apikeys.go backend/internal/storage/apikeys_integration_test.go
git commit -m "feat(tra-503): add ListActiveAPIKeysPaginated storage method"
```

---

## Task 5: Handler — `GetAncestors` returns paginated envelope

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (struct around line 358; handler around line 551)
- Test: `backend/internal/handlers/locations/public_write_integration_test.go` (existing ancestor tests live around line 491; add new pagination test alongside)

The existing `LocationHierarchyResponse{Data}` is shared across all three hierarchy handlers. We're replacing it with three distinct named structs. Add `ListAncestorsResponse` now; we'll delete the shared one in Task 8 once all three handlers are converted.

- [ ] **Step 1: Write a failing integration test for the envelope shape**

Append to `backend/internal/handlers/locations/public_write_integration_test.go`. Helper conventions in this file (per existing tests at lines 490+): `testutil.SetupTestDB(t)` returns `(store, cleanup)`; `seedLocOrgAndKey(t, pool, store, "", scopes)` returns `(orgID, token)`; `buildLocationsPublicReadRouter(store)` returns the chi router; locations are seeded directly via `store.CreateLocation(ctx, locmodel.Location{...})`.

```go
func TestLocationsGetAncestors_PaginationEnvelope(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra503-loc-ancestors-paginate")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	root, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-root", Name: "Root", Path: "p503-root",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	mid, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-mid", Name: "Mid", Path: "p503-root.p503-mid",
		ParentLocationID: &root.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-leaf", Name: "Leaf", Path: "p503-root.p503-mid.p503-leaf",
		ParentLocationID: &mid.ID,
		ValidFrom:        time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicReadRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/p503-leaf/ancestors?limit=1&offset=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 1, body["limit"])
	assert.EqualValues(t, 1, body["offset"])
	assert.EqualValues(t, 2, body["total_count"], "two ancestors of leaf: root and mid")
	data := body["data"].([]any)
	require.Len(t, data, 1)
	first := data[0].(map[string]any)
	assert.Equal(t, "p503-mid", first["identifier"], "depth-asc + offset=1 skips root, returns mid")
}
```

- [ ] **Step 2: Run test to verify it fails**

```
just backend test -tags=integration ./internal/handlers/locations/... -run TestLocationsGetAncestors_PaginationEnvelope
```

Expected: FAIL — `total_count` and `limit` keys are missing from the current bare-array response.

- [ ] **Step 3: Add `ListAncestorsResponse` struct**

Add to `backend/internal/handlers/locations/locations.go` (near the existing `LocationHierarchyResponse` at line 358):

```go
// ListAncestorsResponse is the typed envelope returned by
// GET /api/v1/locations/{identifier}/ancestors.
type ListAncestorsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}
```

- [ ] **Step 4: Update `GetAncestors` handler to use paginated storage + new struct**

Replace the entire body of `GetAncestors` in `backend/internal/handlers/locations/locations.go` (currently at lines 551-583). Update the swaggo `@Success` annotation immediately above the function to reference `ListAncestorsResponse`. Note: this codebase names the request parameter `req` (not `r`) and uses `ctx` as the variable name for the request-ID string (yes, that's a misnomer in the existing code — match it for consistency).

```go
// @Success 200 {object} locations.ListAncestorsResponse
func (handler *Handler) GetAncestors(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, ctx)
		return
	}

	results, err := handler.storage.ListAncestorsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	total, err := handler.storage.CountAncestors(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListAncestorsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}
```

- [ ] **Step 5: Run the new test plus existing ancestor tests**

```
just backend test -tags=integration ./internal/handlers/locations/... -run 'GetAncestors|Ancestors'
```

Expected: PASS for both the new pagination-envelope test AND the pre-existing ancestor tests (which only check `data`).

- [ ] **Step 6: Commit**

```
git add backend/internal/handlers/locations/locations.go backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "feat(tra-503): paginate /locations/{id}/ancestors with envelope response"
```

---

## Task 6: Handler — `GetChildren` returns paginated envelope

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (handler at line 649; struct definitions area line 358)
- Test: `backend/internal/handlers/locations/public_write_integration_test.go` (existing children tests at line 557, 593)

Identical pattern to Task 5. Add `ListChildrenResponse`, replace handler body, update swaggo annotation.

- [ ] **Step 1: Write a failing integration test for the envelope shape**

Append to `public_write_integration_test.go`:

```go
func TestLocationsGetChildren_PaginationEnvelope(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra503-loc-children-paginate")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	parent, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-ch-parent", Name: "Parent", Path: "p503-ch-parent",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	for _, name := range []string{"alpha", "bravo", "charlie"} {
		_, err := store.CreateLocation(context.Background(), locmodel.Location{
			OrgID: orgID, Identifier: "p503-ch-" + name, Name: name,
			Path:             "p503-ch-parent.p503-ch-" + name,
			ParentLocationID: &parent.ID,
			ValidFrom:        time.Now(), IsActive: true,
		})
		require.NoError(t, err)
	}

	r := buildLocationsPublicReadRouter(store)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/p503-ch-parent/children?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body["limit"])
	assert.EqualValues(t, 0, body["offset"])
	assert.EqualValues(t, 3, body["total_count"])
	data := body["data"].([]any)
	require.Len(t, data, 2)
	assert.Equal(t, "alpha", data[0].(map[string]any)["name"], "alphabetical order, page 1")
	assert.Equal(t, "bravo", data[1].(map[string]any)["name"])
}
```

- [ ] **Step 2: Run test to verify it fails**

```
just backend test -tags=integration ./internal/handlers/locations/... -run TestLocationsGetChildren_PaginationEnvelope
```

Expected: FAIL.

- [ ] **Step 3: Add `ListChildrenResponse` struct**

Add to `backend/internal/handlers/locations/locations.go` immediately after `ListAncestorsResponse`:

```go
// ListChildrenResponse is the typed envelope returned by
// GET /api/v1/locations/{identifier}/children.
type ListChildrenResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}
```

- [ ] **Step 4: Update `GetChildren` handler**

Replace the entire body of `GetChildren` in `backend/internal/handlers/locations/locations.go` (currently at lines 649-681). Mirror Task 5 — same identifier resolution, swap storage calls to `ListChildrenPaginated` / `CountChildren`, return `ListChildrenResponse`.

```go
// @Success 200 {object} locations.ListChildrenResponse
func (handler *Handler) GetChildren(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, ctx)
		return
	}

	results, err := handler.storage.ListChildrenPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	total, err := handler.storage.CountChildren(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListChildrenResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}
```

- [ ] **Step 5: Run tests**

```
just backend test -tags=integration ./internal/handlers/locations/... -run 'GetChildren|Children'
```

Expected: PASS (new test + existing children tests).

- [ ] **Step 6: Commit**

```
git add backend/internal/handlers/locations/locations.go backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "feat(tra-503): paginate /locations/{id}/children with envelope response"
```

---

## Task 7: Handler — `GetDescendants` returns paginated envelope (with ordering test)

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (handler at line 600; struct area)
- Test: `backend/internal/handlers/locations/public_write_integration_test.go` (existing descendants tests at line 610)

Same pattern as Tasks 5–6, plus a dedicated **ordering test** that builds a small subtree with predictable path values and asserts page 1 + page 2 cover the expected slices in `path ASC` order.

- [ ] **Step 1: Write the failing tests (envelope + ordering)**

Append to `public_write_integration_test.go`:

```go
func TestLocationsGetDescendants_PaginationEnvelope(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra503-loc-descendants-paginate")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)

	orgID, token := seedLocOrgAndKey(t, pool, store, "", []string{"locations:read"})
	root, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-d-root", Name: "Root", Path: "p503-d-root",
		ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	a, err := store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-d-a", Name: "A", Path: "p503-d-root.p503-d-a",
		ParentLocationID: &root.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-d-a-1", Name: "A.1",
		Path:             "p503-d-root.p503-d-a.p503-d-a-1",
		ParentLocationID: &a.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)
	_, err = store.CreateLocation(context.Background(), locmodel.Location{
		OrgID: orgID, Identifier: "p503-d-b", Name: "B", Path: "p503-d-root.p503-d-b",
		ParentLocationID: &root.ID, ValidFrom: time.Now(), IsActive: true,
	})
	require.NoError(t, err)

	r := buildLocationsPublicReadRouter(store)

	// Page 1: limit=2, offset=0
	req := httptest.NewRequest(http.MethodGet, "/api/v1/locations/p503-d-root/descendants?limit=2&offset=0", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())

	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 3, body["total_count"])
	page1 := body["data"].([]any)
	require.Len(t, page1, 2)
	assert.Equal(t, "p503-d-a", page1[0].(map[string]any)["identifier"], "path-asc: p503-d-a comes first")
	assert.Equal(t, "p503-d-a-1", page1[1].(map[string]any)["identifier"], "path-asc: p503-d-a.p503-d-a-1 next")

	// Page 2: limit=2, offset=2
	req2 := httptest.NewRequest(http.MethodGet, "/api/v1/locations/p503-d-root/descendants?limit=2&offset=2", nil)
	req2.Header.Set("Authorization", "Bearer "+token)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	require.Equal(t, http.StatusOK, w2.Code, w2.Body.String())

	var body2 map[string]any
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &body2))
	page2 := body2["data"].([]any)
	require.Len(t, page2, 1)
	assert.Equal(t, "p503-d-b", page2[0].(map[string]any)["identifier"], "path-asc: p503-d-b last")
}
```

- [ ] **Step 2: Run test to verify it fails**

```
just backend test -tags=integration ./internal/handlers/locations/... -run TestLocationsGetDescendants_PaginationEnvelope
```

Expected: FAIL.

- [ ] **Step 3: Add `ListDescendantsResponse` struct**

Add to `backend/internal/handlers/locations/locations.go`:

```go
// ListDescendantsResponse is the typed envelope returned by
// GET /api/v1/locations/{identifier}/descendants.
type ListDescendantsResponse struct {
	Data       []location.PublicLocationView `json:"data"`
	Limit      int                           `json:"limit"       example:"50"`
	Offset     int                           `json:"offset"      example:"0"`
	TotalCount int                           `json:"total_count" example:"100"`
}
```

- [ ] **Step 4: Update `GetDescendants` handler**

Replace the entire body of `GetDescendants` in `backend/internal/handlers/locations/locations.go` (currently at lines 600-632). Same shape as Tasks 5–6.

```go
// @Success 200 {object} locations.ListDescendantsResponse
func (handler *Handler) GetDescendants(w http.ResponseWriter, req *http.Request) {
	ctx := middleware.GetRequestID(req.Context())
	identifier := chi.URLParam(req, "identifier")

	orgID, err := middleware.GetRequestOrgID(req)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusUnauthorized, modelerrors.ErrUnauthorized,
			apierrors.LocationGetFailed, "missing organization context", ctx)
		return
	}

	loc, err := handler.storage.GetLocationByIdentifier(req.Context(), orgID, identifier)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}
	if loc == nil {
		httputil.WriteJSONError(w, req, http.StatusNotFound, modelerrors.ErrNotFound,
			apierrors.LocationNotFound, "", ctx)
		return
	}
	id := loc.ID

	params, err := httputil.ParseListParams(req, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, req, err, ctx)
		return
	}

	results, err := handler.storage.ListDescendantsPaginated(req.Context(), orgID, id, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	total, err := handler.storage.CountDescendants(req.Context(), orgID, id)
	if err != nil {
		httputil.WriteJSONError(w, req, http.StatusInternalServerError, modelerrors.ErrInternal,
			apierrors.LocationGetFailed, err.Error(), ctx)
		return
	}

	httputil.WriteJSON(w, http.StatusOK, ListDescendantsResponse{
		Data:       toPublicLocationViews(results),
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}
```

- [ ] **Step 5: Run tests**

```
just backend test -tags=integration ./internal/handlers/locations/... -run 'Descendants'
```

Expected: PASS (new tests + existing descendants tests).

- [ ] **Step 6: Commit**

```
git add backend/internal/handlers/locations/locations.go backend/internal/handlers/locations/public_write_integration_test.go
git commit -m "feat(tra-503): paginate /locations/{id}/descendants with envelope response and stable path ordering"
```

---

## Task 8: Delete unused `LocationHierarchyResponse`

**Files:**
- Modify: `backend/internal/handlers/locations/locations.go` (delete struct around line 358)

After Tasks 5–7, no handler references `LocationHierarchyResponse` anymore. Delete it so it doesn't show up as an orphaned schema in OpenAPI.

- [ ] **Step 1: Confirm it's unused**

```
grep -rn "LocationHierarchyResponse" backend/
```

Expected: zero matches (or only the struct definition itself, which we're about to delete).

If anything still references it, fix that first — most likely a leftover swaggo `@Success` annotation. Replace with the appropriate `ListAncestorsResponse` / `ListChildrenResponse` / `ListDescendantsResponse`.

- [ ] **Step 2: Delete the struct definition**

In `backend/internal/handlers/locations/locations.go`, delete the lines defining:

```go
type LocationHierarchyResponse struct {
	Data []location.PublicLocationView `json:"data"`
}
```

(Plus its docstring comment.)

- [ ] **Step 3: Run the full handlers package test suite to confirm nothing broke**

```
just backend test -tags=integration ./internal/handlers/locations/...
```

Expected: PASS.

- [ ] **Step 4: Commit**

```
git add backend/internal/handlers/locations/locations.go
git commit -m "refactor(tra-503): remove unused LocationHierarchyResponse"
```

---

## Task 9: Handler — `ListAPIKeys` returns paginated envelope

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go` (struct at line 27; handler at line 158; swaggo at line 149)
- Test: `backend/internal/handlers/orgs/api_keys_integration_test.go` (existing tests start around line 76)

Extend `ListAPIKeysResponse` with envelope fields. Update `ListAPIKeys` handler to call `ListActiveAPIKeysPaginated` + `CountActiveAPIKeys`. The existing `CountActiveAPIKeys` is reused.

- [ ] **Step 1: Write the failing test**

Append to `backend/internal/handlers/orgs/api_keys_integration_test.go`. Helper conventions in this file (per existing tests at lines 76+): `testutil.SetupTestDB(t)` returns `(store, cleanup)`; `testutil.CreateTestAccount(t, pool)` returns `orgID`; `seedAdminUser(t, pool, orgID)` returns `(userID, sessionToken)`; `newAdminRouter(t, store)` returns the chi router. API keys are seeded via `store.CreateAPIKey(ctx, orgID, name, scopes, creator, expiresAt)`.

```go
func TestListAPIKeys_PaginationEnvelope(t *testing.T) {
	t.Setenv("JWT_SECRET", "tra503-apikeys-paginate")
	store, cleanup := testutil.SetupTestDB(t)
	defer cleanup()
	pool := store.Pool().(*pgxpool.Pool)
	orgID := testutil.CreateTestAccount(t, pool)
	userID, sessionToken := seedAdminUser(t, pool, orgID)

	for i := 0; i < 3; i++ {
		_, err := store.CreateAPIKey(context.Background(), orgID,
			fmt.Sprintf("p503-k%d", i),
			[]string{"assets:read"},
			apikey.Creator{UserID: &userID},
			nil)
		require.NoError(t, err)
		time.Sleep(2 * time.Millisecond)
	}

	r := newAdminRouter(t, store)
	url := fmt.Sprintf("/api/v1/orgs/%d/api-keys?limit=2&offset=0", orgID)
	req := httptest.NewRequest(http.MethodGet, url, nil)
	req.Header.Set("Authorization", "Bearer "+sessionToken)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.EqualValues(t, 2, body["limit"])
	assert.EqualValues(t, 0, body["offset"])
	assert.EqualValues(t, 3, body["total_count"])
	data := body["data"].([]any)
	require.Len(t, data, 2)
}
```

- [ ] **Step 2: Run test to verify it fails**

```
just backend test -tags=integration ./internal/handlers/orgs/... -run TestListAPIKeys_PaginationEnvelope
```

Expected: FAIL — `limit` and `total_count` keys missing from current bare response.

- [ ] **Step 3: Extend `ListAPIKeysResponse` struct**

In `backend/internal/handlers/orgs/api_keys.go`, replace the existing struct (lines 25-29):

```go
// ListAPIKeysResponse is the typed envelope returned by
// GET /api/v1/orgs/{id}/api-keys.
type ListAPIKeysResponse struct {
	Data       []apikey.APIKeyListItem `json:"data"`
	Limit      int                     `json:"limit"       example:"50"`
	Offset     int                     `json:"offset"      example:"0"`
	TotalCount int                     `json:"total_count" example:"100"`
}
```

- [ ] **Step 4: Update `ListAPIKeys` handler**

Replace lines 158-189 of `backend/internal/handlers/orgs/api_keys.go`:

```go
// ListAPIKeys handles GET /api/v1/orgs/{id}/api-keys.
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
	reqID := middleware.GetRequestID(r.Context())
	orgID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusBadRequest, modelerrors.ErrBadRequest,
			"Invalid org id", "", reqID)
		return
	}

	params, err := httputil.ParseListParams(r, httputil.ListAllowlist{})
	if err != nil {
		httputil.RespondListParamError(w, r, err, reqID)
		return
	}

	keys, err := h.storage.ListActiveAPIKeysPaginated(r.Context(), orgID, params.Limit, params.Offset)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to list api keys", "", reqID)
		return
	}

	total, err := h.storage.CountActiveAPIKeys(r.Context(), orgID)
	if err != nil {
		httputil.WriteJSONError(w, r, http.StatusInternalServerError, modelerrors.ErrInternal,
			"Failed to count api keys", "", reqID)
		return
	}

	items := make([]apikey.APIKeyListItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, apikey.APIKeyListItem{
			ID:             k.ID,
			JTI:            k.JTI,
			Name:           k.Name,
			Scopes:         k.Scopes,
			CreatedBy:      k.CreatedBy,
			CreatedByKeyID: k.CreatedByKeyID,
			CreatedAt:      k.CreatedAt,
			ExpiresAt:      k.ExpiresAt,
			LastUsedAt:     k.LastUsedAt,
		})
	}

	httputil.WriteJSON(w, http.StatusOK, ListAPIKeysResponse{
		Data:       items,
		Limit:      params.Limit,
		Offset:     params.Offset,
		TotalCount: total,
	})
}
```

(swaggo annotation at line 149 already points at `orgs.ListAPIKeysResponse` — no change needed there.)

- [ ] **Step 5: Run tests**

```
just backend test -tags=integration ./internal/handlers/orgs/... -run 'APIKey'
```

Expected: PASS for the new pagination test AND all existing api-key tests (they should continue to work since the envelope is additive — the `data` field is in the same place).

- [ ] **Step 6: Commit**

```
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/orgs/api_keys_integration_test.go
git commit -m "feat(tra-503): paginate /orgs/{id}/api-keys with envelope response"
```

---

## Task 10: Cleanup — `assets` and `locations` handlers return typed structs

**Files:**
- Modify: `backend/internal/handlers/assets/assets.go` (line 493 — `ListAssets` handler's return)
- Modify: `backend/internal/handlers/locations/locations.go` (line 437 — `ListLocations` handler's return)

Both handlers currently build `map[string]any{...}`. The named structs (`ListAssetsResponse`, `ListLocationsResponse`) already exist with matching fields. Swap the map literal for a struct value. **No new tests** — existing tests must pass unchanged (that's the regression check; runtime JSON output is byte-identical).

- [ ] **Step 1: Replace `map[string]any` with `ListAssetsResponse` in assets handler**

In `backend/internal/handlers/assets/assets.go`, find the `httputil.WriteJSON(w, http.StatusOK, map[string]any{...})` call at line 493 and replace the map literal with:

```go
httputil.WriteJSON(w, http.StatusOK, ListAssetsResponse{
	Data:       items, // whatever variable name the handler uses for the slice
	Limit:      params.Limit,
	Offset:     params.Offset,
	TotalCount: total,
})
```

The variable names (`items`, `params`, `total`) must match what the existing handler already uses — read lines 437-499 first to confirm. Don't rename anything.

- [ ] **Step 2: Replace `map[string]any` with `ListLocationsResponse` in locations handler**

In `backend/internal/handlers/locations/locations.go`, line 437, same swap with `ListLocationsResponse`.

- [ ] **Step 3: Run all assets + locations tests**

```
just backend test -tags=integration ./internal/handlers/assets/... ./internal/handlers/locations/...
```

Expected: PASS (zero changes; existing tests continue to assert the same JSON keys).

- [ ] **Step 4: Commit**

```
git add backend/internal/handlers/assets/assets.go backend/internal/handlers/locations/locations.go
git commit -m "refactor(tra-503): return typed envelope structs from /assets and /locations"
```

---

## Task 11: Cleanup — `current_locations` typed struct + drop dead constants

**Files:**
- Modify: `backend/internal/handlers/reports/current_locations.go`

Two changes:
1. Delete the unused `defaultLimit = 50` and `maxLimit = 100` constants at lines 15-18 (declared but never referenced — the handler already uses `httputil.ParseListParams` with the global 50/200 limits).
2. Replace the `map[string]any` literal at line 105 with `ListCurrentLocationsResponse{...}`.

- [ ] **Step 1: Sanity-check that the constants are truly unused**

```
grep -n "defaultLimit\|maxLimit" backend/internal/handlers/reports/current_locations.go
```

Expected: only the two declaration lines (15-17) match. If anything else matches, stop and read the file before deleting.

- [ ] **Step 2: Delete the const block (lines 15-18)**

Delete from `backend/internal/handlers/reports/current_locations.go`:

```go
const (
	defaultLimit = 50
	maxLimit     = 100
)
```

- [ ] **Step 3: Replace `map[string]any` with `ListCurrentLocationsResponse`**

In `backend/internal/handlers/reports/current_locations.go`, line 105, replace:

```go
httputil.WriteJSON(w, http.StatusOK, map[string]any{
	"data":        out,
	"limit":       params.Limit,
	"offset":      params.Offset,
	"total_count": total,
})
```

with:

```go
httputil.WriteJSON(w, http.StatusOK, ListCurrentLocationsResponse{
	Data:       out,
	Limit:      params.Limit,
	Offset:     params.Offset,
	TotalCount: total,
})
```

- [ ] **Step 4: Run reports tests**

```
just backend test -tags=integration ./internal/handlers/reports/...
```

Expected: PASS (no tests should break — constants were dead, JSON output is unchanged).

- [ ] **Step 5: Commit**

```
git add backend/internal/handlers/reports/current_locations.go
git commit -m "refactor(tra-503): /locations/current returns typed envelope and drops dead limit constants"
```

---

## Task 12: Cleanup — `asset_history` typed struct + drop dead constants

**Files:**
- Modify: `backend/internal/handlers/reports/asset_history.go`

Mirror Task 11. The `assetHistoryDefaultLimit = 50` and `assetHistoryMaxLimit = 100` constants at lines 17-19 are also declared but unused. Delete them, then swap `map[string]any` for `AssetHistoryResponse{...}` at line 135.

- [ ] **Step 1: Sanity-check that the constants are truly unused**

```
grep -n "assetHistoryDefaultLimit\|assetHistoryMaxLimit" backend/internal/handlers/reports/asset_history.go
```

Expected: only the declaration lines (17-19) match.

- [ ] **Step 2: Delete the const block (lines 17-19)**

Delete from `backend/internal/handlers/reports/asset_history.go`:

```go
const (
	assetHistoryDefaultLimit = 50
	assetHistoryMaxLimit     = 100
)
```

- [ ] **Step 3: Replace `map[string]any` with `AssetHistoryResponse`**

In `backend/internal/handlers/reports/asset_history.go`, line 135, replace:

```go
httputil.WriteJSON(w, http.StatusOK, map[string]any{
	"data":        out,
	"limit":       params.Limit,
	"offset":      params.Offset,
	"total_count": total,
})
```

with:

```go
httputil.WriteJSON(w, http.StatusOK, AssetHistoryResponse{
	Data:       out,
	Limit:      params.Limit,
	Offset:     params.Offset,
	TotalCount: total,
})
```

(Variable names `out`, `params`, `total` should match what the existing handler uses — confirm by reading lines 67-141 first.)

- [ ] **Step 4: Run reports tests**

```
just backend test -tags=integration ./internal/handlers/reports/...
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add backend/internal/handlers/reports/asset_history.go
git commit -m "refactor(tra-503): /assets/{id}/history returns typed envelope and drops dead limit constants"
```

---

## Task 13: Regenerate OpenAPI spec

**Files:**
- Modify: `backend/internal/openapi/openapi.json` (regenerated; do not hand-edit)

The api-spec recipe regenerates `openapi.json` from swaggo annotations and named structs. After Tasks 5–12, the spec needs to reflect: three new schemas (`ListAncestorsResponse`, `ListChildrenResponse`, `ListDescendantsResponse`), one extended schema (`ListAPIKeysResponse` gains envelope fields), and one removed schema (`LocationHierarchyResponse`).

- [ ] **Step 1: Run the api-spec recipe**

```
just backend api-spec
```

The recipe self-heals the `frontend/dist` stub per commit `63ef61e` — no manual prep needed. Expected: command succeeds; `backend/internal/openapi/openapi.json` is updated.

- [ ] **Step 2: Sanity-check the diff**

```
git diff backend/internal/openapi/openapi.json | head -100
```

Confirm the diff includes:
- `ListAncestorsResponse`, `ListChildrenResponse`, `ListDescendantsResponse` added.
- `ListAPIKeysResponse` gained `limit`, `offset`, `total_count` properties.
- `LocationHierarchyResponse` removed.
- The `@Success` references on `GET /locations/{id}/ancestors|children|descendants` and `/orgs/{id}/api-keys` now point at the new schemas.

If anything looks wrong (e.g., `LocationHierarchyResponse` still present), it means a swaggo annotation wasn't updated in Tasks 5–8. Fix it and re-run `just backend api-spec`.

- [ ] **Step 3: Commit the regenerated spec**

```
git add backend/internal/openapi/openapi.json
git commit -m "chore(tra-503): regenerate OpenAPI spec for paginated hierarchy and api-keys endpoints"
```

---

## Task 14: Final verification — full backend test sweep

**Files:** none (verification only)

- [ ] **Step 1: Full test run**

```
just backend test
just backend test -tags=integration
```

Expected: PASS for both. No skipped tests beyond pre-existing skips.

- [ ] **Step 2: Lint + build**

```
just lint
just backend build
```

Expected: clean.

- [ ] **Step 3: If any test fails, debug and fix in place**

If a failure traces to an existing test that checks `limit=150 -> 400` (no longer valid since we standardized to max=200), update the test's bad-input value. If it traces to a test checking the old bare-array shape on the converted endpoints, update the test to assert the envelope shape. Don't proceed until everything is green.

- [ ] **Step 4: Push the branch (no PR yet — that's a separate user action)**

```
git push -u origin worktree-tra-503
```

The branch is ready for PR. The user will create the PR manually per the project's "always PR, never merge locally" rule.
