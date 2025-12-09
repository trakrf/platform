# Implementation Plan: Organization RBAC UI - Phase 3b

## Summary
Add member management and org settings screens with navigation from OrgSwitcher dropdown.

## Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Screen location | `components/` | Follow existing convention (CreateOrgScreen pattern) |
| Permission model | Hide admin controls | Non-admins see screens, admin-only controls hidden (YAGNI) |
| Last-admin protection | Backend-first | Backend enforces rules, UI reflects for UX |
| Navigation | OrgSwitcher dropdown | GitHub-style, admin only sees Settings/Members links |

## File Changes

### New Files
```
frontend/src/components/
├── MembersScreen.tsx       # Member management table
├── OrgSettingsScreen.tsx   # Org name editing + delete
└── DeleteOrgModal.tsx      # Confirmation modal with name-match
```

### Modified Files
```
frontend/src/
├── App.tsx                 # Add routes: 'org-members', 'org-settings'
├── stores/uiStore.ts       # Add to TabType union
└── components/OrgSwitcher.tsx  # Add Settings/Members nav links
```

---

## Tasks

### Task 1: Add Routes (App.tsx + uiStore.ts)

**Files**: `frontend/src/App.tsx`, `frontend/src/stores/uiStore.ts`

1. Update TabType in `uiStore.ts`:
```typescript
export type TabType = '...' | 'org-members' | 'org-settings';
```

2. Add lazy imports in `App.tsx`:
```typescript
const MembersScreen = lazyWithRetry(() => import('@/components/MembersScreen'));
const OrgSettingsScreen = lazyWithRetry(() => import('@/components/OrgSettingsScreen'));
```

3. Add to `VALID_TABS`:
```typescript
const VALID_TABS: TabType[] = [..., 'org-members', 'org-settings'];
```

4. Add to `tabComponents`:
```typescript
'org-members': MembersScreen,
'org-settings': OrgSettingsScreen,
```

5. Add to `loadingScreens`:
```typescript
'org-members': LoadingScreen,
'org-settings': LoadingScreen,
```

---

### Task 2: Add Navigation Links (OrgSwitcher.tsx)

**File**: `frontend/src/components/OrgSwitcher.tsx`

Add third section after "Create Organization" for admin users:

```typescript
// Import Settings, Users icons from lucide-react
import { ChevronDown, Building2, Plus, Check, Settings, Users } from 'lucide-react';

// Inside Menu.Items, after Create Organization section:
{currentRole && ['owner', 'admin'].includes(currentRole) && (
  <div className="p-1">
    <Menu.Item>
      {({ active }) => (
        <a
          href="#org-settings"
          className={`${active ? 'bg-gray-100 dark:bg-gray-700' : ''} group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
        >
          <Settings className="w-4 h-4" />
          Organization Settings
        </a>
      )}
    </Menu.Item>
    <Menu.Item>
      {({ active }) => (
        <a
          href="#org-members"
          className={`${active ? 'bg-gray-100 dark:bg-gray-700' : ''} group flex w-full items-center gap-2 rounded-md px-3 py-2 text-sm text-gray-900 dark:text-gray-100 transition-colors`}
        >
          <Users className="w-4 h-4" />
          Members
        </a>
      )}
    </Menu.Item>
  </div>
)}
```

---

### Task 3: Create MembersScreen

**File**: `frontend/src/components/MembersScreen.tsx`

**Features**:
- Fetch members via `orgsApi.listMembers(orgId)`
- Table: Name, Email, Role, Joined, Actions
- "You" badge on current user's row
- Role dropdown (admin only) - calls `orgsApi.updateMemberRole`
- Remove button (admin only) - calls `orgsApi.removeMember`
- Backend enforces last-admin protection; UI shows error if returned

**Structure**:
```typescript
export default function MembersScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const { profile } = useAuthStore();
  const [members, setMembers] = useState<OrgMember[]>([]);
  const [isLoading, setIsLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const isAdmin = currentRole === 'owner' || currentRole === 'admin';

  // Fetch members on mount / org change
  useEffect(() => {
    if (currentOrg) {
      fetchMembers();
    }
  }, [currentOrg?.id]);

  const fetchMembers = async () => { ... };
  const handleRoleChange = async (userId: number, newRole: OrgRole) => { ... };
  const handleRemoveMember = async (userId: number) => { ... };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-4xl">
        {/* Header with back button */}
        {/* Members table */}
        {/* Each row: Name, Email, Role (dropdown if admin), Joined, Remove button (if admin) */}
      </div>
    </div>
  );
}
```

**Role dropdown options**: owner, admin, manager, operator, viewer

---

### Task 4: Create OrgSettingsScreen

**File**: `frontend/src/components/OrgSettingsScreen.tsx`

**Features**:
- Fetch org details via `orgsApi.get(orgId)`
- Org name input (editable by admin only)
- Save button triggers `orgsApi.update(orgId, { name })`
- Danger zone: Delete button opens DeleteOrgModal (admin only)

**Structure**:
```typescript
export default function OrgSettingsScreen() {
  const { currentOrg, currentRole } = useOrgStore();
  const [name, setName] = useState('');
  const [isLoading, setIsLoading] = useState(false);
  const [showDeleteModal, setShowDeleteModal] = useState(false);
  const isAdmin = currentRole === 'owner' || currentRole === 'admin';

  useEffect(() => {
    if (currentOrg) {
      setName(currentOrg.name);
    }
  }, [currentOrg]);

  const handleSave = async () => { ... };
  const handleDelete = async (confirmName: string) => { ... };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        {/* Header with back button */}
        {/* Name input + Save button */}
        {/* Danger Zone section with Delete button */}
      </div>
      {showDeleteModal && (
        <DeleteOrgModal
          orgName={currentOrg?.name ?? ''}
          onConfirm={handleDelete}
          onCancel={() => setShowDeleteModal(false)}
        />
      )}
    </div>
  );
}
```

---

### Task 5: Create DeleteOrgModal

**File**: `frontend/src/components/DeleteOrgModal.tsx`

**Features**:
- Warning text about consequences
- Input: "Type {org_name} to confirm"
- Delete button disabled until name matches exactly
- Cancel button

**Structure**:
```typescript
interface DeleteOrgModalProps {
  orgName: string;
  onConfirm: (confirmName: string) => void;
  onCancel: () => void;
  isLoading?: boolean;
}

