import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useTagStore } from './tagStore';
import { useAuthStore } from './authStore';
import { useLocationStore } from './locations/locationStore';
import { lookupApi } from '@/lib/api/lookup';
import { ensureOrgContext } from '@/lib/auth/orgContext';
import { LOCATE_TEST_TAG, PRIMARY_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';
import type { Location } from '@/types/locations';

// Mock the lookup API
vi.mock('@/lib/api/lookup');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(42),
  refreshOrgToken: vi.fn().mockResolvedValue(true),
  getTokenOrgId: vi.fn().mockReturnValue(42),
  setOrgToken: vi.fn().mockResolvedValue(undefined),
}));

// Helper to create a minimal mock location
const createMockLocation = (id: number, name: string, tagEpc?: string): Location => ({
  id,
  org_id: 1,
  external_key: `loc_${id}`,
  name,
  description: '',
  parent_id: null,
  tree_path: `loc_${id}`,
  depth: 1,
  valid_from: '2024-01-01',
  valid_to: null,
  is_active: true,
  metadata: {},
  created_at: '2024-01-01T00:00:00Z',
  updated_at: '2024-01-01T00:00:00Z',
  tags: tagEpc ? [{ id: 1, tag_type: 'rfid', value: tagEpc }] : [],
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
    vi.mocked(ensureOrgContext).mockResolvedValue(42);
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
    // Authenticate first (with empty tags) so the auth subscription's
    // refreshAssetEnrichment is a no-op and doesn't race with our flush.
    useAuthStore.setState({ isAuthenticated: true });

    // Set up queue directly
    useTagStore.setState({
      _lookupQueue: new Set(['EPC001'])
    });

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

describe('TagStore - failed lookups do not self-retry (TRA-1093)', () => {
  beforeEach(() => {
    useTagStore.setState({
      tags: [],
      _lookupQueue: new Set<string>(),
      _isLookupInProgress: false,
      _lookupTimer: null,
    });
    useAuthStore.setState({ isAuthenticated: true });
    vi.clearAllMocks();
  });

  /**
   * The error path re-queues the failed batch, and the `finally` block used to
   * immediately re-flush whenever the queue was non-empty. Together those form a
   * closed loop with no backoff and no cap: ~930 attempts/second for as long as
   * the API stays unhealthy.
   *
   * In the browser that is a self-inflicted request storm during any backend
   * outage. In the unit suite the loop outlives the test file that started it —
   * `singleFork: true` keeps the process alive across all 168 files — and its
   * console/RPC traffic starves later files' event loops. That is what made
   * `App.capability.test.tsx` miss its 1000ms `waitFor` budget intermittently.
   */
  it('stops after one failed attempt instead of re-flushing forever', async () => {
    vi.spyOn(console, 'error').mockImplementation(() => {});
    vi.mocked(ensureOrgContext).mockRejectedValue(new Error('No organization context.'));

    useTagStore.getState()._lookupQueue.add('EPC001');
    await useTagStore.getState()._flushLookupQueue();

    expect(ensureOrgContext).toHaveBeenCalledTimes(1);

    // Give any self-scheduled retry a generous window to fire. Before the fix
    // this window held hundreds of attempts.
    await new Promise((resolve) => setTimeout(resolve, 150));

    expect(ensureOrgContext).toHaveBeenCalledTimes(1);
    // The batch is still queued, so a later scan or login can retry it.
    expect(useTagStore.getState()._lookupQueue.has('EPC001')).toBe(true);
  });

  it('still re-flushes when another caller queues work mid-flight', async () => {
    vi.mocked(ensureOrgContext).mockResolvedValue(42);
    let call = 0;
    vi.mocked(lookupApi.byTags).mockImplementation(async () => {
      // On the first flush only: work arrives while that flush is in progress —
      // the race the immediate re-flush exists for. It must still be picked up.
      if (++call === 1) useTagStore.getState()._lookupQueue.add('EPC002');
      return { data: { data: {} } } as never;
    });

    useTagStore.getState()._lookupQueue.add('EPC001');
    await useTagStore.getState()._flushLookupQueue();

    await new Promise((resolve) => setTimeout(resolve, 50));

    expect(lookupApi.byTags).toHaveBeenCalledTimes(2);
    expect(useTagStore.getState()._lookupQueue.size).toBe(0);
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
describe('TagStore - barcode source (TRA-1031)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
  });

  it('accepts and preserves source barcode on new tags', () => {
    useTagStore.getState().addTag({ epc: 'BC1', count: 1, source: 'barcode' });
    expect(useTagStore.getState().tags[0].source).toBe('barcode');
  });

  it('promoting a reconciliation stub keeps the scanning source', () => {
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'BC2', assetIdentifier: 'A-1', count: 0, found: false },
    ]);
    useTagStore.getState().addTag({ epc: 'BC2', count: 1, source: 'barcode' });
    const t = useTagStore.getState().tags.find(x => x.epc === 'BC2')!;
    expect(t.source).toBe('barcode');
    expect(t.reconciled).toBe(true);
  });
});

