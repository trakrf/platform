import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useTagStore } from './tagStore';
import { useAuthStore } from './authStore';
import { useLocationStore } from './locations/locationStore';
import { lookupApi } from '@/lib/api/lookup';
import { LOCATE_TEST_TAG, PRIMARY_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';
import type { Location } from '@/types/locations';

// Mock the lookup API
vi.mock('@/lib/api/lookup');

// Helper to create a minimal mock location
const createMockLocation = (id: number, name: string, tagEpc?: string): Location => ({
  id,
  org_id: 1,
  identifier: `loc_${id}`,
  name,
  description: '',
  parent_location_id: null,
  path: `loc_${id}`,
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  identifiers: tagEpc ? [{ id: 1, type: 'rfid', value: tagEpc, is_active: true }] : [],
});

describe('TagStore - Leading Zero Trimming', () => {
  beforeEach(() => {
    // Clear tags before each test
    useTagStore.getState().clearTags();
  });

  it('should trim leading zeros from EPC for display', () => {
    const testEPC = EPC_FORMATS.toFullEPC(LOCATE_TEST_TAG);

    // Add a tag with leading zeros
    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -60,
      count: 1
    });

    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(1);

    const tag = tags[0];
    expect(tag.epc).toBe(testEPC); // Full EPC preserved
    expect(tag.displayEpc).toBe(LOCATE_TEST_TAG); // Leading zeros trimmed
  });

  it('should handle EPCs with all zeros except last digit', () => {
    const testEPC = '000000000000000000000001';

    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -55
    });

    const tag = useTagStore.getState().tags[0];
    expect(tag.displayEpc).toBe('1');
  });

  it('should handle EPCs that are all zeros', () => {
    const testEPC = '000000000000000000000000';

    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -70
    });

    const tag = useTagStore.getState().tags[0];
    expect(tag.displayEpc).toBe('0'); // Should keep at least one zero
  });

  it('should update displayEpc when updating existing tag', () => {
    const testEPC = EPC_FORMATS.toFullEPC(PRIMARY_TEST_TAG);

    // Add initial tag
    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -60
    });

    // Update the same tag (simulating another read)
    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -58
    });

    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(1);

    const tag = tags[0];
    expect(tag.count).toBe(2); // Count should be incremented
    expect(tag.displayEpc).toBe(PRIMARY_TEST_TAG); // Display EPC should still be trimmed
  });

  it('should handle mixed case hex values', () => {
    const testEPC = '00000000000000000001A0B2';

    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -65
    });

    const tag = useTagStore.getState().tags[0];
    expect(tag.displayEpc).toBe('1A0B2');
  });

  it('should preserve odd number of digits after trimming', () => {
    // Test case where trimming results in odd number of digits
    const testEPC = '000000000000000000000123';

    useTagStore.getState().addTag({
      epc: testEPC,
      rssi: -50
    });

    const tag = useTagStore.getState().tags[0];
    expect(tag.displayEpc).toBe('123'); // 3 digits (odd) is fine
  });
});

describe('TagStore - Auth Guard for Lookup', () => {
  beforeEach(() => {
    // Clear tags and reset lookup queue
    useTagStore.setState({
      tags: [],
      _lookupQueue: new Set<string>(),
      _isLookupInProgress: false,
      _lookupTimer: null
    });
    // Reset auth state
    useAuthStore.setState({ isAuthenticated: false });
    vi.clearAllMocks();
  });

  it('should skip API call when not authenticated', async () => {
    // Set up queue with EPCs
    useTagStore.setState({
      _lookupQueue: new Set(['EPC001', 'EPC002'])
    });

    // Ensure not authenticated
    useAuthStore.setState({ isAuthenticated: false });

    // Mock the API to verify it's NOT called
    const lookupSpy = vi.mocked(lookupApi.byTags);

    await useTagStore.getState()._flushLookupQueue();

    // API should NOT be called
    expect(lookupSpy).not.toHaveBeenCalled();

    // Queue should still have items (not cleared)
    expect(useTagStore.getState()._lookupQueue.size).toBe(2);
  });

  it('should call API when authenticated', async () => {
    // Add a tag to trigger queue setup
    useTagStore.getState().addTag({
      epc: 'EPC001',
      rssi: -60
    });

    // Set up queue directly
    useTagStore.setState({
      _lookupQueue: new Set(['EPC001'])
    });

    useAuthStore.setState({ isAuthenticated: true });

    vi.mocked(lookupApi.byTags).mockResolvedValue({
      data: { data: {} }
    } as any);

    await useTagStore.getState()._flushLookupQueue();

    expect(lookupApi.byTags).toHaveBeenCalled();
  });

  it('should not clear queue when skipping due to auth', async () => {
    const testEpcs = new Set(['TEST001', 'TEST002', 'TEST003']);
    useTagStore.setState({
      _lookupQueue: testEpcs
    });

    useAuthStore.setState({ isAuthenticated: false });

    await useTagStore.getState()._flushLookupQueue();

    // Queue should remain intact for when user logs in
    const queue = useTagStore.getState()._lookupQueue;
    expect(queue.size).toBe(3);
    expect(queue.has('TEST001')).toBe(true);
    expect(queue.has('TEST002')).toBe(true);
    expect(queue.has('TEST003')).toBe(true);
  });
});

