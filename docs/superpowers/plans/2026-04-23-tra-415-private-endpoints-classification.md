# TRA-415 Private-Endpoints Classification — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Classify the 11 endpoints listed in `docs/api/private-endpoints.md` (plus ~6 adjacent unannotated orgs/members/invitations routes) as public or internal, add swaggo annotations so the partition tool produces a complete spec, normalize `GET /orgs/me` to the standard `{"data": ...}` envelope, and rewrite the docs page to reflect reality.

**Architecture:** Two PRs landed in sequence. Platform PR adds `// @Tags <resource>,internal` (or `,public` for `/orgs/me`) swaggo annotations to orgs/members/invitations handlers that currently have none, changes `GetOrgMe` to wrap its response in `{"data": {...}}`, and regenerates `openapi.public.*` + `openapi.internal.*`. Docs PR (in a separate `trakrf-docs` checkout) rewrites the table, fixes the GET→POST method error, updates the response-shape note, and replaces the "decisions to come" placeholder section with a short classification policy.

**Tech Stack:** Go backend, swaggo (`swag init`), custom `apispec` partition tool, chi router, pgx. Docs: Docusaurus markdown.

**Design doc:** `docs/superpowers/specs/2026-04-23-tra-415-private-endpoints-classification-design.md`

**Branches:**
- Platform: `miks2u/tra-415-private-endpoints-classification` (already on it in `/home/mike/platform`)
- Docs: `miks2u/tra-415-classify-private-endpoints` (separate checkout at `/home/mike/trakrf-docs-tra-415` per project convention — don't operate in `/home/mike/trakrf-docs`)

---

## Phase 1 — Platform: `/orgs/me` shape normalization (TDD)

### Task 1: Update shape test to expect enveloped response

**Files:**
- Modify: `backend/internal/handlers/orgs/public_integration_test.go:55-59`

- [ ] **Step 1: Rewrite the happy-path assertion**

Replace lines 55-59 of `backend/internal/handlers/orgs/public_integration_test.go`:

```go
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var body map[string]any
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	data, ok := body["data"].(map[string]any)
	require.True(t, ok, "expected top-level `data` object, got %s", w.Body.String())
	assert.Equal(t, float64(orgID), data["id"])
	assert.Equal(t, "Test Organization", data["name"])
	assert.NotContains(t, body, "id", "bare-object shape must be gone")
	assert.NotContains(t, body, "name", "bare-object shape must be gone")
```

- [ ] **Step 2: Run test — verify it fails**

```bash
just backend test-integration ./internal/handlers/orgs/... -run TestGetOrgMe_ValidAPIKey
```

Expected: FAIL. Body does not contain key `data`; current response is the bare `{"id":..., "name":...}` shape.

### Task 2: Change the handler to emit the envelope

**Files:**
- Modify: `backend/internal/handlers/orgs/public.go:36-39`

- [ ] **Step 1: Wrap the response in `data`**

Replace lines 36-39 of `backend/internal/handlers/orgs/public.go`:

```go
	httputil.WriteJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"id":   org.ID,
			"name": org.Name,
		},
	})
```

- [ ] **Step 2: Re-run the shape test — verify it passes**

```bash
just backend test-integration ./internal/handlers/orgs/... -run TestGetOrgMe_ValidAPIKey
```

Expected: PASS.

- [ ] **Step 3: Run the rest of the orgs integration tests — nothing else broke**

```bash
just backend test-integration ./internal/handlers/orgs/...
```

Expected: all existing tests PASS (this change only affects the happy-path shape; error paths use `WriteJSONError` which already uses the `{"error":...}` envelope and is untouched).

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/orgs/public.go backend/internal/handlers/orgs/public_integration_test.go
git commit -m "$(cat <<'EOF'
fix(tra-415): normalize /orgs/me response to {"data":...} envelope

Was the one endpoint on the public surface emitting a bare object
instead of the {"data":...} envelope every other endpoint uses.
Normalize before /orgs/me lands in the published OpenAPI spec so
integrators see a uniform contract.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 2 — Platform: Swaggo annotations

Swaggo annotations are compile-time metadata; `swag init` extracts them from `// @`-prefixed comments directly above each handler function. All annotations share a common pattern:

- `@Summary` — one-line human description.
- `@Tags <resource>,<public|internal>` — resource tag drives Redoc grouping; discriminator tag drives partition.
- `@ID <resource>.<verb>` — canonical operation id for codegen tools (`assets.list`, `orgs.get`, etc.).
- `@Security BearerAuth` — session-JWT-authenticated internal endpoints. (APIKey is for the public surface.) Auth endpoints like `/login` are unauthenticated — no `@Security` line.
- `@Router /api/v1/... [method]` — must match the chi route exactly.
- `@Accept json` / `@Produce json` — always json.
- `@Param request body X true "..."` — request body schema (for POST/PUT).
- `@Param id path int true "Organization ID"` — path params.
- `@Success <code> {object} <schema>` / `@Failure <code> {object} modelerrors.ErrorResponse "..."` — response schemas.

The security-scheme names `APIKey` and `BearerAuth` are defined globally in `backend/main.go:11-18`; no additional changes there.

### Task 3: Annotate `handlers/orgs/public.go` — `GetOrgMe` (public, API-key)

**Files:**
- Modify: `backend/internal/handlers/orgs/public.go:11-14`

- [ ] **Step 1: Insert annotation block above `GetOrgMe`**

Replace the existing comment block at lines 11-14 (`// GetOrgMe returns the org ... before TRA-396 lands.`) with:

```go
// @Summary Get the org associated with the authenticated API key
// @Description Returns the organization scoped by the API key presented in Authorization. Intended as a lightweight health-check primitive for integrators verifying a key works end-to-end.
// @Tags orgs,public
// @ID orgs.me
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "data: {id, name}"
// @Failure 401 {object} modelerrors.ErrorResponse "Unauthorized"
// @Failure 500 {object} modelerrors.ErrorResponse "Internal server error"
// @Security APIKey
// @Router /api/v1/orgs/me [get]
// GetOrgMe returns the org that the authenticated API key belongs to.
// Scoped to API-key auth (not session auth); serves as the canary endpoint
// customers hit to verify a key works end-to-end.
func (h *Handler) GetOrgMe(w http.ResponseWriter, r *http.Request) {
```

Note: the orgs handlers import `models/errors` under the alias `modelerrors` (they also use stdlib `"errors"`, hence the alias). Swaggo resolves type refs by the in-source package name, so the annotation uses `modelerrors.ErrorResponse`, not `errors.ErrorResponse`. No new imports are needed — the alias is already in scope in every file we're touching.

- [ ] **Step 2: Regenerate specs and verify `/orgs/me` lands in the public spec**

```bash
just backend api-spec
python3 -c "import json; d=json.load(open('docs/api/openapi.public.json')); \
  assert '/api/v1/orgs/me' in d['paths'], 'missing'; \
  print(json.dumps(d['paths']['/api/v1/orgs/me']['get']['responses']['200'], indent=2))"
```

Expected: prints the 200 response schema; no assertion error. Confirms the handler was picked up and partitioned into the public spec.

- [ ] **Step 3: Commit**

```bash
git add backend/internal/handlers/orgs/public.go docs/api/openapi.public.json docs/api/openapi.public.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): publish /orgs/me in OpenAPI spec

Adds swaggo annotations so the API-key health-check endpoint
appears in openapi.public.* alongside the rest of the public
surface.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 4: Annotate `handlers/orgs/orgs.go` — list/create/get/update/delete (internal)

**Files:**
- Modify: `backend/internal/handlers/orgs/orgs.go:30`, `:49`, `:87`, `:112`, `:150`

Five handlers. For each, insert an annotation block immediately above the `func (h *Handler) <Name>(...)` line. Keep the existing one-line `// Name returns ...` doc comment below the annotation block.

- [ ] **Step 1: `List` (line 30-31)**

```go
// @Summary List organizations the authenticated user belongs to
// @Tags orgs,internal
// @ID orgs.list
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "data: []organization.Organization"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs [get]
// List returns all organizations the authenticated user belongs to.
func (h *Handler) List(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 2: `Create` (line 49-50)**

```go
// @Summary Create a new organization
// @Description Creates a team organization with the caller as admin. SPA-only — integrators have a fixed org scoped to their API key.
// @Tags orgs,internal
// @ID orgs.create
// @Accept json
// @Produce json
// @Param request body organization.CreateOrganizationRequest true "Organization to create"
// @Success 201 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Identifier already taken"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs [post]
// Create creates a new team organization with the creator as admin.
func (h *Handler) Create(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 3: `Get` (line 87-88)**

```go
// @Summary Get an organization by id
// @Tags orgs,internal
// @ID orgs.get
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [get]
// Get returns a single organization by ID.
func (h *Handler) Get(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 4: `Update` (line 112-113)**

```go
// @Summary Update an organization's name
// @Tags orgs,internal
// @ID orgs.update
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body organization.UpdateOrganizationRequest true "Update payload"
// @Success 200 {object} map[string]any "data: organization.Organization"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [put]
// Update updates an organization's name.
func (h *Handler) Update(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 5: `Delete` (line 150-151)**

```go
// @Summary Soft-delete an organization
// @Description Requires the caller to repeat the organization name as a confirmation in the request body.
// @Tags orgs,internal
// @ID orgs.delete
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body organization.DeleteOrganizationRequest true "Confirmation payload"
// @Success 200 {object} map[string]any "message: Organization deleted"
// @Failure 400 {object} modelerrors.ErrorResponse "Name mismatch or invalid id"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id} [delete]
// Delete soft-deletes an organization after confirming the name matches.
func (h *Handler) Delete(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 6: Regenerate + verify all five land in the internal spec**

```bash
just backend api-spec
python3 -c "
import json
d = json.load(open('backend/internal/handlers/swaggerspec/openapi.internal.json'))
paths = d['paths']
for p, methods in [('/api/v1/orgs', ['get','post']), ('/api/v1/orgs/{id}', ['get','put','delete'])]:
    assert p in paths, f'missing path {p}'
    for m in methods:
        assert m in paths[p], f'missing {m} on {p}'
print('orgs.go: all five operations present in internal spec')"
```

Expected: `orgs.go: all five operations present in internal spec`.

- [ ] **Step 7: Commit**

```bash
git add backend/internal/handlers/orgs/orgs.go backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): annotate orgs list/create/get/update/delete as internal

Adds swaggo tags so the internal OpenAPI spec reflects the actual
shape of the org CRUD surface the SPA depends on.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 5: Annotate `handlers/orgs/me.go` — `GetMe`, `SetCurrentOrg` (internal)

**Files:**
- Modify: `backend/internal/handlers/orgs/me.go:18`, `:37`

- [ ] **Step 1: `GetMe` — `GET /api/v1/users/me`**

Replace the existing `// GetMe returns...` comment at line 18 with:

```go
// @Summary Get the authenticated user's profile with org memberships
// @Description Returns the caller's user record alongside the organizations they belong to. Used by the SPA to render the user menu and org picker.
// @Tags users,internal
// @ID users.me
// @Accept json
// @Produce json
// @Success 200 {object} map[string]any "data: user profile"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users/me [get]
// GetMe returns the authenticated user's profile with orgs.
func (h *Handler) GetMe(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 2: `SetCurrentOrg` — `POST /api/v1/users/me/current-org`**

Replace the existing `// SetCurrentOrg updates...` comment at line 37 with:

```go
// @Summary Switch the authenticated user's current organization
// @Description SPA org-switcher. Issues a fresh session JWT scoped to the selected org. API-key auth has a fixed org — no analog exists for integrators. Note: route is POST (not GET as some earlier docs suggested).
// @Tags users,internal
// @ID users.set_current_org
// @Accept json
// @Produce json
// @Param request body organization.SetCurrentOrgRequest true "Org to switch to"
// @Success 200 {object} map[string]any "message + fresh token"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse "Not a member of the target org"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/users/me/current-org [post]
// SetCurrentOrg updates the user's current organization.
func (h *Handler) SetCurrentOrg(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 3: Regenerate + verify both present**

```bash
just backend api-spec
python3 -c "
import json
d = json.load(open('backend/internal/handlers/swaggerspec/openapi.internal.json'))
paths = d['paths']
assert 'get' in paths['/api/v1/users/me'], 'users.me missing'
assert 'post' in paths['/api/v1/users/me/current-org'], 'users.set_current_org missing'
print('me.go: both operations present')"
```

Expected: `me.go: both operations present`.

- [ ] **Step 4: Commit**

```bash
git add backend/internal/handlers/orgs/me.go backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): annotate /users/me and /users/me/current-org as internal

Both routes are SPA-only; API-key auth is scoped to a single fixed
org so no integrator analog exists. Corrects a prior docs table that
listed the org-switcher as GET — actual route is POST.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 6: Annotate `handlers/orgs/api_keys.go` — list/create/revoke (internal)

**Files:**
- Modify: `backend/internal/handlers/orgs/api_keys.go:18`, `:94`, `:126`

These are session-JWT-only (an API-key JWT cannot mint or revoke other API keys). Documentation of this constraint is in the docs PR, not here — the swaggo annotation only declares `@Security BearerAuth`.

- [ ] **Step 1: `CreateAPIKey`**

Replace the comment at line 18:

```go
// @Summary Create a new API key for an organization
// @Description Mints an API-key JWT scoped to the target org. Session-JWT-only — API-key tokens are rejected with 401.
// @Tags api-keys,internal
// @ID api_keys.create
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body apikey.CreateAPIKeyRequest true "Key creation payload"
// @Success 201 {object} apikey.APIKeyCreateResponse
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Active-key cap reached"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys [post]
// CreateAPIKey handles POST /api/v1/orgs/{id}/api-keys.
func (h *Handler) CreateAPIKey(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 2: `ListAPIKeys`**

Replace the comment at line 94:

```go
// @Summary List active API keys for an organization
// @Tags api-keys,internal
// @ID api_keys.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: []apikey.APIKeyListItem"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys [get]
// ListAPIKeys handles GET /api/v1/orgs/{id}/api-keys.
func (h *Handler) ListAPIKeys(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 3: `RevokeAPIKey`**

Replace the comment at line 126:

```go
// @Summary Revoke an API key
// @Tags api-keys,internal
// @ID api_keys.revoke
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param keyID path int true "API key id"
// @Success 204 "No Content"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/api-keys/{keyID} [delete]
// RevokeAPIKey handles DELETE /api/v1/orgs/{id}/api-keys/{keyID}.
func (h *Handler) RevokeAPIKey(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 4: Regenerate + verify**

```bash
just backend api-spec
python3 -c "
import json
d = json.load(open('backend/internal/handlers/swaggerspec/openapi.internal.json'))
paths = d['paths']
assert 'post' in paths['/api/v1/orgs/{id}/api-keys'], 'create missing'
assert 'get' in paths['/api/v1/orgs/{id}/api-keys'], 'list missing'
assert 'delete' in paths['/api/v1/orgs/{id}/api-keys/{keyID}'], 'revoke missing'
print('api_keys.go: all three operations present')"
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/orgs/api_keys.go backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): annotate org api-keys CRUD as internal

Session-JWT-only; an API-key token cannot mint or revoke other keys
by design. Future API-key-authenticated rotation primitive is a
separate ticket if demand appears.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 7: Annotate `handlers/orgs/members.go` — list/update/remove (internal)

**Files:**
- Modify: `backend/internal/handlers/orgs/members.go:17`, `:36`, `:92`

- [ ] **Step 1: `ListMembers`**

Replace the comment at line 17:

```go
// @Summary List members of an organization
// @Tags org-members,internal
// @ID org_members.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: []organization.Member"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members [get]
// ListMembers returns all members of an organization.
func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 2: `UpdateMemberRole`**

Replace the comment at line 36:

```go
// @Summary Update a member's role in an organization
// @Tags org-members,internal
// @ID org_members.update_role
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param userId path int true "User id"
// @Param request body organization.UpdateMemberRoleRequest true "New role"
// @Success 200 {object} map[string]any "message: Role updated"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members/{userId} [put]
// UpdateMemberRole updates a member's role in an organization.
func (h *Handler) UpdateMemberRole(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 3: `RemoveMember`**

Replace the comment at line 92:

```go
// @Summary Remove a member from an organization
// @Tags org-members,internal
// @ID org_members.remove
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param userId path int true "User id"
// @Success 200 {object} map[string]any "message: Member removed"
// @Failure 400 {object} modelerrors.ErrorResponse "Self-removal or last-admin"
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/members/{userId} [delete]
// RemoveMember removes a member from an organization.
func (h *Handler) RemoveMember(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 4: Regenerate + verify**

```bash
just backend api-spec
python3 -c "
import json
d = json.load(open('backend/internal/handlers/swaggerspec/openapi.internal.json'))
paths = d['paths']
assert 'get' in paths['/api/v1/orgs/{id}/members'], 'list missing'
assert 'put' in paths['/api/v1/orgs/{id}/members/{userId}'], 'update missing'
assert 'delete' in paths['/api/v1/orgs/{id}/members/{userId}'], 'remove missing'
print('members.go: all three operations present')"
```

- [ ] **Step 5: Commit**

```bash
git add backend/internal/handlers/orgs/members.go backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): annotate org member management as internal

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 8: Annotate `handlers/orgs/invitations.go` — list/create/cancel/resend (internal)

**Files:**
- Modify: `backend/internal/handlers/orgs/invitations.go:17`, `:36`, `:91`, `:114`

- [ ] **Step 1: `ListInvitations`**

Replace the comment at line 17:

```go
// @Summary List pending invitations for an organization
// @Tags org-invitations,internal
// @ID org_invitations.list
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Success 200 {object} map[string]any "data: []organization.Invitation"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/invitations [get]
// ListInvitations returns pending invitations for an organization.
func (h *Handler) ListInvitations(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 2: `CreateInvitation`**

Replace the comment at line 36:

```go
// @Summary Create an invitation and send it by email
// @Tags org-invitations,internal
// @ID org_invitations.create
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param request body organization.CreateInvitationRequest true "Invitation payload"
// @Success 201 {object} map[string]any "data: organization.Invitation"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 409 {object} modelerrors.ErrorResponse "Already invited or member"
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/invitations [post]
// CreateInvitation creates an invitation and sends an email.
func (h *Handler) CreateInvitation(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 3: `CancelInvitation`**

Replace the comment at line 91:

```go
// @Summary Cancel a pending invitation
// @Tags org-invitations,internal
// @ID org_invitations.cancel
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param inviteId path int true "Invitation id"
// @Success 200 {object} map[string]any "message: Invitation cancelled"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/invitations/{inviteId} [delete]
// CancelInvitation cancels a pending invitation.
func (h *Handler) CancelInvitation(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 4: `ResendInvitation`**

Replace the comment at line 114:

```go
// @Summary Re-send a pending invitation email
// @Tags org-invitations,internal
// @ID org_invitations.resend
// @Accept json
// @Produce json
// @Param id path int true "Organization id"
// @Param inviteId path int true "Invitation id"
// @Success 200 {object} map[string]any "message: Invitation resent"
// @Failure 400 {object} modelerrors.ErrorResponse
// @Failure 401 {object} modelerrors.ErrorResponse
// @Failure 403 {object} modelerrors.ErrorResponse
// @Failure 404 {object} modelerrors.ErrorResponse
// @Failure 500 {object} modelerrors.ErrorResponse
// @Security BearerAuth
// @Router /api/v1/orgs/{id}/invitations/{inviteId}/resend [post]
// ResendInvitation re-sends the invitation email.
func (h *Handler) ResendInvitation(w http.ResponseWriter, r *http.Request) {
```

- [ ] **Step 5: Regenerate + verify**

```bash
just backend api-spec
python3 -c "
import json
d = json.load(open('backend/internal/handlers/swaggerspec/openapi.internal.json'))
paths = d['paths']
assert 'get' in paths['/api/v1/orgs/{id}/invitations'], 'list missing'
assert 'post' in paths['/api/v1/orgs/{id}/invitations'], 'create missing'
assert 'delete' in paths['/api/v1/orgs/{id}/invitations/{inviteId}'], 'cancel missing'
assert 'post' in paths['/api/v1/orgs/{id}/invitations/{inviteId}/resend'], 'resend missing'
print('invitations.go: all four operations present')"
```

- [ ] **Step 6: Commit**

```bash
git add backend/internal/handlers/orgs/invitations.go backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml
git commit -m "$(cat <<'EOF'
feat(tra-415): annotate org invitation management as internal

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

---

## Phase 3 — Platform: validate and ship

### Task 9: Full validation pass

- [ ] **Step 1: Run the partition test suite**

```bash
just backend test ./internal/tools/apispec/...
```

Expected: all tests PASS. This confirms the partition tool still cleanly splits every annotated handler into public or internal with no violations.

- [ ] **Step 2: Run the full platform test suite**

```bash
just test
```

Expected: all tests PASS (both backend + frontend, per root justfile).

- [ ] **Step 3: Run the Redocly linter on the public spec**

```bash
just backend api-lint
```

Expected: no new warnings or errors introduced by `/orgs/me`. Existing warnings, if any, are pre-existing and not in this ticket's scope.

- [ ] **Step 4: Final smoke — diff the generated specs vs. `origin/main` and eyeball**

```bash
git diff origin/main -- docs/api/openapi.public.json docs/api/openapi.public.yaml
git diff origin/main -- backend/internal/handlers/swaggerspec/openapi.internal.json backend/internal/handlers/swaggerspec/openapi.internal.yaml | head -100
```

Expected diff summary:
- Public spec: one added path (`/api/v1/orgs/me`), no removed paths, no modifications to existing paths.
- Internal spec: 17 added operations across 11 paths (orgs/\*, users/me/\*, members, invitations, api-keys); no removed or modified operations.

### Task 10: Push and open the platform PR

- [ ] **Step 1: Push the branch**

```bash
git push -u origin miks2u/tra-415-private-endpoints-classification
```

- [ ] **Step 2: Open the PR**

```bash
gh pr create --title "feat(tra-415): classify /orgs/me public; annotate SPA routes internal" --body "$(cat <<'EOF'
## Summary

Resolves every row in `docs/api/private-endpoints.md` via swaggo tags.

- \`GET /api/v1/orgs/me\` → public. Now in \`openapi.public.*\`. Response normalized from bare \`{id, name}\` to \`{"data": {...}}\` for envelope parity with the rest of the public surface.
- \`POST /api/v1/auth/{login,signup,forgot-password,reset-password,accept-invite}\` → already tagged internal, unchanged.
- \`GET /api/v1/users/me\`, \`POST /api/v1/users/me/current-org\`, \`GET|POST /api/v1/orgs\`, \`GET|PUT|DELETE /api/v1/orgs/{id}\`, \`GET|POST|DELETE /api/v1/orgs/{id}/api-keys\` → internal.
- Adjacent SPA-only routes picked up in the same sweep so the internal spec is actually complete: members (GET/PUT/DELETE), invitations (GET/POST/DELETE/resend).

Docs PR (separate, in trakrf-docs repo) rewrites \`docs/api/private-endpoints.md\` once this is on preview.

## Test plan

- [ ] CI green
- [ ] \`curl -H "Authorization: Bearer <api-key>" https://app.preview.trakrf.id/api/v1/orgs/me\` returns \`{"data": {...}}\`
- [ ] Preview redoc at \`/api\` shows the new \`/orgs/me\` entry

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Phase 4 — Docs: rewrite `private-endpoints.md`

**Precondition:** Phase 3 PR merged and deployed to `app.preview.trakrf.id`. Verify before starting:

```bash
curl -fsS https://app.preview.trakrf.id/api/v1/openapi.json | jq '.paths["/api/v1/orgs/me"].get.operationId'
```

Expected: `"orgs.me"`. If null or 404, preview hasn't rolled over yet — wait.

### Task 11: Set up a sibling checkout of `trakrf-docs`

**Files:** new working directory at `/home/mike/trakrf-docs-tra-415/`.

- [ ] **Step 1: Clone a fresh sibling checkout**

```bash
cd /home/mike
git clone git@github.com:trakrf/docs.git trakrf-docs-tra-415
cd trakrf-docs-tra-415
git checkout -b miks2u/tra-415-classify-private-endpoints
```

Note: per the project convention, don't operate in `/home/mike/trakrf-docs` — that's the main checkout and is for browsing/reading. Dedicated ticket work goes in a sibling `trakrf-docs-<ticket>` directory.

- [ ] **Step 2: Install deps (one-time, for the linter step later)**

```bash
pnpm install
```

### Task 12: Rewrite the table

**Files:**
- Modify: `/home/mike/trakrf-docs-tra-415/docs/api/private-endpoints.md:15-27`

- [ ] **Step 1: Replace the pending-only table with the classified table**

Open `docs/api/private-endpoints.md` and replace lines 15-27 (the existing table block, including the header separator) with:

```markdown
| Endpoint                       | Method(s)         | Used by                | Status                           | Classification                            |
| ------------------------------ | ----------------- | ---------------------- | -------------------------------- | ----------------------------------------- |
| `/api/v1/auth/login`           | POST              | SPA login form         | Internal                         | Internal                                  |
| `/api/v1/auth/signup`          | POST              | SPA signup form        | Internal                         | Internal                                  |
| `/api/v1/auth/forgot-password` | POST              | SPA password recovery  | Internal                         | Internal                                  |
| `/api/v1/auth/reset-password`  | POST              | SPA password recovery  | Internal                         | Internal                                  |
| `/api/v1/auth/accept-invite`   | POST              | SPA invite acceptance  | Internal                         | Internal                                  |
| `/api/v1/users/me`             | GET               | SPA user context       | Internal                         | Internal                                  |
| `/api/v1/users/me/current-org` | POST              | SPA org switcher       | Internal                         | Internal                                  |
| `/api/v1/orgs`                 | GET               | SPA org picker         | Internal                         | Internal                                  |
| `/api/v1/orgs/{id}`            | GET               | SPA org detail         | Internal                         | Internal                                  |
| `/api/v1/orgs/{id}/api-keys`   | GET, POST, DELETE | Settings → API Keys UI | Internal                         | Internal — see API-key note below         |
| `/api/v1/orgs/me`              | GET               | API-key health check   | Public (see [`/api`](/api))      | Public                                    |
```

Note: the previous table listed `/users/me/current-org` as `GET`. It's `POST` — the handler decodes a JSON body with the target org id. The `Method(s)` column is now correct.

### Task 13: Rewrite the "response-shape note" section for `/orgs/me`

**Files:**
- Modify: `/home/mike/trakrf-docs-tra-415/docs/api/private-endpoints.md:29-42`

- [ ] **Step 1: Replace the section with shape-is-now-enveloped prose**

Replace lines 29-42 (the entire `## Response-shape note: /orgs/me {#orgs-me}` section) with:

```markdown
## Response shape: `/orgs/me` {#orgs-me}

`GET /api/v1/orgs/me` is excluded from rate limiting (see [Rate limits → Exclusions](./rate-limits#exclusions)) and is commonly used as an API-key liveness probe. It uses the same `{"data": ...}` envelope as every other endpoint on the public surface:

```json
{
  "data": {
    "id": 123,
    "name": "Example Org"
  }
}
```

If you're using `/orgs/me` as a health check, consider also probing a "real" endpoint (e.g. `GET /api/v1/assets?limit=1`) so your checks exercise the database path, not just the token verification path.
```

### Task 14: Trim the "API-key management" section

**Files:**
- Modify: `/home/mike/trakrf-docs-tra-415/docs/api/private-endpoints.md:44-55`

- [ ] **Step 1: Keep the session-JWT-only explanation; drop the speculative "if and when" paragraph**

Replace lines 44-55 (the entire `## API-key management: session-JWT-only today {#api-key-management}` section) with:

```markdown
## API-key management is Internal {#api-key-management}

The `/api/v1/orgs/{id}/api-keys` endpoints back the Settings → API Keys UI and accept a **session-scoped JWT only** — an API-key JWT cannot mint or revoke other API keys. The auth mechanics are the standard `Authorization: Bearer <session-jwt>` form (no `Set-Cookie`); the server rejects API-key-scoped tokens on these endpoints with `401 unauthorized`. The intended flow is administrator → web UI.

That rules out CI-scripted key rotation against this endpoint. Options:

- **Rotate via the UI** — an admin mints a new key, updates the integration, and deletes the old key. This is the supported path end-to-end.
- **Ask for a rotation primitive** — if you have a concrete CI-rotation requirement, [email support](mailto:support@trakrf.id) so we can prioritize an API-key-authenticated rotation endpoint. Flagging this keeps us honest rather than handing out an undocumented endpoint that might move.
```

### Task 15: Replace "Classification decisions to come" with "Classification policy"

**Files:**
- Modify: `/home/mike/trakrf-docs-tra-415/docs/api/private-endpoints.md:57-65`

- [ ] **Step 1: Swap the forward-looking placeholder section for settled policy prose**

Replace lines 57-65 (the `## Classification decisions to come` section and everything below it through end-of-file) with:

```markdown
## Classification policy {#policy}

Every row above is one of:

- **Public** — published in [`/api`](/api). Contract stability covered by the OpenAPI spec and the [versioning policy](./versioning).
- **Internal** — listed here, not in [`/api`](/api). Subject to change without notice.

Public-with-caveats is not a separate classification. When a public endpoint has a stability nuance, it's expressed inline in the [`/api`](/api) reference (e.g. via `x-stability` or deprecation annotations on that endpoint).

If you believe a row belongs in a different bucket — especially if there's a concrete integration use case for an Internal endpoint — [email support](mailto:support@trakrf.id) and we'll review.
```

### Task 16: Lint and commit

- [ ] **Step 1: Run the Docusaurus link checker**

```bash
cd /home/mike/trakrf-docs-tra-415
pnpm build
```

Expected: build succeeds with no broken-link warnings for `./rate-limits`, `./versioning`, `/api`, or `./private-endpoints` anchors.

- [ ] **Step 2: Visual smoke — render the page locally**

```bash
cd /home/mike/trakrf-docs-tra-415
pnpm start &
sleep 3
curl -fsS http://localhost:3000/docs/api/private-endpoints > /dev/null
kill %1
```

Expected: `curl` returns 200; page renders. Manually confirm in browser: table has 11 rows, no "Pending" or "Undocumented" text anywhere, `/orgs/me` row's Status column links to `/api`.

- [ ] **Step 3: Commit**

```bash
cd /home/mike/trakrf-docs-tra-415
git add docs/api/private-endpoints.md
git commit -m "$(cat <<'EOF'
docs(tra-415): classify private-endpoints.md rows

- 10 rows marked Internal (SPA-only routes)
- /orgs/me now Public and published in the OpenAPI spec at /api
- Fix POST method for /users/me/current-org (was incorrectly listed as GET)
- Replace bare-object shape warning with current enveloped shape
- Replace "decisions to come" placeholder with settled classification policy

Paired with trakrf/platform PR that adds the swaggo annotations
and normalizes the /orgs/me response shape.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
```

### Task 17: Push and open the docs PR

- [ ] **Step 1: Push**

```bash
cd /home/mike/trakrf-docs-tra-415
git push -u origin miks2u/tra-415-classify-private-endpoints
```

- [ ] **Step 2: Open the PR, linking to the platform PR**

```bash
gh pr create --title "docs(tra-415): classify private-endpoints.md rows" --body "$(cat <<'EOF'
## Summary

- Every row now has a real Status + Classification (no more "Pending"/"Undocumented")
- \`/orgs/me\` marked Public; response shape section rewritten for the new \`{"data": {...}}\` envelope
- API-key management section trimmed to reflect the settled Internal classification
- "Decisions to come" replaced with a short Classification policy
- Fix: \`/users/me/current-org\` is \`POST\`, not \`GET\` as the previous table said

Paired with trakrf/platform#N (TRA-415 swaggo annotations + shape normalization) — already merged and live on preview.

## Test plan

- [ ] \`pnpm build\` clean
- [ ] Docs preview renders; table has 11 rows with no "Pending" cells
- [ ] \`/orgs/me\` link goes to the published \`/api\` entry

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"
```

---

## Done criteria

- Platform PR merged, deployed to preview: `curl -H "Authorization: Bearer <api-key>" https://app.preview.trakrf.id/api/v1/orgs/me` returns `{"data": {"id": ..., "name": ...}}`.
- `openapi.public.json` on preview contains `/api/v1/orgs/me`.
- `openapi.internal.json` (generated locally during the build) contains the 17 previously-unannotated orgs/users/members/invitations/api-keys operations.
- Docs PR merged: `docs.trakrf.id/docs/api/private-endpoints` shows a fully-classified table with no "Pending" or "Undocumented" cells; `/users/me/current-org` method column shows POST.
- Linear TRA-415 closed.
