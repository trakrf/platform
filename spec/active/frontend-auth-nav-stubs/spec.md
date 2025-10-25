# Feature: Frontend Auth - Navigation & Stub Pages

## Origin
This specification is based on Linear issue **TRA-88**. Part of a larger Frontend Auth effort - this is Part 1 focused on creating navigation structure and placeholder screens to unblock parallel CRUD development.

## Outcome
Add Assets and Locations navigation tabs with stub pages that enable the cofounder to start building real CRUD screens without waiting for full navigation infrastructure.

## User Story
As a **platform developer**
I want **Assets and Locations tabs with navigable stub pages**
So that **parallel development can begin on CRUD features without blocking on navigation implementation**

## Context

**Discovery**: Frontend Auth project needs scaffolding to enable parallel work streams.

**Current**: Navigation exists for Home, Inventory, Locate, Barcode, Settings, and Help tabs.

**Desired**: Extend navigation with Assets and Locations tabs that route to placeholder screens, maintaining existing design patterns.

**Strategic Goal**: Quick win (~1.5 hours) that unblocks cofounder to replace stubs with real screens later.

## Technical Requirements

### Navigation Changes
- Add **Assets** tab to TabNavigation component
  - Icon: package or box icon (from existing icon library)
  - Position: After Barcode, before Locations

- Add **Locations** tab to TabNavigation component
  - Icon: map-pin icon
  - Position: After Assets, before Settings

- **Final tab order**: Home → Inventory → Locate → Barcode → **Assets** → **Locations** → Settings → Help

### Routing
- Maintain **hash-based routing** (no React Router dependency)
- Add route `#assets` → AssetsScreen component
- Add route `#locations` → LocationsScreen component
- Update App.tsx routing logic

### Stub Components

Create two placeholder screens that:
- Show clear "coming soon" messaging
- Explain what the feature will do
- Match existing app design patterns:
  - Dark theme
  - Centered layout
  - Consistent typography

**AssetsScreen.tsx**:
- Component name: `AssetsScreen`
- Message: Explain Assets CRUD functionality is coming
- Style: Match existing screen components

**LocationsScreen.tsx**:
- Component name: `LocationsScreen`
- Message: Explain Locations CRUD functionality is coming
- Style: Match existing screen components

### Design Constraints
- Match existing dark theme
- Use existing component patterns
- Maintain accessibility (ARIA labels, keyboard navigation)
- No authentication checks in this phase (pure navigation)

### Non-Requirements (Explicit Scope Limits)
- ❌ No React Router migration
- ❌ No authentication/authorization checks
- ❌ No real CRUD functionality
- ❌ No API integration
- ❌ No data models

## Implementation Notes

### File Changes Expected
```
frontend/src/
├── components/TabNavigation.tsx    # Add Assets & Locations tabs
├── screens/
│   ├── AssetsScreen.tsx           # New stub component
│   └── LocationsScreen.tsx        # New stub component
└── App.tsx                         # Add hash routing for #assets, #locations
```

### Code Pattern Example

**Tab Addition** (pseudo-code):
```typescript
// In TabNavigation.tsx
{
  icon: <PackageIcon />,
  label: 'Assets',
  path: '#assets',
  ariaLabel: 'Navigate to Assets'
}
```

**Stub Component** (pseudo-code):
```typescript
// AssetsScreen.tsx
export function AssetsScreen() {
  return (
    <CenteredLayout>
      <Heading>Assets Management</Heading>
      <Text>
        Asset tracking and management features coming soon.
        This page will allow you to view, create, and manage assets.
      </Text>
    </CenteredLayout>
  );
}
```

**Routing** (pseudo-code):
```typescript
// In App.tsx
case '#assets':
  return <AssetsScreen />;
case '#locations':
  return <LocationsScreen />;
```

## Validation Criteria

- [ ] Assets tab appears in navigation bar
- [ ] Locations tab appears in navigation bar
- [ ] Tab order matches: Home → Inventory → Locate → Barcode → Assets → Locations → Settings → Help
- [ ] Clicking Assets tab navigates to `#assets` route
- [ ] Clicking Locations tab navigates to `#locations` route
- [ ] AssetsScreen displays with placeholder content
- [ ] LocationsScreen displays with placeholder content
- [ ] Visual design matches existing screens (dark theme, centered layout)
- [ ] Icons are appropriate (package/box for Assets, map-pin for Locations)
- [ ] Navigation remains functional for all existing tabs
- [ ] No console errors or warnings
- [ ] Keyboard navigation works (tab through nav items)

## Success Metrics

### Functional
- All tabs navigate correctly
- Hash routing works for new routes
- Existing functionality unaffected

### UX
- Tabs are clearly labeled
- Stubs set appropriate expectations
- Design consistency maintained

### Team Impact
- **Unblocks cofounder** to start building real Assets/Locations screens
- Stub pages can be replaced incrementally without refactoring navigation
- Clear handoff point for CRUD implementation

## Dependencies
**None** - can be built immediately!

## Next Steps (Post-Implementation)
1. Cofounder replaces AssetsScreen stub with real CRUD implementation
2. Cofounder replaces LocationsScreen stub with real CRUD implementation
3. Part 4 (Integration) will add authentication checks to protect these routes

## Linear Issue Reference
- **Issue**: [TRA-88](https://linear.app/trakrf/issue/TRA-88/frontend-auth-navigation-and-stub-pages)
- **Branch**: `miks2u/tra-88-frontend-auth-navigation-stub-pages`
- **Priority**: High
- **Status**: In Progress
- **Estimate**: ~1.5 hours
- **Labels**: frontend

## Key Decisions from Issue
- **Decision**: Use hash-based routing (defer React Router migration)
  - **Rationale**: Minimize scope, maintain existing patterns

- **Decision**: No auth checks in this phase
  - **Rationale**: Auth protection comes in Part 4, keep concerns separated

- **Decision**: Stubs over skeletons
  - **Rationale**: Clear messaging about "coming soon" vs broken UI

## Architecture Notes
This is **scaffolding work** - intentionally minimal implementation to enable parallel development. The goal is unblocking, not completeness.