export function DeleteOrgModal({ orgName, onConfirm, onCancel, isLoading }: DeleteOrgModalProps) {
  const [confirmName, setConfirmName] = useState('');
  const canDelete = confirmName === orgName;

  return (
    <div className="fixed inset-0 z-50 flex items-center justify-center">
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black bg-opacity-50" onClick={onCancel} />

      {/* Modal */}
      <div className="relative bg-gray-800 rounded-lg shadow-xl p-6 max-w-md w-full mx-4">
        <h3 className="text-lg font-semibold text-white mb-2">Delete Organization</h3>
        <p className="text-gray-400 mb-4">
          This action cannot be undone. All members will be removed and data will be permanently deleted.
        </p>
        <label className="block text-sm text-gray-300 mb-2">
          Type <span className="font-mono font-bold text-red-400">{orgName}</span> to confirm
        </label>
        <input
          type="text"
          value={confirmName}
          onChange={(e) => setConfirmName(e.target.value)}
          placeholder="Organization name"
          className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg mb-4"
          disabled={isLoading}
        />
        <div className="flex gap-3 justify-end">
          <button onClick={onCancel} disabled={isLoading} className="...">Cancel</button>
          <button
            onClick={() => onConfirm(confirmName)}
            disabled={!canDelete || isLoading}
            className="px-4 py-2 bg-red-600 text-white rounded-lg disabled:opacity-50"
          >
            {isLoading ? 'Deleting...' : 'Delete Organization'}
          </button>
        </div>
      </div>
    </div>
  );
}
```

---

## Validation Checklist

- [ ] Members table shows all org members
- [ ] Admin can change member roles via dropdown
- [ ] Admin can remove members (backend enforces last-admin rule)
- [ ] Current user row shows "You" badge
- [ ] Non-admin sees screens but no admin controls
- [ ] Org name can be edited and saved
- [ ] Delete requires typing org name exactly
- [ ] `just frontend validate` passes

## Implementation Order

1. Task 1: Routes (App.tsx) - enables navigation
2. Task 5: DeleteOrgModal - dependency for Task 4
3. Task 4: OrgSettingsScreen - simpler, fewer moving parts
4. Task 3: MembersScreen - most complex, table + role changes
5. Task 2: Navigation links - final wiring

## Dependencies

- `orgsApi` methods already exist from Phase 3a
- `OrgMember` type already exists
- `useOrgStore` provides `currentOrg`, `currentRole`
- `useAuthStore` provides `profile` for current user ID
