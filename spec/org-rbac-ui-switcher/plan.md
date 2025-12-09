# Implementation Plan: Organization RBAC UI - Phase 3a: Org Switcher

Generated: 2024-12-09
Specification: spec.md

## Understanding

Implement org switching UI that allows users to see their current organization in the header (breadcrumb style) and switch between organizations. Uses hybrid store pattern where authStore holds profile data and orgStore derives state + owns org-specific actions.

**Key decisions from clarifying questions:**
- Store pattern: Simple (like authStore), not complex action factories
- Header: Breadcrumb style - "OrgName / PageTitle"
- URLs: Org via state only, no URL changes needed
- API: Hybrid - authStore fetches `/users/me`, orgStore derives + has actions
- Create org: Dedicated screen at `#create-org`

## Relevant Files

**Reference Patterns** (existing code to follow):
- `src/stores/authStore.ts` - Simple store pattern with persist middleware
- `src/components/UserMenu.tsx` - HeadlessUI Menu dropdown pattern
- `src/components/Header.tsx` - Header integration point
- `src/lib/api/auth.ts` - API client pattern
- `src/types/assets/index.ts` - Type definition organization

**Files to Create**:
- `src/lib/types/org.ts` - Org-related type definitions
- `src/lib/api/orgs.ts` - Org API client methods
- `src/stores/orgStore.ts` - Org state management (derives from authStore)
- `src/components/RoleBadge.tsx` - Role indicator badge
- `src/components/OrgSwitcher.tsx` - Header dropdown component
- `src/screens/CreateOrgScreen.tsx` - Create organization screen

**Files to Modify**:
- `src/stores/authStore.ts` - Add profile fetch with orgs data
- `src/components/Header.tsx` - Add OrgSwitcher, change title to breadcrumb
- `src/App.tsx` - Add `#create-org` route
- `src/stores/index.ts` - Export orgStore

## Architecture Impact
- **Subsystems affected**: Types, API client, Stores, Components, Routing
- **New dependencies**: None (using existing HeadlessUI, Zustand)
- **Breaking changes**: None (additive only)

## Task Breakdown

### Task 1: Add Org Types
**File**: `src/lib/types/org.ts`
**Action**: CREATE
**Pattern**: Reference `src/types/assets/index.ts`

**Implementation**:
```typescript
export type OrgRole = 'owner' | 'admin' | 'manager' | 'operator' | 'viewer';

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

export interface Organization {
  id: number;
  name: string;
  identifier: string;
  is_personal: boolean;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface CreateOrgRequest {
  name: string;
}

export interface CreateOrgResponse {
  data: Organization;
}

export interface SetCurrentOrgRequest {
  org_id: number;
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 2: Add Org API Client
**File**: `src/lib/api/orgs.ts`
**Action**: CREATE
**Pattern**: Reference `src/lib/api/auth.ts`

**Implementation**:
```typescript
import { apiClient } from './client';
import type {
  UserProfile,
  CreateOrgRequest,
  CreateOrgResponse,
  SetCurrentOrgRequest
} from '@/lib/types/org';

export const orgsApi = {
  // Get user profile with orgs
  getProfile: () =>
    apiClient.get<{ data: UserProfile }>('/users/me'),

  // Switch current org
  setCurrentOrg: (data: SetCurrentOrgRequest) =>
    apiClient.post('/users/me/current-org', data),

  // Create new org
  create: (data: CreateOrgRequest) =>
    apiClient.post<CreateOrgResponse>('/orgs', data),
};
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 3: Update AuthStore with Profile
**File**: `src/stores/authStore.ts`
**Action**: MODIFY
**Pattern**: Existing authStore pattern

**Implementation**:
Add to AuthState interface:
```typescript
// Add to interface
profile: UserProfile | null;
fetchProfile: () => Promise<void>;
```

