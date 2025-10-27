# Feature: Auto-Create Personal Organizations on Signup

## Origin
This specification emerged from improving the signup UX by following GitHub's architecture pattern for personal accounts and organizations.

## Outcome
**Remove friction from signup by auto-creating a personal organization from the user's email instead of asking them to name their organization.**

Users signing up will:
1. Only provide email and password
2. Automatically get a personal organization created (named from full email: mike@example.com → "mike-example-com")
3. Be set as the owner of that organization
4. Have the same foundation for future team features (invites, multi-org, switching)

## User Story
```gherkin
As a new user signing up
I want to start using the app immediately
So that I don't have to think about "organization names" before I understand the product

Given I am on the signup page
When I enter my email "mike@example.com" and password
Then a personal organization "mike" is auto-created for me
And I am set as the owner
And I can rename it later if needed
```

## Context

### Current State
- Signup form asks for: email, password, **organization name** (required)
- Backend creates user + organization + org_users membership in one transaction
- Organization name is user-provided string, slugified for domain field

### Problems with Current Approach
1. **Cognitive load**: New users don't know what to call their "organization"
2. **Friction**: Extra required field delays getting started
3. **Confusion**: "Organization" implies a team, but most signups are individuals
4. **Not standard**: GitHub, Linear, and other dev tools auto-create personal accounts

### Desired State
- Signup form asks for: email, password (**no org name field**)
- Backend auto-generates organization name from full email (slugified: mike@example.com → "mike-example-com")
- User can rename organization later in settings (future feature)
- Foundation is laid for team features (invites, multi-org) without schema changes

### Research Findings

**GitHub's Architecture:**
- Personal accounts are organizations with one member
- User identity separate from organizations
- Users → Organizations via membership table (many-to-many)
- Everything works the same (permissions, billing) whether personal or team

**Unified Account Pattern (Industry Standard):**
- One `organizations` table (personal flag optional)
- One `users` table
- One `memberships` table (with roles)
- Personal account = organization with one owner
- Upgrading to team = inviting more members (no migration needed)

**Our Current Schema - Already Perfect!**
```sql
organizations (name, domain, metadata, ...)
users (email, name, password_hash, ...)
org_users (org_id, user_id, role, status, ...) -- ✅ many-to-many with roles
  - role: owner, admin, member, readonly
  - status: active, inactive, suspended, invited  -- ✅ already supports invites!
```

## Technical Requirements

### Frontend Changes
- [x] Remove `organizationName` field from SignupScreen component
- [x] Remove `organizationName` validation logic
- [x] Update form state to only track `email` and `password`
- [x] Remove `organizationName` from signup API call
- [x] Update tests for SignupScreen

### Backend Changes
- [x] Remove `OrgName` field from `SignupRequest` struct (breaking change, greenfield)
  ```go
  type SignupRequest struct {
      Email    string `json:"email" validate:"required,email"`
      Password string `json:"password" validate:"required,min=8"`
      // OrgName removed - auto-generated from email
  }
  ```
- [x] Add helper function to generate org name from email:
  ```go
  // generateOrgNameFromEmail slugifies the entire email to create unique org names
  // Example: "mike@example.com" -> "mike-example-com"
  // This eliminates collisions since emails are unique
  func generateOrgNameFromEmail(email string) string {
      // Replace @ and . with hyphens, convert to lowercase
      slug := strings.ToLower(email)
      slug = strings.ReplaceAll(slug, "@", "-")
      slug = strings.ReplaceAll(slug, ".", "-")
      return slug
  }
  ```
- [x] Update `Signup()` service to always auto-generate org name:
  ```go
  orgName := generateOrgNameFromEmail(request.Email)
  domain := slugifyOrgName(orgName)

  // Insert with is_personal flag
  orgQuery := `
    INSERT INTO trakrf.organizations (name, domain, is_personal)
    VALUES ($1, $2, true)
    RETURNING ...
  `
  ```
- [x] Update API documentation in handler comments
- [x] Update tests for auth service and handler

### Database Changes (Required)
**Add `is_personal` flag to organizations table:**

```sql
ALTER TABLE trakrf.organizations
  ADD COLUMN is_personal BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN organizations.is_personal IS
  'True if this is a personal organization (single-owner account)';
```

**Why `is_personal` flag (not `personal_org_id` FK)?**
- ✅ Simple, no circular dependencies
- ✅ Single source of truth
- ✅ org_users join is already indexed
- ✅ Matches GitHub's model ("personal accounts are just organizations")
- ✅ Can add FK later if needed (YAGNI)

