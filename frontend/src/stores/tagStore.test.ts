import { describe, it, expect, beforeEach, vi } from 'vitest';
import { useTagStore } from './tagStore';
import { useAuthStore } from './authStore';
import { lookupApi } from '@/lib/api/lookup';
import { LOCATE_TEST_TAG, PRIMARY_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';

// Mock the lookup API
vi.mock('@/lib/api/lookup');

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