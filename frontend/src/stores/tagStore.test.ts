import { describe, it, expect, beforeEach } from 'vitest';
import { useTagStore } from './tagStore';
import { LOCATE_TEST_TAG, PRIMARY_TEST_TAG, EPC_FORMATS } from '@test-utils/constants';

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