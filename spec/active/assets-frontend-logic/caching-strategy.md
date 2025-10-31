# Asset Store Caching Strategy

## TTL Configuration

**Cache Duration**: 1 hour (60 minutes)

**Rationale**:
- Assets are relatively static data (change rarely)
- Reduces unnecessary API calls
- Improves user experience with instant data availability
- Balances freshness vs. performance

## Cache Population Strategy (Phase 4)

### On Asset Creation (POST /api/v1/assets)

**✅ Immediate Cache Update** (Optimistic + Confirmed):

```typescript
// In Phase 4 API integration hook
const createAsset = async (data: CreateAssetRequest) => {
  const response = await assetApi.create(data);

  // CRITICAL: Immediately add to cache
  useAssetStore.getState().addAsset(response.data);

  return response.data;
};
```

**Benefits**:
- No refetch needed after creation
- User sees new asset immediately in lists
- Cache stays synchronized with backend

### On Asset Update (PUT /api/v1/assets/:id)

**✅ Immediate Cache Update**:

```typescript
const updateAsset = async (id: number, updates: UpdateAssetRequest) => {
  const response = await assetApi.update(id, updates);

  // CRITICAL: Update cache with server response
  useAssetStore.getState().updateCachedAsset(id, response.data);

  return response.data;
};
```

**Benefits**:
- UI reflects server state immediately
- Handles server-side transformations (timestamps, defaults)
- No stale data

### On Asset Deletion (DELETE /api/v1/assets/:id)

**✅ Immediate Cache Removal**:

```typescript
const deleteAsset = async (id: number) => {
  await assetApi.delete(id);

  // CRITICAL: Remove from cache immediately
  useAssetStore.getState().removeAsset(id);
};
```

### On List Fetch (GET /api/v1/assets)

**✅ Bulk Cache Population**:

```typescript
const fetchAssets = async () => {
  const response = await assetApi.list();

  // CRITICAL: Bulk add to cache (sets lastFetched timestamp)
  useAssetStore.getState().addAssets(response.data);

  return response.data;
};
```

### On Single Asset Fetch (GET /api/v1/assets/:id)

**✅ Check Cache First, Then Fetch**:

```typescript
const fetchAssetById = async (id: number) => {
  // Check cache first
  const cached = useAssetStore.getState().getAssetById(id);
  if (cached) {
    return cached;
  }

  // Cache miss - fetch from API
  const response = await assetApi.getById(id);
  useAssetStore.getState().addAsset(response.data);

  return response.data;
};
```

## Cache Invalidation Triggers

### When to Call `invalidateCache()`

**✅ Explicit invalidation scenarios**:
1. **Bulk Upload Complete**: After CSV import completes
2. **User Logout**: Clear all user data
3. **Manual Refresh**: User clicks "Refresh" button
4. **Error Recovery**: After 401/403 errors

**❌ Do NOT invalidate on**:
- Individual CRUD operations (use targeted updates instead)
- Navigation between pages
- Filter/sort changes

### Bulk Upload Special Case

```typescript
const handleBulkUploadComplete = async (jobId: string) => {
  const status = await assetApi.getJobStatus(jobId);

  if (status.status === 'completed') {
    // Clear cache - too many changes to track individually
    useAssetStore.getState().invalidateCache();

    // Refetch to populate cache
    await fetchAssets();
  }
};
```

## Cache Consistency Patterns

### 1. Server is Source of Truth

**Always use server response to update cache**:

```typescript
// ❌ WRONG - Don't trust optimistic updates
const updateAsset = async (id: number, updates: UpdateAssetRequest) => {
  useAssetStore.getState().updateCachedAsset(id, updates); // Risky!
  await assetApi.update(id, updates);
};

// ✅ CORRECT - Server response is truth
const updateAsset = async (id: number, updates: UpdateAssetRequest) => {
  const response = await assetApi.update(id, updates);
  useAssetStore.getState().updateCachedAsset(id, response.data); // Safe!
};
```

### 2. Error Handling

**On API Errors - Do NOT Update Cache**:

```typescript
const createAsset = async (data: CreateAssetRequest) => {
  try {
    const response = await assetApi.create(data);
    useAssetStore.getState().addAsset(response.data);
    return response.data;
  } catch (error) {
    // Do NOT update cache on error
    throw error;
  }
};
```

### 3. Race Condition Prevention

**Use timestamp-based staleness checks**:

```typescript
const isCacheStale = () => {
  const { cache } = useAssetStore.getState();
  const now = Date.now();
  return now - cache.lastFetched > cache.ttl;
};

const fetchAssetsIfStale = async () => {
  if (isCacheStale()) {
    return await fetchAssets();
  }
  return useAssetStore.getState().cache.byId.values();
};
```

## Performance Optimization

### Lazy Loading Pattern

```typescript
// In React component
useEffect(() => {
  const fetchIfNeeded = async () => {
    const cached = useAssetStore.getState().cache.byId;

    // Only fetch if cache is empty OR expired
    if (cached.size === 0 || isCacheStale()) {
      await fetchAssets();
    }
  };

  fetchIfNeeded();
}, []);
```

### Selective Fetching

```typescript
// Fetch only if specific asset not in cache
const ensureAssetLoaded = async (id: number) => {
  const cached = useAssetStore.getState().getAssetById(id);
  if (!cached) {
    await fetchAssetById(id);
  }
  return useAssetStore.getState().getAssetById(id)!;
};
```

## LocalStorage Persistence Benefits

**Cross-Session Caching**:
- User closes browser → Cache persists in LocalStorage
- User reopens app → Cache loads instantly (if not expired)
- No API call needed if TTL not exceeded

**Offline Resilience**:
- User can browse cached assets even if API is down
- Read-only operations work offline
- Graceful degradation

## Phase 4 Implementation Checklist

Phase 4 API integration should:

- [ ] Call `addAsset()` after successful create
- [ ] Call `updateCachedAsset()` after successful update
- [ ] Call `removeAsset()` after successful delete
- [ ] Call `addAssets()` after list fetch
- [ ] Check cache before API calls (reduce redundant fetches)
- [ ] Call `invalidateCache()` after bulk upload
- [ ] Implement `isCacheStale()` helper
- [ ] Add error boundaries (don't cache on error)
- [ ] Test TTL enforcement (1 hour expiration)
- [ ] Verify LocalStorage persistence across sessions

## Monitoring & Debugging

**Cache Health Checks**:

```typescript
// Developer console helpers
window.debugAssetCache = () => {
  const { cache } = useAssetStore.getState();
  console.log({
    totalAssets: cache.byId.size,
    lastFetched: new Date(cache.lastFetched).toISOString(),
    ttl: `${cache.ttl / 1000 / 60} minutes`,
    isExpired: Date.now() - cache.lastFetched > cache.ttl,
    types: Array.from(cache.byType.keys()),
    activeCount: cache.activeIds.size,
  });
};
```

## Summary

**Key Principles**:
1. **Cache aggressively** (1-hour TTL for static data)
2. **Update immediately** (don't wait for refetch)
3. **Trust the server** (use API response, not optimistic data)
4. **Invalidate sparingly** (only for bulk changes)
5. **Persist locally** (LocalStorage for cross-session caching)

This strategy ensures:
- ✅ Fast user experience (instant data access)
- ✅ Reduced API load (fewer redundant fetches)
- ✅ Data consistency (server is source of truth)
- ✅ Offline resilience (LocalStorage fallback)
