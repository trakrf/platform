/**
 * AcceptInviteScreen - Handle invitation acceptance via token URL
 * States:
 * 1. No token: Show error "Invalid invitation link"
 * 2. Not logged in: Show invitation details with Login/Signup buttons
 * 3. Logged in + accepting: Loading state
 * 4. Success: Redirect to home with toast
 * 5. Invalid/expired: Error message
 * 6. Already member: "You're already a member"
 */

import { useState } from 'react';
import { Mail, LogIn, UserPlus } from 'lucide-react';
import { useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import toast from 'react-hot-toast';

interface AcceptInviteScreenProps {
  token: string | null;
}

export default function AcceptInviteScreen({ token }: AcceptInviteScreenProps) {
  const { isAuthenticated } = useAuthStore();
  const [isAccepting, setIsAccepting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [acceptedOrg, setAcceptedOrg] = useState<{ name: string; role: string } | null>(null);

  // No token - invalid link
  if (!token) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-red-900/20 flex items-center justify-center">
            <Mail className="w-6 h-6 text-red-400" />
          </div>
          <h1 className="text-2xl font-semibold text-white mb-4">
            Invalid Invitation Link
          </h1>
          <p className="text-gray-400 mb-6">
            This invitation link is invalid or incomplete. Please check the link and try again.
          </p>
          <a
            href="#home"
            className="block w-full text-center bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors"
          >
            Go Home
          </a>
        </div>
      </div>
    );
  }

  // Success state - show accepted org
  if (acceptedOrg) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-green-900/20 flex items-center justify-center">
            <Mail className="w-6 h-6 text-green-400" />
          </div>
          <h1 className="text-2xl font-semibold text-white mb-4">
            Welcome to {acceptedOrg.name}!
          </h1>
          <p className="text-gray-400 mb-6">
            You&apos;ve joined as <span className="text-white capitalize">{acceptedOrg.role}</span>.
          </p>
          <a
            href="#home"
            className="block w-full text-center bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors"
          >
            Go to Dashboard
          </a>
        </div>
      </div>
    );
  }

  // Handle accept invitation
  const handleAccept = async () => {
    if (!token || isAccepting) return;

    setIsAccepting(true);
    setError(null);

    try {
      const response = await orgsApi.acceptInvitation(token);
      const { org_name, role } = response.data;

      // Refresh profile to get updated org list
      await useAuthStore.getState().fetchProfile();

      toast.success(`Joined ${org_name} successfully!`);
      setAcceptedOrg({ name: org_name, role });
    } catch (err: unknown) {
      const errorMessage = extractErrorMessage(err);
      setError(errorMessage);
    } finally {
      setIsAccepting(false);
    }
  };

  // Handle decline - just go home
  const handleDecline = () => {
    window.location.hash = '#home';
  };

  // Not logged in - show login/signup options
  if (!isAuthenticated) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-blue-900/20 flex items-center justify-center">
            <Mail className="w-6 h-6 text-blue-400" />
          </div>
          <h1 className="text-2xl font-semibold text-white mb-4">
            You&apos;ve Been Invited!
          </h1>
          <p className="text-gray-400 mb-6">
            Sign in or create an account to accept this organization invitation.
          </p>

          <div className="space-y-3">
            <a
              href={`#login?returnTo=accept-invite&token=${encodeURIComponent(token)}`}
              className="flex items-center justify-center gap-2 w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors"
            >
              <LogIn className="w-4 h-4" />
              Sign In
            </a>
            <a
              href={`#signup?returnTo=accept-invite&token=${encodeURIComponent(token)}`}
              className="flex items-center justify-center gap-2 w-full bg-gray-700 hover:bg-gray-600 text-white py-2 px-4 rounded-lg font-medium transition-colors"
            >
              <UserPlus className="w-4 h-4" />
              Create Account
            </a>
          </div>

          <p className="text-gray-500 text-sm mt-6">
            After signing in, you&apos;ll be able to accept or decline the invitation.
          </p>
        </div>
      </div>
    );
  }

  // Logged in - show accept/decline options
  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
        <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-blue-900/20 flex items-center justify-center">
          <Mail className="w-6 h-6 text-blue-400" />
        </div>
        <h1 className="text-2xl font-semibold text-white mb-4">
          Organization Invitation
        </h1>
        <p className="text-gray-400 mb-6">
          You&apos;ve been invited to join an organization. Click Accept to join.
        </p>

        {/* Error Display */}
        {error && (
          <div className="bg-red-900/20 border border-red-800 rounded-lg p-3 mb-4">
            <p className="text-red-400 text-sm">{error}</p>
          </div>
        )}

        <div className="space-y-3">
          <button
            onClick={handleAccept}
            disabled={isAccepting}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
            data-testid="accept-invite-button"
          >
            {isAccepting ? 'Accepting...' : 'Accept Invitation'}
          </button>
          <button
            onClick={handleDecline}
            disabled={isAccepting}
            className="w-full bg-gray-700 hover:bg-gray-600 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50"
            data-testid="decline-invite-button"
          >
            Decline
          </button>
        </div>
      </div>
    </div>
  );
}

// Helper to extract error message from API responses
function extractErrorMessage(err: unknown): string {
  const data = (err as { response?: { data?: Record<string, unknown> } })
    .response?.data;
  const errorObj = (data?.error as Record<string, unknown>) || data;

  // Check for specific error messages
  const detail = typeof errorObj?.detail === 'string' ? errorObj.detail : '';
  const title = typeof errorObj?.title === 'string' ? errorObj.title : '';

  // Handle common error cases
  if (detail.toLowerCase().includes('already a member') || title.toLowerCase().includes('already a member')) {
    return 'You are already a member of this organization.';
  }
  if (detail.toLowerCase().includes('expired') || title.toLowerCase().includes('expired')) {
    return 'This invitation has expired. Please request a new invitation.';
  }
  if (detail.toLowerCase().includes('invalid') || title.toLowerCase().includes('invalid')) {
    return 'This invitation link is invalid.';
  }

  // Default error
  return detail || title ||
    (typeof data?.error === 'string' ? data.error : '') ||
    (typeof (err as Error).message === 'string' ? (err as Error).message : '') ||
    'Failed to accept invitation. Please try again.';
}