describe('TagStore - batched addTags (TRA-1150)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
  });

  // Counts writes that replace the tags array. That identity change is what
  // re-renders every component subscribed to state.tags, so it is the quantity
  // that starved the main thread — not the raw number of set() calls.
  const countTagWrites = (fn: () => void): number => {
    let writes = 0;
    const unsub = useTagStore.subscribe((state, prev) => {
      if (state.tags !== prev.tags) writes++;
    });
    try {
      fn();
    } finally {
      unsub();
    }
    return writes;
  };

  it('counts every read in a batch, including repeats of the same EPC', () => {
    const a = EPC_FORMATS.toFullEPC(PRIMARY_TEST_TAG);
    const b = EPC_FORMATS.toFullEPC(LOCATE_TEST_TAG);

    useTagStore.getState().addTags([
      { epc: a, rssi: -60, count: 1, source: 'rfid' },
      { epc: b, rssi: -55, count: 1, source: 'rfid' },
      { epc: a, rssi: -58, count: 1, source: 'rfid' },
    ]);

    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(2);
    expect(
      tags.reduce((sum, t) => sum + (t.count || 1), 0),
      'three reads must count as three'
    ).toBe(3);

    const tagA = tags.find(t => t.epc === a)!;
    expect(tagA.count).toBe(2);
    expect(tagA.readCount).toBe(2);
  });

  it('merges a batch into tags already in the store', () => {
    const a = EPC_FORMATS.toFullEPC(PRIMARY_TEST_TAG);
    useTagStore.getState().addTags([{ epc: a, rssi: -60, count: 1, source: 'rfid' }]);
    useTagStore.getState().addTags([{ epc: a, rssi: -61, count: 1, source: 'rfid' }]);

    const tags = useTagStore.getState().tags;
    expect(tags).toHaveLength(1);
    expect(tags[0].count).toBe(2);
  });

  it('replaces the tags array exactly once per batch', () => {
    const writes = countTagWrites(() => {
      useTagStore.getState().addTags([
        { epc: '000000000000000000010018', count: 1, source: 'rfid' },
        { epc: '000000000000000000010019', count: 1, source: 'rfid' },
        { epc: '000000000000000000010020', count: 1, source: 'rfid' },
      ]);
    });

    expect(writes, 'a three-tag batch must be one store write, not three').toBe(1);
    expect(useTagStore.getState().tags).toHaveLength(3);
  });

  it('keeps the single write when the batch merges into existing tags', () => {
    useTagStore.getState().addTags([
      { epc: '000000000000000000010018', count: 1, source: 'rfid' },
      { epc: '000000000000000000010019', count: 1, source: 'rfid' },
    ]);

    const writes = countTagWrites(() => {
      useTagStore.getState().addTags([
        { epc: '000000000000000000010018', count: 1, source: 'rfid' },
        { epc: '000000000000000000010019', count: 1, source: 'rfid' },
        { epc: '000000000000000000010018', count: 1, source: 'rfid' },
      ]);
    });

    expect(writes).toBe(1);
    expect(
      useTagStore.getState().tags.reduce((sum, t) => sum + (t.count || 1), 0),
      'two reads then three more reads is five reads'
    ).toBe(5);
  });

  it('addTag still works and is one batch of one', () => {
    const a = EPC_FORMATS.toFullEPC(PRIMARY_TEST_TAG);
    const writes = countTagWrites(() => {
      useTagStore.getState().addTag({ epc: a, rssi: -60, count: 1 });
    });

    expect(writes).toBe(1);
    expect(useTagStore.getState().tags).toHaveLength(1);
    expect(useTagStore.getState().tags[0].count).toBe(1);
  });

  it('ignores an empty batch without touching the store', () => {
    const writes = countTagWrites(() => {
      useTagStore.getState().addTags([]);
    });

    expect(writes).toBe(0);
    expect(useTagStore.getState().tags).toHaveLength(0);
  });

  it('promotes a reconciliation stub matched inside a batch', () => {
    useTagStore.getState().mergeReconciliationTags([
      { epc: 'BATCH1', assetIdentifier: 'A-7', count: 0, found: false },
    ]);

    useTagStore.getState().addTags([{ epc: 'BATCH1', count: 1, source: 'rfid' }]);

    const t = useTagStore.getState().tags.find(x => x.epc === 'BATCH1')!;
    expect(t.reconciled).toBe(true);
    expect(t.source).toBe('rfid');
    expect(t.assetIdentifier).toBe('A-7');
  });
});

