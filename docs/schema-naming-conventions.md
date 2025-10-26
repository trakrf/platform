# TrakRF Schema Naming Conventions

**Last Updated**: 2025-10-26

## Naming Philosophy

We use `org_*` abbreviations consistently throughout the codebase for brevity and readability. "Organization" is a widely-recognized abbreviation that causes no confusion.

---

## Tables

### Full Names (Plural)
- `organizations` - Application customer/tenant entities
- `users` - User authentication entities
- `locations` - Physical places
- `assets` - Trackable entities
- `identifiers` - Physical/logical identifiers (RFID, barcodes, etc.)
- `scan_devices` - RFID readers, barcode scanners, etc.
- `scan_points` - Sensors/antennas on scan devices

### Abbreviated Names (Plural)
- `org_users` - User-organization membership junction table
- `identifier_scans` - Raw sensor scan events (hypertable)
- `asset_scans` - Derived business scan events (hypertable)

---

## Columns

### Foreign Keys
Always abbreviated:
- `org_id` - References `organizations(id)`
- `user_id` - References `users(id)`
- `asset_id` - References `assets(id)`
- `location_id` - References `locations(id)`
- `scan_device_id` - References `scan_devices(id)`
- `scan_point_id` - References `scan_points(id)`
- `identifier_id` - References `identifiers(id)`

### API Fields
Always abbreviated:
- `org_name` - Organization name (used in signup API)
- `org_id` - Organization ID

---

## Row-Level Security

### Session Variables
- `app.current_org_id` - Current organization context
- `app.current_user_id` - Current user context

### Policy Names
Pattern: `org_isolation_{table_name}`

Examples:
- `org_isolation_assets`
- `org_isolation_locations`
- `org_isolation_scan_devices`
- `org_isolation_org_users`

---

## API Endpoints & Request Bodies

### Auth Endpoints

**Signup**:
```json
POST /api/v1/auth/signup
{
  "email": "user@example.com",
  "password": "securepass123",
  "org_name": "Acme Corporation"
}
```

**Login**:
```json
POST /api/v1/auth/login
{
  "email": "user@example.com",
  "password": "securepass123"
}
```

---

## Frontend Code

### TypeScript Interfaces
```typescript
// API Request Types
interface SignupRequest {
  email: string;
  password: string;
  org_name: string;  // abbreviated
}

// Store Types
interface AuthState {
  user: User | null;
  token: string | null;
  signup: (email: string, password: string, orgName: string) => Promise<void>;
}
```

### React Components
```tsx
// Form fields
<input name="org_name" placeholder="Organization Name" />

// API calls
authStore.signup({ email, password, org_name: orgName });
```

---

## Backend Code (Go)

### Struct Tags
```go
type SignupRequest struct {
    Email    string `json:"email" validate:"required,email"`
    Password string `json:"password" validate:"required,min=8"`
    OrgName  string `json:"org_name" validate:"required,min=2"`  // JSON field abbreviated
}
```

### Database Models
```go
type Organization struct {
    ID        int       `db:"id"`
    Name      string    `db:"name"`
    Domain    string    `db:"domain"`
    CreatedAt time.Time `db:"created_at"`
}

type OrgUser struct {
    OrgID  int    `db:"org_id"`  // abbreviated column
    UserID int    `db:"user_id"`
    Role   string `db:"role"`
}
```

---

## Migration Files

### Naming Pattern
- `000002_organizations.up.sql` - Full name for main entity
- `000004_org_users.up.sql` - Abbreviated for junction table
- `000006_scan_devices.up.sql` - Descriptive compound name
- `000010_identifier_scans.up.sql` - Descriptive compound name

---

## Consistency Rules

### ✅ Always Abbreviate
- Foreign key columns: `org_id`
- Junction tables: `org_users`
- API fields: `org_name`
- Session variables: `app.current_org_id`

### ✅ Always Use Full Name
- Table names (except junctions): `organizations`, `locations`, `assets`
- Go struct names: `Organization`, `OrgUser`
- Policy names: `org_isolation_*`

### ✅ Context Matters
- **SQL**: `org_id`, `org_users`
- **Go**: `OrgID`, `OrgName`, `OrgUser`
- **JSON**: `org_name`, `org_id`
- **TypeScript**: `org_name`, `orgName` (camelCase in code)

---

## Examples in Context

### SQL Query
```sql
SELECT
    a.id,
    a.name,
    a.org_id,
    o.name as org_name
FROM assets a
JOIN organizations o ON o.id = a.org_id
WHERE a.org_id = current_setting('app.current_org_id')::INT;
```

### Go Code
```go
func (s *AuthService) Signup(req SignupRequest) (*AuthResponse, error) {
    // Create organization
    org := &Organization{
        Name: req.OrgName,  // OrgName field, org_name JSON tag
    }

    // Create org_users membership
    membership := &OrgUser{
        OrgID:  org.ID,
        UserID: user.ID,
        Role:   "owner",
    }

    return &AuthResponse{Token: token, User: user}, nil
}
```

### Frontend TypeScript
```typescript
const signup = async (email: string, password: string, orgName: string) => {
  const response = await authApi.signup({
    email,
    password,
    org_name: orgName,  // snake_case for API
  });
  return response.data;
};
```

---

## Rationale

**Why abbreviate?**
- `org_id` typed frequently (every foreign key, every query)
- `org_name` common in forms and APIs
- `org_users` clear and concise
- "org" is universally recognized

**Why NOT abbreviate table names?**
- `organizations` reads naturally in queries
- Table names less frequently typed
- Consistency with other full names (locations, assets)

**Result**: Balance between brevity (columns/fields) and readability (tables/entities)

---

**Last Updated**: 2025-10-26
