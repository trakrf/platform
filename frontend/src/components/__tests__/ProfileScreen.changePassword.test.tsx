import '@testing-library/jest-dom';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import toast from 'react-hot-toast';
import ProfileScreen from '../ProfileScreen';
import { authApi } from '@/lib/api/auth';
import { useAuthStore } from '@/stores';
import type { UserProfile } from '@/types/org';

vi.mock('@/lib/api/users', () => ({
  usersApi: { updateProfile: vi.fn() },
}));

vi.mock('@/lib/api/auth', () => ({
  authApi: { changePassword: vi.fn() },
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

const fillPasswords = (current: string, next: string, confirm: string) => {
  fireEvent.change(screen.getByLabelText(/current password/i), {
    target: { value: current },
  });
  fireEvent.change(screen.getByLabelText(/^new password/i), {
    target: { value: next },
  });
  fireEvent.change(screen.getByLabelText(/confirm new password/i), {
    target: { value: confirm },
  });
};

const changeButton = () => screen.getByRole('button', { name: /change password/i });

beforeEach(() => {
  useAuthStore.setState({ profile: PROFILE, user: USER });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  useAuthStore.setState({ profile: null, user: null });
  window.location.hash = '';
});

describe('ProfileScreen change password', () => {
  it('disables the change button until all three fields are filled', () => {
    render(<ProfileScreen />);
    expect(changeButton()).toBeDisabled();

    fillPasswords('oldpass123', 'newpass123', 'newpass123');
    expect(changeButton()).toBeEnabled();
  });

  it('rejects a short new password without calling the API', async () => {
    render(<ProfileScreen />);
    fillPasswords('oldpass123', 'short', 'short');
    fireEvent.click(changeButton());

    expect(await screen.findByText(/must be at least 8 characters/i)).toBeInTheDocument();
    expect(authApi.changePassword).not.toHaveBeenCalled();
  });

  it('rejects a mismatched confirmation without calling the API', async () => {
    render(<ProfileScreen />);
    fillPasswords('oldpass123', 'newpass123', 'different123');
    fireEvent.click(changeButton());

    expect(await screen.findByText(/do not match/i)).toBeInTheDocument();
    expect(authApi.changePassword).not.toHaveBeenCalled();
  });

  it('submits current and new password and clears the form on success', async () => {
    vi.mocked(authApi.changePassword).mockResolvedValue({
      data: { message: 'Password updated successfully' },
    } as Awaited<ReturnType<typeof authApi.changePassword>>);
    render(<ProfileScreen />);

    fillPasswords('oldpass123', 'newpass123', 'newpass123');
    fireEvent.click(changeButton());

    await waitFor(() => {
      expect(authApi.changePassword).toHaveBeenCalledWith('oldpass123', 'newpass123');
      expect(toast.success).toHaveBeenCalled();
    });
    expect(screen.getByLabelText(/current password/i)).toHaveValue('');
    expect(screen.getByLabelText(/^new password/i)).toHaveValue('');
    expect(screen.getByLabelText(/confirm new password/i)).toHaveValue('');
  });

  it('surfaces the wrong-current-password detail from the server', async () => {
    vi.mocked(authApi.changePassword).mockRejectedValue({
      response: {
        status: 400,
        data: { error: { detail: 'Current password is incorrect' } },
      },
    });
    render(<ProfileScreen />);

    fillPasswords('wrongpass1', 'newpass123', 'newpass123');
    fireEvent.click(changeButton());

    expect(await screen.findByText(/current password is incorrect/i)).toBeInTheDocument();
  });
});