**Finding user's personal org:**
```sql
SELECT o.*
FROM organizations o
JOIN org_users ou ON o.id = ou.org_id
WHERE ou.user_id = $1
  AND o.is_personal = true
  AND ou.role = 'owner'
LIMIT 1;
```

This allows:
- Filtering personal vs team orgs in UI
- Different billing/feature logic
- Analytics on account types

### API Contract Changes

**Before:**
```json
POST /api/v1/auth/signup
{
  "email": "mike@example.com",
  "password": "securepass123",
  "org_name": "My Company"    // ⬅️ Required
}
```

**After:**
```json
POST /api/v1/auth/signup
{
  "email": "mike@example.com",
  "password": "securepass123"
  // org_name removed, auto-generated from email
}
```

**Breaking Change (Greenfield):**
- Remove `org_name` from request entirely (frontend won't send it)
- Backend auto-generates org name from email
- No backward compatibility needed (greenfield project)

## Implementation Details

### Organization Name Generation Logic

**Primary rule:** Slugify the entire email (replace `@` and `.` with hyphens)

**Examples:**
```
mike@example.com        → org_name: "mike-example-com",        domain: "mike-example-com"
alice.smith@company.io  → org_name: "alice-smith-company-io",  domain: "alice-smith-company-io"
bob+test@gmail.com      → org_name: "bob+test-gmail-com",     domain: "bob+test-gmail-com"
```

**Collision handling:** None needed!
- Email uniqueness constraint guarantees unique org names
- No need for counters, random suffixes, or collision detection
- 99.9% collision elimination (only edge case: user changes email, old org name still exists)

### Future Features Enabled by Current Schema

**✅ User Invitations:**
```sql
-- Already supported via org_users.status = 'invited'
INSERT INTO org_users (org_id, user_id, role, status)
VALUES (123, 456, 'member', 'invited');
```

**✅ Multi-Organization Access:**
```sql
-- User can belong to multiple orgs
SELECT o.* FROM organizations o
JOIN org_users ou ON ou.org_id = o.id
WHERE ou.user_id = 456 AND ou.status = 'active';
```

**✅ Organization Switching:**
```typescript
// Frontend: query user's orgs, show dropdown
GET /api/v1/users/me/organizations
// Switch context by updating JWT with different org_id
```

**✅ Rename Personal Org:** (Future settings page)
```sql
UPDATE organizations SET name = 'My New Company Name'
WHERE id = 123;
```

## Validation Criteria

### Functional Requirements
- [ ] Signup succeeds with only email and password (no org_name)
- [ ] Organization is created with name = full email slugified (e.g., "mike-example-com")
- [ ] Organization is created with `is_personal = true`
- [ ] Organization domain is same as name (e.g., "mike-example-com")
- [ ] User is set as owner in org_users table
- [ ] JWT includes org_id in claims

### Testing Requirements
- [ ] Unit test: `generateOrgNameFromEmail()` with various email formats
- [ ] Unit test: Signup service auto-generates org name when omitted
- [ ] Unit test: Signup service uses provided org name when present
- [ ] Integration test: Full signup flow without org_name field
- [ ] E2E test: Frontend signup form without org name field
- [ ] E2E test: Verify org created in database with correct name

### Edge Cases
- [ ] Email with dots: `alice.smith@example.com` → org: `"alice-smith-example-com"`
- [ ] Email with plus: `bob+tag@example.com` → org: `"bob+tag-example-com"`
- [ ] Email with hyphens: `john-doe@example.com` → org: `"john-doe-example-com"`
- [ ] Very long email (50+ chars): Should succeed (PostgreSQL VARCHAR(255) can handle it)
- [ ] Duplicate org name collision: Impossible due to email uniqueness constraint

## Conversation References

**Key Insight:**
> "I want to follow the exact same architecture as github. most people are generally familiar with that."

**Architecture Decision:**
> "Personal accounts are just organizations with a single user. This unified approach is recommended because it's very complex and painful to add teams functionality later."

**Research Finding:**
> "Most people will be better off with the Linear model" (hybrid: single user account, multiple organizations, can have multiple user accounts)

**Current Schema Assessment:**
> Your `organizations` + `users` + `org_users` schema already matches the industry-standard unified account pattern. No migration needed.

**Scope Confirmation:**
> "fixing signup form, making sure we have schema right for future"
> "yes invites, yes multiorg, yes switching, all future none needed now"

## Out of Scope (Future Enhancements)

The following are explicitly **NOT** part of this spec:
- [ ] Organization renaming UI (settings page)
- [ ] Organization switching UI (dropdown in header)
- [ ] User invitation flow (invite links, acceptance)
- [ ] Multiple organizations per user (supported by schema, no UI yet)
- [ ] Collision handling for duplicate org names
- [ ] `is_personal` flag in organizations table (optional, not required)

## Success Metrics

**Before:** User sees 3 required fields on signup
**After:** User sees 2 required fields on signup (33% reduction in friction)

**Developer Experience:**
- Schema supports future team features without migration
- Follows industry-standard architecture pattern (GitHub, Linear)
- No collision handling needed (email uniqueness guarantees unique org names)
- Simple, maintainable implementation

## Implementation Checklist

### Phase 1: Database Migration
- [ ] Create migration file: `000016_personal_orgs.up.sql`
- [ ] Add `is_personal` column to organizations table
- [ ] Run migration: `just db-migrate-up`
- [ ] Verify column added: `\d trakrf.organizations`

### Phase 2: Backend Changes
- [ ] Add `generateOrgNameFromEmail()` helper function
- [ ] Remove `OrgName` from `SignupRequest` struct
- [ ] Update `Signup()` service to auto-generate org name
- [ ] Update INSERT query to set `is_personal = true`
- [ ] Update handler documentation/swagger comments
- [ ] Write unit tests for email → org name generation
- [ ] Write unit tests for signup service
- [ ] Run backend tests: `just backend test`

### Phase 3: Frontend Changes
- [ ] Remove organizationName state from SignupScreen
- [ ] Remove organizationName input field from form
- [ ] Remove organizationName validation logic
- [ ] Update signup API call (remove org_name from request body)
- [ ] Update SignupScreen tests
- [ ] Run frontend tests: `just frontend test`

### Phase 4: Integration Testing
- [ ] Test full signup flow end-to-end
- [ ] Verify organization created with correct name
- [ ] Verify user is owner in org_users table
- [ ] Verify login works after signup
- [ ] Test with various email formats (dots, hyphens, plus signs)
- [ ] Run full validation: `just validate`

### Phase 5: Documentation
- [ ] Update API documentation (if separate from code)
- [ ] Add comment explaining auto-org-creation logic in code
- [ ] Update README if signup flow is documented there

## Questions Resolved

**Q:** Do we need to rename `organizations` back to `accounts`?
**A:** No. Current schema is correct. "Organizations" is clearer than "accounts" for the domain.

**Q:** Will we regret not having separate `accounts` and `organizations` tables?
**A:** No. The unified pattern is industry standard and explicitly recommended by experts. Adding `is_personal` flag later is trivial if needed.

**Q:** What if two users have the same email prefix (mike@example.com, mike@company.com)?
**A:** Not an issue! Using full email slugification: mike@example.com → "mike-example-com", mike@company.com → "mike-company-com". No collisions possible since emails are unique.

**Q:** Should org name be the full email or just the prefix?
**A:** Full email (slugified). While just the prefix looks cleaner, using the full email eliminates collision handling entirely. Since emails are unique, org names are guaranteed unique. Format: `mike@example.com` → `"mike-example-com"` is readable and collision-free.

## References

- GitHub account types: https://docs.github.com/en/get-started/learning-about-github/types-of-github-accounts
- Multi-tenant SaaS data modeling: https://www.flightcontrol.dev/blog/ultimate-guide-to-multi-tenant-saas-data-modeling
- Building optimal user database model: https://www.donedone.com/blog/building-the-optimal-user-database-model-for-your-application

---

## Linear Issue Template

**Title:** Auto-create personal organizations on signup (remove org name field)

**Description:**
Remove the organization name field from signup form and auto-generate personal organizations from email addresses, following GitHub's architecture pattern.

**Acceptance Criteria:**
- Signup form only asks for email and password
- Organization auto-created with name from full email slugified (e.g., mike@example.com → org: "mike-example-com")
- User set as owner of their personal organization
- All tests passing (unit, integration, e2e)
- Backward compatible (API still accepts org_name if provided)

**Labels:** `feature`, `auth`, `ux-improvement`, `architecture`, `database`

**Estimate:** 2-3 hours (database + backend + frontend + tests)

**Priority:** High (Foundation work - do BEFORE TRA-96/97/98)

**Key Benefit:** No collision handling needed! Using full email slugification (mike@example.com → "mike-example-com") guarantees unique org names since emails are unique.

**Recommended Order:**
1. **This issue** (Personal Orgs) - Get UX right first
2. TRA-96 (Auth Foundation)
3. TRA-97 (Header & User Menu)
4. TRA-98 (Protected Routes)
