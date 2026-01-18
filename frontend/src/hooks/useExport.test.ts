/**
 * Tests for useExport hook
 */

import { renderHook, act } from '@testing-library/react';
import { useExport } from './useExport';

describe('useExport', () => {
  it('initializes with default format csv', () => {
    const { result } = renderHook(() => useExport());
    expect(result.current.selectedFormat).toBe('csv');
    expect(result.current.isModalOpen).toBe(false);
  });

  it('initializes with custom default format', () => {
    const { result } = renderHook(() => useExport('pdf'));
    expect(result.current.selectedFormat).toBe('pdf');
    expect(result.current.isModalOpen).toBe(false);
  });

  it('opens modal with selected format', () => {
    const { result } = renderHook(() => useExport());

    act(() => {
      result.current.openExport('pdf');
    });

    expect(result.current.isModalOpen).toBe(true);
    expect(result.current.selectedFormat).toBe('pdf');
  });

  it('changes format when opening with different format', () => {
    const { result } = renderHook(() => useExport('csv'));

    act(() => {
      result.current.openExport('xlsx');
    });

    expect(result.current.selectedFormat).toBe('xlsx');
  });

  it('closes modal', () => {
    const { result } = renderHook(() => useExport());

    act(() => {
      result.current.openExport('xlsx');
    });
    expect(result.current.isModalOpen).toBe(true);

    act(() => {
      result.current.closeExport();
    });
    expect(result.current.isModalOpen).toBe(false);
  });

  it('preserves format after closing modal', () => {
    const { result } = renderHook(() => useExport());

    act(() => {
      result.current.openExport('pdf');
    });
    act(() => {
      result.current.closeExport();
    });

    // Format should be preserved even after closing
    expect(result.current.selectedFormat).toBe('pdf');
  });
});
