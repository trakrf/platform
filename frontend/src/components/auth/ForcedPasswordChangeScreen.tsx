/**
 * ForcedPasswordChangeScreen — the no-dismiss gate for an account still on the
 * password an operator typed for it (TRA-1135).
 *
 * Onsite onboarding is synchronous and every account-creation flow we own is
 * asynchronous, so an operator sets a password in the room. That is a supported
 * flow only if the account cannot stay on it: App renders this instead of the
 * entire layout — sidebar, header and all — whenever the flag is set, and there
 * is deliberately no skip, no back link and no route away from it.
 *
 * It asks for the current password because the endpoint behind it does
 * (TRA-1130 re-proves possession before rotating). The alternative — stashing
 * the plaintext from the login form — is worse than one extra field.
 */

import { useState } from 'react';
import { useAuthStore } from '@/stores';
import { authApi } from '@/lib/api/auth';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import toast from 'react-hot-toast';

export default function ForcedPasswordChangeScreen() {
  const email = useAuthStore((s) => s.profile?.email ?? s.user?.email ?? '');

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [isSubmitting, setIsSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const fieldsFilled =
    currentPassword.length > 0 && newPassword.length > 0 && confirmPassword.length > 0;

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!fieldsFilled || isSubmitting) return;

    if (newPassword.length < 8) {
      setError('New password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setError('New passwords do not match');
      return;
    }
    // The server would accept this — it only checks that the current password
    // verifies — and the flag would clear, leaving the account on exactly the
    // shared password this screen exists to retire.
    if (newPassword === currentPassword) {
      setError('New password must be different from your current password');
      return;
    }

    setError(null);
    setIsSubmitting(true);
    try {
      await authApi.changePassword(currentPassword, newPassword);

      // Clear both copies of the flag. The gate reads the server-derived
      // profile when it has one and the persisted login user before that, so
      // leaving either set keeps the user stuck on this screen.
      useAuthStore.setState((s) => ({
        profile: s.profile ? { ...s.profile, must_change_password: false } : s.profile,
        user: s.user ? { ...s.user, must_change_password: false } : s.user,
      }));

      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      toast.success('Password changed');
    } catch (err: unknown) {
      setError(getApiErrorMessage(err, 'Failed to change password'));
    } finally {
      setIsSubmitting(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <h1 className="text-2xl font-semibold text-white mb-2">Set your password</h1>
        <p className="text-sm text-gray-400 mb-6">
          {email ? `${email} is ` : 'This account is '}
          still using the password it was set up with. Choose your own to continue.
        </p>

        {error && (
          <div
            role="alert"
            className="mb-4 p-3 rounded bg-red-900/40 border border-red-700 text-red-200 text-sm"
          >
            {error}
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          <div>
            <label
              htmlFor="forced-current-password"
              className="block text-sm font-medium text-gray-300 mb-2"
            >
              Current password
            </label>
            <input
              id="forced-current-password"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(e) => setCurrentPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <div>
            <label
              htmlFor="forced-new-password"
              className="block text-sm font-medium text-gray-300 mb-2"
            >
              New password
            </label>
            <input
              id="forced-new-password"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(e) => setNewPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <p className="mt-2 text-xs text-gray-400">At least 8 characters.</p>
          </div>

          <div>
            <label
              htmlFor="forced-confirm-password"
              className="block text-sm font-medium text-gray-300 mb-2"
            >
              Confirm new password
            </label>
            <input
              id="forced-confirm-password"
              type="password"
              autoComplete="new-password"
              value={confirmPassword}
              onChange={(e) => setConfirmPassword(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <button
            type="submit"
            disabled={!fieldsFilled || isSubmitting}
            data-testid="forced-password-submit"
            className="w-full px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {isSubmitting ? 'Saving...' : 'Set new password'}
          </button>
        </form>
      </div>
    </div>
  );
}
