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

  it('should set type to unknown for new tags initially', () => {
    useTagStore.getState().addTag({ epc: 'UNKNOWN123' });
    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('unknown');
  });

  it('should queue all new tags for lookup', () => {
    useTagStore.getState().addTag({ epc: 'NEWTAG123' });

    // All new tags should be queued for classification via lookup API
    expect(useTagStore.getState()._lookupQueue.has('NEWTAG123')).toBe(true);
  });

  it('should preserve existing type when updating tag reads', () => {
    // Manually set a tag as classified
    useTagStore.setState({
      tags: [{
        epc: 'LOCATION999',
        displayEpc: 'LOCATION999',
        count: 1,
        rssi: -60,
        source: 'rfid',
        type: 'location',
        locationId: 1,
        locationName: 'Storage Room',
      }]
    });

    // Update with another read (same tag scanned again)
    useTagStore.getState().addTag({ epc: 'LOCATION999', rssi: -55 });

    const tag = useTagStore.getState().tags[0];
    expect(tag.type).toBe('location'); // Type preserved
    expect(tag.count).toBe(2);
    expect(tag.rssi).toBe(-55);
  });

  it('should not re-queue existing tags for lookup', () => {
    // Add a tag first
    useTagStore.getState().addTag({ epc: 'EXISTINGTAG' });
    expect(useTagStore.getState()._lookupQueue.has('EXISTINGTAG')).toBe(true);

    // Clear the queue
    useTagStore.setState({ _lookupQueue: new Set<string>() });

    // Add same tag again (another read)
    useTagStore.getState().addTag({ epc: 'EXISTINGTAG' });

    // Should NOT be queued again since tag already exists
    expect(useTagStore.getState()._lookupQueue.has('EXISTINGTAG')).toBe(false);
  });
});

describe('TagStore - mergeReconciliationTags', () => {
  beforeEach(() => {
    useTagStore.getState().clearTags();
  });

  it('should mark RFID-scanned tags as reconciled: true when merged', () => {
    // Add a tag via RFID scan (source: 'rfid')
    useTagStore.getState().addTag({ epc: 'DEADBEEF', rssi: -60 });
    const before = useTagStore.getState().tags[0];
    expect(before.source).toBe('rfid');

    // Merge reconciliation data for this tag
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'DEADBEEF', count: 0, found: false, description: 'Laptop' },
    ]);

    const after = useTagStore.getState().tags.find(t => t.epc === 'DEADBEEF');
    expect(after?.reconciled).toBe(true); // Was bug: source === 'scan' → always false
    expect(after?.description).toBe('Laptop');
  });

  it('should leave reconciliation-only tags as reconciled: false', () => {
    // Merge a tag that was NOT previously scanned
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'CAFE7731', count: 0, found: false, description: 'Monitor' },
    ]);

    const tag = useTagStore.getState().tags.find(t => t.epc === 'CAFE7731');
    expect(tag?.reconciled).toBe(false);
    expect(tag?.source).toBe('reconciliation');
  });

  it('should pass assetIdentifier through to TagInfo', () => {
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ]);

    const tag = useTagStore.getState().tags.find(t => t.epc === 'DEADBEEF');
    expect(tag?.assetIdentifier).toBe('ASSET-0003');
  });

  it('should set assetIdentifier on existing scanned tags during merge', () => {
    // Scan a tag first
    useTagStore.getState().addTag({ epc: 'DEADBEEF', rssi: -50 });

    // Merge reconciliation with assetIdentifier
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'DEADBEEF', assetIdentifier: 'ASSET-0003', count: 0, found: false },
    ]);

    const tag = useTagStore.getState().tags.find(t => t.epc === 'DEADBEEF');
    expect(tag?.assetIdentifier).toBe('ASSET-0003');
    expect(tag?.reconciled).toBe(true);
  });

  it('should promote reconciliation stub when scanned tag matches (import-then-scan)', () => {
    // Step 1: Import CSV — creates reconciliation stub with short EPC
    useTagStore.getState().mergeReconciliationTags([
      { epc: '10018', assetIdentifier: 'ASSET-0020', count: 0, found: false, description: 'sss' },
    ]);

    const stub = useTagStore.getState().tags[0];
    expect(stub.source).toBe('reconciliation');
    expect(stub.reconciled).toBe(false);
    expect(stub.count).toBe(0);

    // Step 2: Scan tag — full EPC with leading zeros should match the stub
    useTagStore.getState().addTag({ epc: '000000000000000000010018', rssi: -45 });

    // Should have ONE entry (merged), not two
    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(1);

    const tag = tags[0];
    expect(tag.source).toBe('rfid');           // Promoted from 'reconciliation'
    expect(tag.reconciled).toBe(true);          // Marked as found
    expect(tag.assetIdentifier).toBe('ASSET-0020'); // Kept from stub
    expect(tag.description).toBe('sss');        // Kept from stub
    expect(tag.count).toBe(1);                  // First scan
    expect(tag.rssi).toBe(-45);                 // From scan
    expect(tag.epc).toBe('000000000000000000010018'); // Updated to full EPC
  });

  it('should not duplicate when scanning tag that already has reconciliation stub', () => {
    // Import two tags for same asset
    useTagStore.getState().mergeReconciliationTags([
      { epc: '10018', assetIdentifier: 'ASSET-0020', count: 0, found: false },
      { epc: '10019', assetIdentifier: 'ASSET-0020', count: 0, found: false },
    ]);
    expect(useTagStore.getState().tags).toHaveLength(2);

    // Scan first tag
    useTagStore.getState().addTag({ epc: '000000000000000000010018', rssi: -50 });

    // Still 2 entries (one promoted, one still stub)
    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(2);

    const scanned = tags.find(t => t.reconciled === true);
    const missing = tags.find(t => t.reconciled === false);
    expect(scanned).toBeDefined();
    expect(missing).toBeDefined();
    expect(scanned!.assetIdentifier).toBe('ASSET-0020');
    expect(missing!.epc).toBe('10019');
  });
});