Add to store implementation:
```typescript
// Initial state
profile: null,

// Add method
fetchProfile: async () => {
  try {
    const response = await orgsApi.getProfile();
    set({ profile: response.data.data });
  } catch (err) {
    console.error('Failed to fetch profile:', err);
  }
},

// Update login to fetch profile after success
login: async (email, password) => {
  // ... existing login logic ...
  // After successful login, fetch profile
  await get().fetchProfile();
},

// Update logout to clear profile
logout: () => {
  set({
    user: null,
    token: null,
    profile: null,  // Add this
    isAuthenticated: false,
    error: null,
  });
},

// Update initialize to fetch profile if authenticated
initialize: () => {
  const state = get();
  if (state.token) {
    // Validate token and fetch profile
    get().fetchProfile();
  }
},
```

Add to partialize for persistence:
```typescript
partialize: (state) => ({
  token: state.token,
  user: state.user,
  profile: state.profile,  // Add this
}),
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 4: Create OrgStore
**File**: `src/stores/orgStore.ts`
**Action**: CREATE
**Pattern**: Reference `src/stores/authStore.ts` (simple pattern)

**Implementation**:
```typescript
import { create } from 'zustand';
import { useAuthStore } from './authStore';
import { orgsApi } from '@/lib/api/orgs';
import type { OrgRole, UserOrg, UserOrgWithRole, Organization } from '@/lib/types/org';

interface OrgState {
  // Loading states
  isLoading: boolean;
  error: string | null;

  // Derived getters (read from authStore)
  getCurrentOrg: () => UserOrgWithRole | null;
  getUserOrgs: () => UserOrg[];
  getCurrentRole: () => OrgRole | null;

  // Actions
  switchOrg: (orgId: number) => Promise<void>;
  createOrg: (name: string) => Promise<Organization>;

  // Permission helpers
  isOwner: () => boolean;
  isAdmin: () => boolean;
  canManageMembers: () => boolean;
  canManageSettings: () => boolean;
}

export const useOrgStore = create<OrgState>((set, get) => ({
  isLoading: false,
  error: null,

  // Derived from authStore
  getCurrentOrg: () => {
    return useAuthStore.getState().profile?.current_org ?? null;
  },

  getUserOrgs: () => {
    return useAuthStore.getState().profile?.orgs ?? [];
  },

  getCurrentRole: () => {
    return useAuthStore.getState().profile?.current_org?.role ?? null;
  },

  // Switch org
  switchOrg: async (orgId: number) => {
    set({ isLoading: true, error: null });
    try {
      await orgsApi.setCurrentOrg({ org_id: orgId });
      // Refresh profile to get updated current_org
      await useAuthStore.getState().fetchProfile();
      set({ isLoading: false });
    } catch (err: any) {
      const message = err.response?.data?.error?.detail || 'Failed to switch organization';
      set({ error: message, isLoading: false });
      throw err;
    }
  },

  // Create org
  createOrg: async (name: string) => {
    set({ isLoading: true, error: null });
    try {
      const response = await orgsApi.create({ name });
      const newOrg = response.data.data;
      // Switch to new org
      await get().switchOrg(newOrg.id);
      set({ isLoading: false });
      return newOrg;
    } catch (err: any) {
      const message = err.response?.data?.error?.detail || 'Failed to create organization';
      set({ error: message, isLoading: false });
      throw err;
    }
  },

  // Permission helpers
  isOwner: () => {
    const role = get().getCurrentRole();
    return role === 'owner';
  },

  isAdmin: () => {
    const role = get().getCurrentRole();
    return role === 'owner' || role === 'admin';
  },

  canManageMembers: () => {
    const role = get().getCurrentRole();
    return role === 'owner' || role === 'admin';
  },

  canManageSettings: () => {
    const role = get().getCurrentRole();
    return role === 'owner' || role === 'admin';
  },
}));
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 5: Export OrgStore
**File**: `src/stores/index.ts`
**Action**: MODIFY

