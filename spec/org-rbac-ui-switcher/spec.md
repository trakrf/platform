# Feature: Organization RBAC UI - Phase 3a: Org Switcher

## Metadata
**Linear**: [TRA-136](https://linear.app/trakrf/issue/TRA-136)
**Workspace**: frontend
**Type**: feature
**Priority**: Urgent
**Phase**: 3a of 3 (Foundation - Switcher & Store)

## Outcome
Users can see their current organization in the header and switch between organizations they belong to.

## User Story
As a TrakRF user belonging to multiple organizations
I want to see my current org and switch between orgs
So that I can work in the correct context

## Context
**Current**: No org awareness in UI, no switcher.
**Desired**: Header shows current org with dropdown to switch.
**Depends On**: org-rbac (Phase 2 - API endpoints complete)

---

## Technical Requirements

### 1. Types (lib/types/org.ts)

```typescript
export type OrgRole = 'owner' | 'admin' | 'manager' | 'operator' | 'viewer';

export interface Organization {
  id: number;
  name: string;
}

export interface UserOrg {
  id: number;
  name: string;
}

export interface UserOrgWithRole {
  id: number;
  name: string;
  role: OrgRole;
}

export interface UserProfile {
  id: number;
  name: string;
  email: string;
  is_superadmin: boolean;
  current_org: UserOrgWithRole | null;
  orgs: UserOrg[];
}
```

### 2. API Client (lib/api/orgs.ts)

```typescript
export const orgsApi = {
  // Get user profile with orgs
  getMe: () => Promise<UserProfile>,

  // Switch current org
  setCurrentOrg: (orgId: number) => Promise<void>,

  // Create new org
  create: (name: string) => Promise<Organization>,
};
```

### 3. Org Store (stores/orgStore.ts)

```typescript
interface OrgState {
  // Current org context
  currentOrg: UserOrgWithRole | null;

  // All user's orgs
  userOrgs: UserOrg[];

  // Loading state
  isLoading: boolean;

  // Actions
  setCurrentOrg: (orgId: number) => Promise<void>;
  fetchUserProfile: () => Promise<void>;
  createOrg: (name: string) => Promise<Organization>;

  // Permission helpers
  isAdmin: () => boolean;
  isOwner: () => boolean;
  canManageMembers: () => boolean;
}
```

### 4. Components

#### RoleBadge.tsx
```typescript
interface RoleBadgeProps {
  role: OrgRole;
  size?: 'sm' | 'md';
}

// Colors:
// - owner: purple
// - admin: red/rose
// - manager: blue
// - operator: green
// - viewer: gray
```

#### OrgSwitcher.tsx
```typescript
// Header dropdown component
// - Shows current org name + role badge
// - Dropdown lists all user's orgs with role badges
// - "Create Organization" option at bottom (opens modal)
// - Selecting org calls setCurrentOrg and refreshes
```

#### CreateOrgModal.tsx
```typescript
// Simple modal for creating new org
// - Name input (required, min 1 char)
// - Create button
// - Loading state
// - Success closes modal and switches to new org
```

### 5. Integration

- Add OrgSwitcher to app header/navigation area
- On app load, call fetchUserProfile to populate store
- After org switch, refresh relevant data (assets, etc.)

---

## File Structure

```
frontend/src/
├── lib/
│   ├── types/
│   │   └── org.ts           # NEW: Org types
│   └── api/
│       └── orgs.ts          # NEW: Org API client
├── stores/
│   └── orgStore.ts          # NEW: Org Zustand store
├── components/
│   ├── RoleBadge.tsx        # NEW: Role indicator
│   ├── OrgSwitcher.tsx      # NEW: Header dropdown
│   └── CreateOrgModal.tsx   # NEW: Create org modal
└── App.tsx                  # MODIFY: Add OrgSwitcher to header
```

---

## Implementation Tasks

### Task 1: Types
- [ ] Create `lib/types/org.ts` with OrgRole, Organization, UserOrg, UserOrgWithRole, UserProfile

### Task 2: API Client
- [ ] Create `lib/api/orgs.ts` with getMe, setCurrentOrg, create methods

### Task 3: Org Store
- [ ] Create `stores/orgStore.ts` with Zustand
- [ ] Implement state and actions
- [ ] Implement permission helpers

### Task 4: RoleBadge Component
- [ ] Create badge component with role colors
- [ ] Support sm/md sizes

### Task 5: OrgSwitcher Component
- [ ] Create dropdown with current org display
- [ ] List all orgs with badges
- [ ] Handle org selection
- [ ] Add "Create Organization" option

### Task 6: CreateOrgModal Component
- [ ] Create modal with name input
- [ ] Handle creation and switching

### Task 7: Header Integration
- [ ] Add OrgSwitcher to app header
- [ ] Initialize org store on app load

---

## Validation Criteria

- [ ] Header shows current org name and role
- [ ] Dropdown shows all user's orgs with role badges
- [ ] Clicking different org switches context
- [ ] Role badges show correct colors
- [ ] "Create Organization" opens modal
- [ ] New org creation works and switches to it
- [ ] `just frontend validate` passes

## Success Metrics
- [ ] Org switching works end-to-end
- [ ] No console errors
- [ ] Types are correct throughout
- [ ] Components render correctly
