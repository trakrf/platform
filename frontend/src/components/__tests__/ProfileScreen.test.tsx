import '@testing-library/jest-dom';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import ProfileScreen from '../ProfileScreen';
import { usersApi } from '@/lib/api/users';
import { useAuthStore } from '@/stores';
import type { UserProfile } from '@/types/org';

vi.mock('@/lib/api/users', () => ({
  usersApi: { updateProfile: vi.fn() },
}));

vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const PROFILE: UserProfile = {
  id: 7,
  name: 'Tim (Demo)',
  email: 'tim@example.com',
  is_superadmin: false,
  current_org: null,
  orgs: [],
};

const USER = {
  id: 7,
  email: 'tim@example.com',
  name: 'Tim (Demo)',
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const okResponse = (name: string, email: string) =>
  ({ data: { data: { ...PROFILE, name, email } } }) as Awaited<
    ReturnType<typeof usersApi.updateProfile>
  >;

beforeEach(() => {
  useAuthStore.setState({ profile: PROFILE, user: USER });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  useAuthStore.setState({ profile: null, user: null });
  window.location.hash = '';
});

describe('ProfileScreen', () => {
  it('seeds the form from the current profile', () => {
    render(<ProfileScreen />);
    expect(screen.getByLabelText(/display name/i)).toHaveValue('Tim (Demo)');
    expect(screen.getByLabelText(/email/i)).toHaveValue('tim@example.com');
  });

  it('disables save until something changes', () => {
    render(<ProfileScreen />);
    expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();

    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: 'Tim Operator' },
    });
    expect(screen.getByRole('button', { name: /save/i })).toBeEnabled();
  });

  it('sends only the changed fields', async () => {
    vi.mocked(usersApi.updateProfile).mockResolvedValue(
      okResponse('Tim Operator', 'tim@example.com')
    );
    render(<ProfileScreen />);

    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: 'Tim Operator' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(usersApi.updateProfile).toHaveBeenCalledWith({ name: 'Tim Operator' });
    });
  });

  it('updates the auth store so the header reflects the new name', async () => {
    vi.mocked(usersApi.updateProfile).mockResolvedValue(
      okResponse('Tim Operator', 'tim@example.com')
    );
    render(<ProfileScreen />);

    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: 'Tim Operator' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    await waitFor(() => {
      expect(useAuthStore.getState().profile?.name).toBe('Tim Operator');
      // Header/OrgSwitcher render from `user`, not `profile` — if this one is
      // left behind, the rename looks like it did not take.
      expect(useAuthStore.getState().user?.name).toBe('Tim Operator');
    });
  });

  it('surfaces a duplicate-email conflict from the server', async () => {
    vi.mocked(usersApi.updateProfile).mockRejectedValue({
      response: { status: 409, data: { error: { detail: 'Email already exists' } } },
    });
    render(<ProfileScreen />);

    fireEvent.change(screen.getByLabelText(/email/i), {
      target: { value: 'taken@example.com' },
    });
    fireEvent.click(screen.getByRole('button', { name: /save/i }));

    expect(await screen.findByText(/email already exists/i)).toBeInTheDocument();
  });

  it('rejects a blank display name without calling the API', () => {
    render(<ProfileScreen />);
    fireEvent.change(screen.getByLabelText(/display name/i), {
      target: { value: '   ' },
    });
    expect(screen.getByRole('button', { name: /save/i })).toBeDisabled();
    expect(usersApi.updateProfile).not.toHaveBeenCalled();
  });
});