**Implementation**:
```typescript
// Add export
export { useOrgStore } from './orgStore';
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 6: Create RoleBadge Component
**File**: `src/components/RoleBadge.tsx`
**Action**: CREATE
**Pattern**: Simple presentational component

**Implementation**:
```typescript
import type { OrgRole } from '@/lib/types/org';

interface RoleBadgeProps {
  role: OrgRole;
  size?: 'sm' | 'md';
}

const roleConfig: Record<OrgRole, { label: string; className: string }> = {
  owner: {
    label: 'Owner',
    className: 'bg-purple-100 text-purple-800 dark:bg-purple-900 dark:text-purple-200',
  },
  admin: {
    label: 'Admin',
    className: 'bg-rose-100 text-rose-800 dark:bg-rose-900 dark:text-rose-200',
  },
  manager: {
    label: 'Manager',
    className: 'bg-blue-100 text-blue-800 dark:bg-blue-900 dark:text-blue-200',
  },
  operator: {
    label: 'Operator',
    className: 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200',
  },
  viewer: {
    label: 'Viewer',
    className: 'bg-gray-100 text-gray-800 dark:bg-gray-700 dark:text-gray-200',
  },
};

export function RoleBadge({ role, size = 'sm' }: RoleBadgeProps) {
  const config = roleConfig[role];

  const sizeClasses = size === 'sm'
    ? 'px-1.5 py-0.5 text-xs'
    : 'px-2 py-1 text-sm';

  return (
    <span className={`inline-flex items-center rounded font-medium ${sizeClasses} ${config.className}`}>
      {config.label}
    </span>
  );
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 7: Create OrgSwitcher Component
**File**: `src/components/OrgSwitcher.tsx`
**Action**: CREATE
**Pattern**: Reference `src/components/UserMenu.tsx` for HeadlessUI Menu

**Implementation**:
```typescript
import { Menu } from '@headlessui/react';
import { ChevronDown, Plus, Building2 } from 'lucide-react';
import { useOrgStore } from '@/stores';
import { useUIStore } from '@/stores';
import { RoleBadge } from './RoleBadge';

export function OrgSwitcher() {
  const getCurrentOrg = useOrgStore((s) => s.getCurrentOrg);
  const getUserOrgs = useOrgStore((s) => s.getUserOrgs);
  const switchOrg = useOrgStore((s) => s.switchOrg);
  const isLoading = useOrgStore((s) => s.isLoading);

  const currentOrg = getCurrentOrg();
  const userOrgs = getUserOrgs();

  const handleSwitchOrg = async (orgId: number) => {
    if (orgId === currentOrg?.id) return;
    try {
      await switchOrg(orgId);
      // Could add toast notification here
    } catch (err) {
      // Error handled in store
    }
  };

  const handleCreateOrg = () => {
    useUIStore.getState().setActiveTab('create-org' as any);
    window.location.hash = '#create-org';
  };

  if (!currentOrg) {
    return null;
  }

  return (
    <Menu as="div" className="relative inline-block text-left">
      <Menu.Button
        className="flex items-center gap-2 px-3 py-2 rounded-lg hover:bg-gray-100 dark:hover:bg-gray-700 transition-colors"
        disabled={isLoading}
      >
        <Building2 className="w-4 h-4 text-gray-500 dark:text-gray-400" />
        <span className="font-medium text-gray-900 dark:text-gray-100 max-w-[150px] truncate">
          {currentOrg.name}
        </span>
        <RoleBadge role={currentOrg.role} size="sm" />
        <ChevronDown className={`w-4 h-4 text-gray-500 dark:text-gray-400 transition-transform ${isLoading ? 'animate-spin' : ''}`} />
      </Menu.Button>

      <Menu.Items className="absolute left-0 mt-2 w-64 origin-top-left divide-y divide-gray-100 dark:divide-gray-700 rounded-lg bg-white dark:bg-gray-800 shadow-lg ring-1 ring-black ring-opacity-5 focus:outline-none z-50">
        {/* Org list */}
        <div className="p-1 max-h-64 overflow-y-auto">
          {userOrgs.map((org) => (
            <Menu.Item key={org.id}>
              {({ active }) => (
                <button
                  onClick={() => handleSwitchOrg(org.id)}
                  className={`${
                    active ? 'bg-gray-100 dark:bg-gray-700' : ''
                  } ${
                    org.id === currentOrg.id ? 'bg-blue-50 dark:bg-blue-900/20' : ''
                  } group flex w-full items-center justify-between rounded-md px-3 py-2 text-sm transition-colors`}
                >
                  <span className="text-gray-900 dark:text-gray-100 truncate">
                    {org.name}
                  </span>
                  {org.id === currentOrg.id && (
                    <span className="text-blue-600 dark:text-blue-400 text-xs">Current</span>
                  )}
                </button>
              )}
            </Menu.Item>
          ))}
        </div>

        {/* Create org option */}
        <div className="p-1">
          <Menu.Item>
            {({ active }) => (
              <button
                onClick={handleCreateOrg}
                className={`${
                  active ? 'bg-gray-100 dark:bg-gray-700' : ''
                } group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
              >
                <Plus className="w-4 h-4" />
                Create Organization
              </button>
            )}
          </Menu.Item>
        </div>
      </Menu.Items>
    </Menu>
  );
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 8: Create CreateOrgScreen
**File**: `src/screens/CreateOrgScreen.tsx`
**Action**: CREATE
**Pattern**: Reference existing screen components

