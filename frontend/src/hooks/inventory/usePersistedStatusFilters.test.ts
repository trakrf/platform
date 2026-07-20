import { describe, it, expect, beforeEach } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import { usePersistedStatusFilters, STATUS_FILTERS_STORAGE_KEY } from './usePersistedStatusFilters';

describe('usePersistedStatusFilters', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  it('starts empty when nothing is persisted', () => {
    const { result } = renderHook(() => usePersistedStatusFilters(true));
    expect(result.current[0].size).toBe(0);
  });

  it('hydrates from localStorage', () => {
    localStorage.setItem(STATUS_FILTERS_STORAGE_KEY, JSON.stringify(['Assets', 'Found']));
    const { result } = renderHook(() => usePersistedStatusFilters(true));
    expect(result.current[0]).toEqual(new Set(['Assets', 'Found']));
  });

  it('persists changes to localStorage', () => {
    const { result } = renderHook(() => usePersistedStatusFilters(true));
    act(() => {
      result.current[1](new Set(['Assets']));
    });
    expect(JSON.parse(localStorage.getItem(STATUS_FILTERS_STORAGE_KEY)!)).toEqual(['Assets']);
  });

  it('survives a remount (reload simulation)', () => {
    const first = renderHook(() => usePersistedStatusFilters(true));
    act(() => {
      first.result.current[1](new Set(['Assets']));
    });
    first.unmount();
    const second = renderHook(() => usePersistedStatusFilters(true));
    expect(second.result.current[0]).toEqual(new Set(['Assets']));
  });

  it('drops a persisted Assets filter when not authenticated', () => {
    localStorage.setItem(STATUS_FILTERS_STORAGE_KEY, JSON.stringify(['Assets']));
    const { result } = renderHook(() => usePersistedStatusFilters(false));
    expect(result.current[0].size).toBe(0);
  });

  it('keeps non-Assets filters when not authenticated', () => {
    localStorage.setItem(STATUS_FILTERS_STORAGE_KEY, JSON.stringify(['Assets', 'Found']));
    const { result } = renderHook(() => usePersistedStatusFilters(false));
    expect(result.current[0]).toEqual(new Set(['Found']));
  });

  it('drops Assets on auth change to logged out', () => {
    const { result, rerender } = renderHook(
      ({ auth }: { auth: boolean }) => usePersistedStatusFilters(auth),
      { initialProps: { auth: true } },
    );
    act(() => {
      result.current[1](new Set(['Assets']));
    });
    rerender({ auth: false });
    expect(result.current[0].size).toBe(0);
  });

  it('ignores malformed persisted JSON', () => {
    localStorage.setItem(STATUS_FILTERS_STORAGE_KEY, 'not json{');
    const { result } = renderHook(() => usePersistedStatusFilters(true));
    expect(result.current[0].size).toBe(0);
  });
});