describe('TagStore - clearEnrichment (TRA-1191)', () => {
  /**
   * An EPC the reader physically observed is not org-scoped data. What that EPC
   * RESOLVES TO — an asset, a location — is. So a change of org context has to
   * invalidate the resolution while leaving the observation alone.
   *
   * The store was registered in `orgScopedCache`'s ORG_SCOPED_STORES with
   * `clearTags`, which throws the observation away too. Because that runs on
   * LOGIN (authStore's setOrgContext invalidates after setCurrentOrg), logging
   * in destroyed the anonymous scan — and the anonymous-scan-then-log-in-to-
   * enrich flow is exactly what tagStore's own auth subscription exists to
   * serve. The two features cancelled out, and the subscription's lookup ran
   * against an empty tag list.
   *
   * An ORG SWITCH still clears outright, per TRA-318; only auth changes take
   * this softer path. The distinction lives in the registry, not here.
   */
  beforeEach(() => {
    useTagStore.getState().clearTags();
  });

  it('keeps the scan and drops only what the org resolved it to', () => {
    useTagStore.getState().setTags([
      {
        epc: 'AAAA0001',
        count: 3,
        rssi: -55,
        source: 'rfid',
        type: 'asset',
        timestamp: 1_700_000_000,
        firstSeenTime: 1_700_000_000,
        lastSeenTime: 1_700_000_009,
        assetId: 11,
        assetName: 'Pump A',
        assetIdentifier: 'ASSET-11',
      },
      {
        epc: 'AAAA0002',
        count: 1,
        rssi: -70,
        source: 'rfid',
        type: 'location',
        locationId: 22,
        locationName: 'Bay 2',
      },
    ]);

    useTagStore.getState().clearEnrichment();

    const tags = useTagStore.getState().tags;

    // The observation survives, in full.
    expect(tags).toHaveLength(2);
    expect(tags.map(t => t.epc)).toEqual(['AAAA0001', 'AAAA0002']);
    expect(tags[0].count).toBe(3);
    expect(tags[0].rssi).toBe(-55);
    expect(tags[0].firstSeenTime).toBe(1_700_000_000);
    expect(tags[0].lastSeenTime).toBe(1_700_000_009);
    expect(tags[1].source).toBe('rfid');

    // The org-scoped resolution does not.
    for (const t of tags) {
      expect(t.type).toBe('unknown');
      expect(t.assetId).toBeUndefined();
      expect(t.assetName).toBeUndefined();
      expect(t.assetIdentifier).toBeUndefined();
      expect(t.locationId).toBeUndefined();
      expect(t.locationName).toBeUndefined();
    }
  });

  it('leaves the tags re-enrichable, which clearTags cannot', async () => {
    // The point of keeping the scan is that the next lookup has something to
    // resolve. Asserted through the real queue path rather than by inspecting
    // fields, because "re-enrichable" is a behaviour, not a shape.
    useTagStore.getState().setTags([
      { epc: 'AAAA0003', count: 1, source: 'rfid', type: 'asset', assetId: 99 },
    ]);
    useTagStore.getState().clearEnrichment();

    vi.mocked(lookupApi.byTags).mockResolvedValue({
      data: {
        data: {
          AAAA0003: {
            entity_type: 'asset',
            entity_id: 7,
            asset: { id: 7, name: 'Re-resolved', external_key: 'ASSET-7' },
          },
        },
      },
      // eslint-disable-next-line @typescript-eslint/no-explicit-any
    } as any);

    useAuthStore.setState({ isAuthenticated: true });
    await useTagStore.getState().refreshAssetEnrichment();

    const tag = useTagStore.getState().tags[0];
    expect(lookupApi.byTags).toHaveBeenCalledWith({ type: 'rfid', values: ['AAAA0003'] });
    expect(tag.assetId).toBe(7);
    expect(tag.assetName).toBe('Re-resolved');
  });

  it('is a no-op on an empty store rather than throwing', () => {
    expect(() => useTagStore.getState().clearEnrichment()).not.toThrow();
    expect(useTagStore.getState().tags).toEqual([]);
  });
});