**Implementation**:
```typescript
import { useState } from 'react';
import { Building2, ArrowLeft } from 'lucide-react';
import { useOrgStore, useUIStore } from '@/stores';
import toast from 'react-hot-toast';

export default function CreateOrgScreen() {
  const [name, setName] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const createOrg = useOrgStore((s) => s.createOrg);
  const error = useOrgStore((s) => s.error);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!name.trim()) return;

    setIsSubmitting(true);
    try {
      await createOrg(name.trim());
      toast.success('Organization created!');
      // Redirect to home (now in context of new org)
      useUIStore.getState().setActiveTab('home');
      window.location.hash = '#home';
    } catch (err) {
      // Error is in store
    } finally {
      setIsSubmitting(false);
    }
  };

  const handleBack = () => {
    window.history.back();
  };

  return (
    <div className="max-w-md mx-auto">
      <button
        onClick={handleBack}
        className="flex items-center gap-2 text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 mb-6 transition-colors"
      >
        <ArrowLeft className="w-4 h-4" />
        Back
      </button>

      <div className="bg-white dark:bg-gray-800 rounded-lg shadow-sm border border-gray-200 dark:border-gray-700 p-6">
        <div className="flex items-center gap-3 mb-6">
          <div className="p-2 bg-blue-100 dark:bg-blue-900 rounded-lg">
            <Building2 className="w-6 h-6 text-blue-600 dark:text-blue-400" />
          </div>
          <div>
            <h1 className="text-xl font-semibold text-gray-900 dark:text-gray-100">
              Create Organization
            </h1>
            <p className="text-sm text-gray-500 dark:text-gray-400">
              Set up a new organization to manage assets and team members.
            </p>
          </div>
        </div>

        <form onSubmit={handleSubmit}>
          <div className="mb-4">
            <label
              htmlFor="org-name"
              className="block text-sm font-medium text-gray-700 dark:text-gray-300 mb-1"
            >
              Organization Name
            </label>
            <input
              id="org-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder="Acme Corporation"
              className="w-full px-3 py-2 border border-gray-300 dark:border-gray-600 rounded-lg bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 placeholder-gray-400 dark:placeholder-gray-500 focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent"
              autoFocus
              required
              minLength={1}
              maxLength={255}
            />
          </div>

          {error && (
            <div className="mb-4 p-3 bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg text-sm text-red-600 dark:text-red-400">
              {error}
            </div>
          )}

          <button
            type="submit"
            disabled={!name.trim() || isSubmitting}
            className="w-full px-4 py-2 bg-blue-600 hover:bg-blue-700 disabled:bg-blue-400 disabled:cursor-not-allowed text-white font-medium rounded-lg transition-colors"
          >
            {isSubmitting ? 'Creating...' : 'Create Organization'}
          </button>
        </form>
      </div>
    </div>
  );
}
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 9: Update Header with Breadcrumb
**File**: `src/components/Header.tsx`
**Action**: MODIFY
**Pattern**: Existing Header structure

**Implementation**:
Add import:
```typescript
import { OrgSwitcher } from './OrgSwitcher';
import { useOrgStore } from '@/stores';
```

Replace page title section with breadcrumb:
```typescript
// Find the h1 with currentPage.title and replace with:
<div className="flex items-center gap-2">
  {isAuthenticated && <OrgSwitcher />}
  {isAuthenticated && currentOrg && (
    <span className="text-gray-400 dark:text-gray-500">/</span>
  )}
  <h1 className="text-lg md:text-2xl font-bold text-gray-900 dark:text-gray-100">
    {currentPage.title}
  </h1>
