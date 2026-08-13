/**
 * ProfileScreen - the signed-in user's own account details (TRA-958).
 *
 * Every member reaches this from the account menu; it is deliberately not
 * admin-gated, because the thing being edited is the user, not the org.
 */

import { useState, useEffect } from 'react';
import { ArrowLeft } from 'lucide-react';
import { useAuthStore } from '@/stores';
import { usersApi } from '@/lib/api/users';
import { authApi } from '@/lib/api/auth';
import { getApiErrorMessage } from '@/lib/api/errorMessage';
import toast from 'react-hot-toast';

export default function ProfileScreen() {
  const profile = useAuthStore((s) => s.profile);
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [originalName, setOriginalName] = useState('');
  const [originalEmail, setOriginalEmail] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const [currentPassword, setCurrentPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [confirmPassword, setConfirmPassword] = useState('');
  const [isChangingPassword, setIsChangingPassword] = useState(false);
  const [passwordError, setPasswordError] = useState<string | null>(null);

  useEffect(() => {
    if (profile) {
      setName(profile.name ?? '');
      setEmail(profile.email ?? '');
      setOriginalName(profile.name ?? '');
      setOriginalEmail(profile.email ?? '');
    }
  }, [profile]);

  const nameChanged = name.trim() !== originalName;
  const emailChanged = email.trim() !== originalEmail;
  const hasChanges = nameChanged || emailChanged;
  const isValid = name.trim().length >= 1 && email.trim().length >= 3;

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!hasChanges || !isValid || isSaving) return;

    setError(null);
    setIsSaving(true);

    // Send only what moved. The endpoint is a partial update, and an unchanged
    // email round-tripped back is a needless uniqueness check.
    const body: { name?: string; email?: string } = {};
    if (nameChanged) body.name = name.trim();
    if (emailChanged) body.email = email.trim();

    try {
      const res = await usersApi.updateProfile(body);
      const updated = res.data.data;

      // The header and account menu render from authStore.user, the org
      // picker from profile — both have to move or the rename looks like it
      // did not take.
      useAuthStore.setState((s) => ({
        profile: updated,
        user: s.user ? { ...s.user, name: updated.name, email: updated.email } : s.user,
      }));

      setName(updated.name ?? '');
      setEmail(updated.email ?? '');
      setOriginalName(updated.name ?? '');
      setOriginalEmail(updated.email ?? '');
      toast.success('Profile updated');
    } catch (err: unknown) {
      setError(getApiErrorMessage(err, 'Failed to update profile'));
    } finally {
      setIsSaving(false);
    }
  };

  const passwordFieldsFilled =
    currentPassword.length > 0 && newPassword.length > 0 && confirmPassword.length > 0;

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!passwordFieldsFilled || isChangingPassword) return;

    if (newPassword.length < 8) {
      setPasswordError('New password must be at least 8 characters');
      return;
    }
    if (newPassword !== confirmPassword) {
      setPasswordError('New passwords do not match');
      return;
    }

    setPasswordError(null);
    setIsChangingPassword(true);
    try {
      await authApi.changePassword(currentPassword, newPassword);
      setCurrentPassword('');
      setNewPassword('');
      setConfirmPassword('');
      toast.success('Password changed');
    } catch (err: unknown) {
      setPasswordError(getApiErrorMessage(err, 'Failed to change password'));
    } finally {
      setIsChangingPassword(false);
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <div className="flex items-center gap-3 mb-6">
          <a
            href="#scan"
            className="text-gray-400 hover:text-white transition-colors"
            aria-label="Back"
          >
            <ArrowLeft className="w-5 h-5" />
          </a>
          <h1 className="text-2xl font-semibold text-white">Profile</h1>
        </div>

        {error && (
          <div className="mb-4 p-3 rounded bg-red-900/40 border border-red-700 text-red-200 text-sm">
            {error}
          </div>
        )}

        <form onSubmit={handleSave} className="space-y-6">
          <div>
            <label htmlFor="profile-name" className="block text-sm font-medium text-gray-300 mb-2">
              Display name
            </label>
            <input
              id="profile-name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              maxLength={255}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
          </div>

          <div>
            <label htmlFor="profile-email" className="block text-sm font-medium text-gray-300 mb-2">
              Email
            </label>
            <input
              id="profile-email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
            />
            <p className="mt-2 text-xs text-gray-400">
              This is the address you sign in with. Changing it changes your login.
            </p>
          </div>

          <button
            type="submit"
            disabled={!hasChanges || !isValid || isSaving}
            className="w-full px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
          >
            {isSaving ? 'Saving...' : 'Save'}
          </button>
        </form>

        <div className="mt-8 pt-6 border-t border-gray-700">
          <h2 className="text-lg font-semibold text-white mb-4">Change password</h2>

          {passwordError && (
            <div className="mb-4 p-3 rounded bg-red-900/40 border border-red-700 text-red-200 text-sm">
              {passwordError}
            </div>
          )}

          <form onSubmit={handleChangePassword} className="space-y-4">
            <div>
              <label
                htmlFor="current-password"
                className="block text-sm font-medium text-gray-300 mb-2"
              >
                Current password
              </label>
              <input
                id="current-password"
                type="password"
                autoComplete="current-password"
                value={currentPassword}
                onChange={(e) => setCurrentPassword(e.target.value)}
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>

            <div>
              <label htmlFor="new-password" className="block text-sm font-medium text-gray-300 mb-2">
                New password
              </label>
              <input
                id="new-password"
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
                htmlFor="confirm-new-password"
                className="block text-sm font-medium text-gray-300 mb-2"
              >
                Confirm new password
              </label>
              <input
                id="confirm-new-password"
                type="password"
                autoComplete="new-password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                className="w-full px-3 py-2 bg-gray-700 border border-gray-600 rounded text-white focus:outline-none focus:ring-2 focus:ring-blue-500"
              />
            </div>

            <button
              type="submit"
              disabled={!passwordFieldsFilled || isChangingPassword}
              className="w-full px-4 py-2 bg-blue-600 text-white rounded hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition-colors"
            >
              {isChangingPassword ? 'Changing...' : 'Change password'}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