describe('memory-bank data survives repeated reads (TRA-1251)', () => {
  beforeEach(() => {
    useTagStore.setState({ tags: [], _lookupQueue: new Set(), _lookupTimer: null });
  });

  const EPC = '000105000F0E01001901007D';

  it('stores tid and userData from a read that carried them', () => {
    useTagStore.getState().addTags([
      { epc: EPC, rssi: -44, count: 1, source: 'rfid', tid: 'E28011606000', userData: 'DEADBEEF' }
    ]);

    const [tag] = useTagStore.getState().tags;
    expect(tag.tid).toBe('E28011606000');
    expect(tag.userData).toBe('DEADBEEF');
  });

  it('does not let a later read without bank data erase an earlier one that had it', () => {
    // This is the point of the whole task. A bank read can succeed on one
    // inventory round and be refused on the next — the tag moves, the round
    // trip gets noisier, the chip declines. addTags merges with
    // { ...existing, ...tag }, so an incoming record whose tid is undefined
    // overwrites a good value with nothing, and nobody ever learns that a
    // successful read was thrown away.
    //
    // ⚠ The keys below are written out as `undefined` DELIBERATELY. Omitting
    // them entirely makes this test pass against the unfixed code, because a
    // spread skips absent keys — and that is not the shape the product
    // produces. tagReadToStoreTags maps `tid: tag.tid`, which always creates
    // the key, undefined value and all. Reproducing the real shape is the only
    // version of this test that means anything.
    useTagStore.getState().addTags([
      { epc: EPC, rssi: -44, count: 1, source: 'rfid', tid: 'E28011606000', userData: 'DEADBEEF' }
    ]);
    useTagStore.getState().addTags([
      { epc: EPC, rssi: -46, count: 1, source: 'rfid', tid: undefined, userData: undefined }
    ]);

    const [tag] = useTagStore.getState().tags;
    expect(tag.tid, 'a refused re-read must not erase a good one').toBe('E28011606000');
    expect(tag.userData).toBe('DEADBEEF');
    expect(tag.count, 'the read itself still counts').toBe(2);
  });

  it('lets a later read fill in bank data the first read lacked', () => {
    useTagStore.getState().addTags([
      { epc: EPC, rssi: -44, count: 1, source: 'rfid' }
    ]);
    useTagStore.getState().addTags([
      { epc: EPC, rssi: -46, count: 1, source: 'rfid', tid: 'E28011606000' }
    ]);

    expect(useTagStore.getState().tags[0].tid).toBe('E28011606000');
  });
});