</div>

// Add inside component to get currentOrg:
const currentOrg = useOrgStore((s) => s.getCurrentOrg)();
```

**Validation**: `just frontend lint && just frontend typecheck`

---

### Task 10: Add Route to App.tsx
**File**: `src/App.tsx`
**Action**: MODIFY

**Implementation**:
Add to TabType (in uiStore.ts or where defined):
```typescript
// Add 'create-org' to valid tabs
export type TabType = 'home' | 'inventory' | ... | 'create-org';
```

Add to VALID_TABS array in App.tsx:
```typescript
const VALID_TABS = ['home', 'inventory', ..., 'create-org'];
```

Add lazy import:
```typescript
const CreateOrgScreen = lazy(() => import('@/screens/CreateOrgScreen'));
```

Add to tabComponents:
```typescript
const tabComponents: Record<string, React.ComponentType<any>> = {
  // ... existing ...
  'create-org': CreateOrgScreen,
};
```

**Validation**: `just frontend lint && just frontend typecheck && just frontend build`

---

## Risk Assessment

- **Risk**: authStore changes could affect existing login flow
  **Mitigation**: Minimal changes - just adding profile field and fetchProfile method

- **Risk**: OrgSwitcher showing before profile loaded
  **Mitigation**: Return null if no currentOrg, handle loading state

- **Risk**: HeadlessUI Menu styling inconsistency
  **Mitigation**: Follow exact pattern from UserMenu.tsx

## Integration Points
- Store updates: authStore (add profile), new orgStore
- Route changes: Add #create-org
- Header changes: Add OrgSwitcher, breadcrumb style

## VALIDATION GATES (MANDATORY)

After EVERY code change, run from frontend directory:
```bash
just lint      # Must pass
just typecheck # Must pass
```

After Task 10 (final):
```bash
just validate  # Full validation: lint + typecheck + test + build
```

**Do not proceed to next task until current task passes all gates.**

## Plan Quality Assessment

**Complexity Score**: 5/10 (MEDIUM-LOW)
**Confidence Score**: 9/10 (HIGH)

**Confidence Factors**:
✅ Clear requirements from spec + clarifying questions
✅ Similar patterns found: UserMenu dropdown, authStore persistence
✅ All clarifying questions answered (store pattern, header placement, API integration, create flow)
✅ Existing HeadlessUI Menu pattern to follow
✅ No new dependencies required
✅ Minimal changes to existing code (additive mostly)

**Assessment**: Well-scoped feature with clear patterns to follow. Hybrid store approach keeps concerns separated while avoiding complexity.

**Estimated one-pass success probability**: 85%

**Reasoning**: Strong existing patterns, clear decisions from Q&A, mostly new files (low risk of breaking existing code). Main risk is authStore modification but changes are additive.