describe('TagStore - Tag Classification (TRA-312)', () => {
  beforeEach(() => {
    // Clear tags and location cache
    useTagStore.getState().clearTags();
    useLocationStore.getState().invalidateCache();
    useTagStore.setState({
      _lookupQueue: new Set<string>(),
      _isLookupInProgress: false,
      _lookupTimer: null
    });
    vi.clearAllMocks();
  });

  it('should set type to unknown for unrecognized tags', () => {
    useTagStore.getState().addTag({ epc: 'UNKNOWN123' });
    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('unknown');
  });

  it('should set type to location when EPC matches location tag', () => {
    // Setup: populate location cache with a location that has tag identifier
    const location = createMockLocation(1, 'Warehouse A - Rack 12', 'LOCATION123');
    useLocationStore.getState().setLocations([location]);

    useTagStore.getState().addTag({ epc: 'LOCATION123' });
    const tag = useTagStore.getState().tags[0];

    expect(tag.type).toBe('location');
    expect(tag.locationId).toBe(1);
    expect(tag.locationName).toBe('Warehouse A - Rack 12');
  });

  it('should not queue location tags for asset lookup', () => {
    // Setup location cache
    const location = createMockLocation(1, 'Warehouse A', 'LOCATIONEPC');
    useLocationStore.getState().setLocations([location]);

    useTagStore.getState().addTag({ epc: 'LOCATIONEPC' });

    // Verify tag was NOT queued for lookup
    expect(useTagStore.getState()._lookupQueue.size).toBe(0);
  });

  it('should queue unknown tags for asset lookup', () => {
    useTagStore.getState().addTag({ epc: 'UNKNOWNEPC' });

    // Verify tag WAS queued for lookup
    expect(useTagStore.getState()._lookupQueue.has('UNKNOWNEPC')).toBe(true);
  });

  it('should preserve existing type when updating tag reads', () => {
    // First add as location
    const location = createMockLocation(1, 'Storage Room', 'LOCATION999');
    useLocationStore.getState().setLocations([location]);

    useTagStore.getState().addTag({ epc: 'LOCATION999', rssi: -60 });
    expect(useTagStore.getState().tags[0].type).toBe('location');

    // Update with another read (same tag scanned again)
    useTagStore.getState().addTag({ epc: 'LOCATION999', rssi: -55 });

    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('location');
    expect(tag.count).toBe(2);
    expect(tag.rssi).toBe(-55);
  });

  it('should re-enrich unknown tags with locations after _enrichTagsWithLocations', () => {
    // Add tag while location cache is empty
    useTagStore.getState().addTag({ epc: 'LATERKNOWN' });
    expect(useTagStore.getState().tags[0].type).toBe('unknown');

    // Populate location cache
    const location = createMockLocation(2, 'New Location', 'LATERKNOWN');
    useLocationStore.getState().setLocations([location]);

    // Run enrichment
    useTagStore.getState()._enrichTagsWithLocations();

    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('location');
    expect(tag.locationId).toBe(2);
    expect(tag.locationName).toBe('New Location');
  });

  it('should not re-enrich tags already classified as asset', () => {
    // Manually set a tag as asset type
    useTagStore.setState({
      tags: [{
        epc: 'ASSET123',
        displayEpc: 'ASSET123',
        count: 1,
        source: 'rfid',
        type: 'asset',
        assetId: 99,
        assetName: 'Test Asset',
      }]
    });

    // Populate location cache with same EPC (shouldn't happen in real life)
    const location = createMockLocation(5, 'Should Not Match', 'ASSET123');
    useLocationStore.getState().setLocations([location]);

    // Run enrichment
    useTagStore.getState()._enrichTagsWithLocations();

    // Should still be asset, not location
    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('asset');
    expect(tag.assetId).toBe(99);
  });
});