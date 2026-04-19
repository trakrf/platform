import React, { type ReactNode } from 'react';
import '@testing-library/jest-dom';
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import toast from 'react-hot-toast';
import InventoryScreen from '@/components/InventoryScreen';
import { useTagStore, useAuthStore } from '@/stores';

vi.mock('react-hot-toast', () => ({
  default: vi.fn(),
}));

const renderWithQueryClient = () => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  const Wrapper = ({ children }: { children: ReactNode }) =>
    React.createElement(QueryClientProvider, { client: queryClient }, children);
  return render(<InventoryScreen />, { wrapper: Wrapper });
};

describe('InventoryScreen Reconcile auth gate', () => {
  afterEach(() => {
    cleanup();
    vi.mocked(toast).mockClear();
    sessionStorage.clear();
    window.location.hash = '';
  });

  beforeEach(() => {
    useTagStore.getState().clearTags();
    useAuthStore.setState({ isAuthenticated: false, token: null, user: null });
    sessionStorage.clear();
    window.location.hash = '';
  });

  it('shows upsell toast and stays on inventory when an unauthenticated user clicks Reconcile', async () => {
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, 'click');

    renderWithQueryClient();

    const reconcileButton = screen.getAllByRole('button', { name: /reconcile/i })[0];
    fireEvent.click(reconcileButton);

    await waitFor(() => {
      expect(toast).toHaveBeenCalledWith(
        'Reconciliation is a paid feature. Log in to start your free trial.'
      );
    });
    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
    expect(window.location.hash).toBe('');
    expect(clickSpy).not.toHaveBeenCalled();

    clickSpy.mockRestore();
  });

  it('opens file picker for authenticated users without toasting', async () => {
    useAuthStore.setState({
      isAuthenticated: true,
      token: 'test-token',
      user: { id: 1, email: 't@e.st' } as never,
    });
    const clickSpy = vi.spyOn(HTMLInputElement.prototype, 'click');

    renderWithQueryClient();

    const reconcileButton = screen.getAllByRole('button', { name: /reconcile/i })[0];
    fireEvent.click(reconcileButton);

    expect(clickSpy).toHaveBeenCalledTimes(1);
    expect(toast).not.toHaveBeenCalled();
    expect(window.location.hash).toBe('');
    expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();

    clickSpy.mockRestore();
  });

  it('does not render a Download Sample menu item', () => {
    renderWithQueryClient();

    expect(screen.queryByRole('button', { name: /sample/i })).not.toBeInTheDocument();
    expect(screen.queryByTitle(/download sample/i)).not.toBeInTheDocument();
  });
});
