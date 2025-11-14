# ltree Usage Examples for Locations API

## Overview
The locations API uses PostgreSQL's ltree extension to efficiently query hierarchical data in a single query. This provides significant performance benefits over traditional recursive queries.

## API Endpoint

### Get Location with Relations
```
GET /api/v1/locations/{id}?include=relations
```

**Query Parameters:**
- `include=relations` - Include both ancestors and children in a single optimized query

## Response Structure

### Example Hierarchy
```
USA (id: 1)
├── California (id: 2)
│   ├── Warehouse 1 (id: 3)
│   │   ├── Zone A (id: 4)
│   │   └── Zone B (id: 5)
│   └── Warehouse 2 (id: 6)
└── Texas (id: 7)
```

### Requesting Warehouse 1 (id: 3)

**Request:**
```bash
curl -H "Authorization: Bearer $TOKEN" \
  'http://localhost:8080/api/v1/locations/3?include=relations'
```

**Response:**
```json
{
  "data": {
    "id": 3,
    "name": "Warehouse 1",
    "identifier": "warehouse_1",
    "path": "usa.california.warehouse_1",
    "depth": 3,
    "parent_location_id": 2,
    "ancestors": [
      {
        "id": 1,
        "name": "USA",
        "identifier": "usa",
        "path": "usa",
        "depth": 1,
        "parent_location_id": null
      },
      {
        "id": 2,
        "name": "California",
        "identifier": "california",
        "path": "usa.california",
        "depth": 2,
        "parent_location_id": 1
      }
    ],
    "children": [
      {
        "id": 4,
        "name": "Zone A",
        "identifier": "zone_a",
        "path": "usa.california.warehouse_1.zone_a",
        "depth": 4,
        "parent_location_id": 3
      },
      {
        "id": 5,
        "name": "Zone B",
        "identifier": "zone_b",
        "path": "usa.california.warehouse_1.zone_b",
        "depth": 4,
        "parent_location_id": 3
      }
    ]
  }
}
```

## How ltree Works

### The Magic Query
The implementation uses a single SQL query with ltree operators:

```sql
WITH target AS (
    SELECT id, path FROM locations WHERE id = 3
)
SELECT l.*,
       CASE
           WHEN l.id = 3 THEN 'target'
           WHEN l.path @> (SELECT path FROM target) THEN 'ancestor'
           WHEN l.parent_location_id = 3 THEN 'child'
       END as relation_type
FROM locations l, target t
WHERE l.deleted_at IS NULL
  AND (
      l.id = 3                           -- target location
      OR l.path @> t.path                -- ancestors (ltree: @> is ancestor of)
      OR l.parent_location_id = 3        -- immediate children
  )
```

### ltree Operators Used

| Operator | Meaning | Example |
|----------|---------|---------|
| `@>` | Is ancestor of | `'usa' @> 'usa.california'` → true |
| `<@` | Is descendant of | `'usa.california' <@ 'usa'` → true |
| `||` | Concatenate | `'usa' || 'california'` → 'usa.california' |

### Performance Benefits

**Traditional Approach (Multiple Queries):**
1. Get location by ID
2. Recursive query for ancestors
3. Query for immediate children

**Result:** 3 database round trips

**ltree Approach (Single Query):**
1. Get location + ancestors + children in one query

**Result:** 1 database round trip (3x faster!)

## Use Cases

### Building a Breadcrumb Navigation
```typescript
const { data } = await fetch(`/api/v1/locations/${locationId}?include=relations`);

// Render breadcrumb from ancestors
const breadcrumb = [...data.ancestors, data]
  .map(loc => loc.name)
  .join(' > ');

console.log(breadcrumb); // "USA > California > Warehouse 1"
```

### Building a Tree Selector
```typescript
const { data } = await fetch(`/api/v1/locations/${locationId}?include=relations`);

// Show current location with expandable children
return (
  <TreeNode location={data}>
    {data.children.map(child => (
      <TreeNode key={child.id} location={child} />
    ))}
  </TreeNode>
);
```

### Path-Based Filtering
The `path` field supports powerful pattern matching:

```sql
-- Find all locations under California
SELECT * FROM locations
WHERE path <@ 'usa.california';

-- Find all warehouses at any level
SELECT * FROM locations
WHERE path ~ '*.warehouse_*';

-- Get all locations at depth 3
SELECT * FROM locations
WHERE depth = 3;
```

## Testing

Run the comprehensive test suite:
```bash
go test -run "TestGetLocationWithRelations" ./internal/storage -v
```

Tests include:
- ✅ Mid-tree location (has both ancestors and children)
- ✅ Root location (no ancestors, only children)
- ✅ Leaf location (has ancestors, no children)
- ✅ Not found scenario

All **25 location tests passing** with ltree integration!

## Next Steps

1. **Frontend Integration**: Use `?include=relations` for tree components
2. **Caching**: Consider caching ancestor paths for frequently accessed locations
3. **Search**: Leverage ltree pattern matching for location search
