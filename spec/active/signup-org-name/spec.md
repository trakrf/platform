# Feature: Required Organization Name at Signup (TRA-190)

## Outcome

Users provide an organization name during signup instead of having a confusing "personal org" auto-created with their email address.

## User Story

As a new user signing up for TrakRF
I want to name my organization during registration
So that I see a meaningful name in the UI instead of my email address

## Context

**Current**: At signup, we auto-create a "personal organization" using the user's email (e.g., "miks2u+t7@gmail.com" appears in the org switcher). This is confusing and adds unnecessary complexity.

**Desired**: Users provide their organization name upfront. No "personal org" concept - we're B2B SaaS, not GitHub with personal repos.

**Rationale**:
- Simplifies the data model
- Eliminates a concept we'd have to explain to white-label partners
- One less code path to maintain

## Technical Requirements

### Frontend
- Add required "Organization Name" field to signup form
- Validate: non-empty, reasonable length
- Remove any UI references to "personal organization" concept
- Add helper text near org name field: "If your company is already using TrakRF, ask your admin for an invite instead of creating a new organization."

### Backend
- Stop auto-creating personal orgs at signup
- Create org with user-provided name instead
- Set `is_personal=false` for new orgs

### Database (Optional)
- Consider removing `is_personal` column from organizations table
- Migration to drop column if we fully remove the concept
- Or: leave column but stop using it (safer for rollback)

## Out of Scope

- "Join existing organization" flow
- Domain-based org matching
- Invite workflow improvements

These can be added later when self-serve signup volume justifies it.

## Validation Criteria

- [ ] Signup form has required "Organization Name" field
- [ ] Helper text guides existing company users to request an invite
- [ ] New users see their chosen org name in the switcher (not email)
- [ ] Existing users/orgs unaffected
- [ ] All auth flows still work (signup, login, password reset)
- [ ] Backend tests pass
- [ ] Frontend tests pass

## Files Likely Affected

**Frontend**:
- `frontend/src/components/auth/SignupForm.tsx` (or similar)
- Any "personal org" references in UI

**Backend**:
- `backend/internal/handlers/auth/signup.go` (or similar)
- Org creation logic in auth flow

**Database**:
- Migration for `is_personal` column (if removing)
