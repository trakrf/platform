import '@testing-library/jest-dom';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import ForcedPasswordChangeScreen from '../ForcedPasswordChangeScreen';
import { authApi } from '@/lib/api/auth';
import { useAuthStore } from '@/stores';
import type { UserProfile } from '@/types/org';

vi.mock('@/lib/api/auth', () => ({
  authApi: { changePassword: vi.fn() },
}));

vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

const FLAGGED_PROFILE: UserProfile = {
  id: 7,
  name: 'Dwayne',
  email: 'dwayne@example.com',
  is_superadmin: false,
  must_change_password: true,
  current_org: null,
  orgs: [],
};

const FLAGGED_USER = {
  id: 7,
  email: 'dwayne@example.com',
  name: 'Dwayne',
  created_at: '2026-08-13T00:00:00Z',
  updated_at: '2026-08-13T00:00:00Z',
  must_change_password: true,
};

const fill = (current: string, next: string, confirm: string) => {
  fireEvent.change(screen.getByLabelText(/current password/i), { target: { value: current } });
  fireEvent.change(screen.getByLabelText(/^new password/i), { target: { value: next } });
  fireEvent.change(screen.getByLabelText(/confirm new password/i), {
    target: { value: confirm },
  });
};

const submitButton = () => screen.getByRole('button', { name: /set new password/i });

beforeEach(() => {
  useAuthStore.setState({ profile: FLAGGED_PROFILE, user: FLAGGED_USER });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  useAuthStore.setState({ profile: null, user: null });
});

describe('ForcedPasswordChangeScreen', () => {
  // The whole point is that there is no way past it. A "skip", "later" or
  // "back" control would make the flag advisory, which is the state this
  // ticket exists to leave behind.
  it('offers no way to dismiss it', () => {
    render(<ForcedPasswordChangeScreen />);

    expect(screen.queryByRole('button', { name: /skip|later|cancel|dismiss/i })).toBeNull();
    expect(screen.queryByRole('link')).toBeNull();
  });

  it('disables submit until all three fields are filled', () => {
    render(<ForcedPasswordChangeScreen />);
    expect(submitButton()).toBeDisabled();

    fill('Changeme!', 'chosen-pass1', 'chosen-pass1');
    expect(submitButton()).toBeEnabled();
  });

  it('rejects a short new password without calling the API', async () => {
    render(<ForcedPasswordChangeScreen />);
    fill('Changeme!', 'short', 'short');
    fireEvent.click(submitButton());

    // Matched on the alert, not on the text: the field hint says "at least 8
    // characters" too, and asserting on the bare string would pass without an
    // error ever being raised.
    expect(await screen.findByRole('alert')).toHaveTextContent(/at least 8 characters/i);
    expect(authApi.changePassword).not.toHaveBeenCalled();
  });

  it('rejects a mismatched confirmation without calling the API', async () => {
    render(<ForcedPasswordChangeScreen />);
    fill('Changeme!', 'chosen-pass1', 'chosen-pass2');
    fireEvent.click(submitButton());

    expect(await screen.findByText(/do not match/i)).toBeInTheDocument();
    expect(authApi.changePassword).not.toHaveBeenCalled();
  });

  it('refuses to re-use the bootstrap password', async () => {
    render(<ForcedPasswordChangeScreen />);
    fill('Changeme!', 'Changeme!', 'Changeme!');
    fireEvent.click(submitButton());

    expect(await screen.findByText(/different from your current password/i)).toBeInTheDocument();
    expect(authApi.changePassword).not.toHaveBeenCalled();
  });

  // Clearing both copies of the flag is what lets the app through: the gate
  // reads the server-derived profile when it has one and the persisted login
  // user before that, so leaving either set would keep the user stuck.
  it('clears the flag on both the profile and the user on success', async () => {
    vi.mocked(authApi.changePassword).mockResolvedValue({
      data: { message: 'Password updated successfully' },
    } as Awaited<ReturnType<typeof authApi.changePassword>>);

    render(<ForcedPasswordChangeScreen />);
    fill('Changeme!', 'chosen-pass1', 'chosen-pass1');
    fireEvent.click(submitButton());

    await waitFor(() => {
      expect(authApi.changePassword).toHaveBeenCalledWith('Changeme!', 'chosen-pass1');
    });

    await waitFor(() => {
      expect(useAuthStore.getState().profile?.must_change_password).toBe(false);
      expect(useAuthStore.getState().user?.must_change_password).toBe(false);
    });
  });

  it('surfaces the wrong-current-password detail and stays gated', async () => {
    vi.mocked(authApi.changePassword).mockRejectedValue({
      response: {
        status: 400,
        data: { error: { detail: 'Current password is incorrect' } },
      },
    });

    render(<ForcedPasswordChangeScreen />);
    fill('wrong-one1', 'chosen-pass1', 'chosen-pass1');
    fireEvent.click(submitButton());

    expect(await screen.findByText(/current password is incorrect/i)).toBeInTheDocument();
    expect(useAuthStore.getState().profile?.must_change_password).toBe(true);
  });
});
