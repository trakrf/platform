/**
 * AcceptInviteScreen - Handle invitation acceptance via token URL
 * States:
 * 1. No token: Show error "Invalid invitation link"
 * 2. Not logged in: Auto-redirect to login (user exists) or signup (new user)
 * 3. Logged in + email mismatch: Show account mismatch with logout option
 * 4. Logged in + email match: Show accept/decline options
 * 5. Accepting: Loading state
 * 6. Success: Show welcome message
 * 7. Invalid/expired: Error message
 * 8. Already member: "You're already a member"
 */

import { useState, useEffect } from 'react';
import { Mail, Loader2, AlertTriangle } from 'lucide-react';
import { useAuthStore } from '@/stores';
import { orgsApi } from '@/lib/api/orgs';
import { authApi, type InvitationInfo } from '@/lib/api/auth';
import toast from 'react-hot-toast';

interface AcceptInviteScreenProps {
  token: string | null;
}

export default function AcceptInviteScreen({ token }: AcceptInviteScreenProps) {
  const { isAuthenticated, user, logout } = useAuthStore();
  const [isAccepting, setIsAccepting] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [acceptedOrg, setAcceptedOrg] = useState<{ name: string; role: string } | null>(null);

  // State for invitation info (used for both auth and unauth flows)
  const [inviteInfo, setInviteInfo] = useState<InvitationInfo | null>(null);
  const [inviteLoading, setInviteLoading] = useState(true);
  const [inviteFetchError, setInviteFetchError] = useState<string | null>(null);

  // Fetch invitation info for all users with a token
  useEffect(() => {
    if (!token) {
      setInviteLoading(false);
      return;
    }

    setInviteLoading(true);
    authApi
      .getInvitationInfo(token)
      .then((res) => {
        const info = res.data.data;
        setInviteInfo(info);
        setInviteLoading(false);

        // If not authenticated, auto-redirect to login or signup
        if (!isAuthenticated) {
          if (info.user_exists) {
            // User exists - send to login with email pre-filled
            window.location.hash = `#login?returnTo=accept-invite&token=${encodeURIComponent(token)}&email=${encodeURIComponent(info.email)}`;
          } else {
            // New user - send to signup
            window.location.hash = `#signup?returnTo=accept-invite&token=${encodeURIComponent(token)}`;
          }
        }
      })
      .catch(() => {
        setInviteFetchError('This invitation is invalid or has expired.');
        setInviteLoading(false);
      });
  }, [isAuthenticated, token]);

  // Check for email mismatch when authenticated
  const emailMismatch = isAuthenticated && inviteInfo && user?.email?.toLowerCase() !== inviteInfo.email.toLowerCase();

  // Handle logout and redirect back to accept the invite
  const handleLogoutAndContinue = async () => {
    await logout();
    // Redirect to login with the invited email pre-filled
    if (inviteInfo && token) {
      window.location.hash = `#login?returnTo=accept-invite&token=${encodeURIComponent(token)}&email=${encodeURIComponent(inviteInfo.email)}`;
    }
  };

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
      const { org_name, role } = response.data.data;

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

  // Not logged in - show loading or error (success auto-redirects to login/signup)
  if (!isAuthenticated) {
    // Error fetching invite info
    if (inviteFetchError) {
      return (
        <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
          <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
            <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-red-900/20 flex items-center justify-center">
              <Mail className="w-6 h-6 text-red-400" />
            </div>
            <h1 className="text-2xl font-semibold text-white mb-4">
              Invalid Invitation
            </h1>
            <p className="text-gray-400 mb-6">{inviteFetchError}</p>
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

    // Loading state (will auto-redirect on success)
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <Loader2 className="w-8 h-8 mx-auto mb-4 text-blue-400 animate-spin" />
          <p className="text-gray-400">Loading invitation...</p>
        </div>
      </div>
    );
  }

  // Logged in but still loading invite info
  if (inviteLoading) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <Loader2 className="w-8 h-8 mx-auto mb-4 text-blue-400 animate-spin" />
          <p className="text-gray-400">Loading invitation...</p>
        </div>
      </div>
    );
  }

  // Logged in but invite fetch failed
  if (inviteFetchError) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-red-900/20 flex items-center justify-center">
            <Mail className="w-6 h-6 text-red-400" />
          </div>
          <h1 className="text-2xl font-semibold text-white mb-4">
            Invalid Invitation
          </h1>
          <p className="text-gray-400 mb-6">{inviteFetchError}</p>
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

  // Logged in with email mismatch - show account mismatch screen
  if (emailMismatch && inviteInfo) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-yellow-900/20 flex items-center justify-center">
            <AlertTriangle className="w-6 h-6 text-yellow-400" />
          </div>
          <h1 className="text-2xl font-semibold text-white mb-4">
            Account Mismatch
          </h1>
          <p className="text-gray-400 mb-4">
            This invitation was sent to:
          </p>
          <p className="text-white font-medium mb-4">
            {inviteInfo.email}
          </p>
          <p className="text-gray-400 mb-6">
            You&apos;re currently logged in as:
            <br />
            <span className="text-white">{user?.email}</span>
          </p>

          <div className="space-y-3">
            <button
              onClick={handleLogoutAndContinue}
              className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors"
              data-testid="logout-continue-button"
            >
              Log out and continue
            </button>
            <a
              href="#home"
              className="block w-full text-center bg-gray-700 hover:bg-gray-600 text-white py-2 px-4 rounded-lg font-medium transition-colors"
            >
              Go to Dashboard
            </a>
          </div>
        </div>
      </div>
    );
  }

  // Logged in with matching email - show accept/decline options
  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
        <div className="w-12 h-12 mx-auto mb-4 rounded-full bg-blue-900/20 flex items-center justify-center">
          <Mail className="w-6 h-6 text-blue-400" />
        </div>
        <h1 className="text-2xl font-semibold text-white mb-4">
          {inviteInfo ? `Join ${inviteInfo.org_name}` : 'Organization Invitation'}
        </h1>
        <p className="text-gray-400 mb-6">
          {inviteInfo ? (
            <>You&apos;ve been invited to join <span className="text-white font-medium">{inviteInfo.org_name}</span> as <span className="text-white capitalize">{inviteInfo.role}</span>.</>
          ) : (
            <>You&apos;ve been invited to join an organization. Click Accept to join.</>
          )}
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
  if (detail.toLowerCase().includes('was sent to') || title.toLowerCase().includes('was sent to')) {
    // Extract email from message like "This invitation was sent to bob@example.com"
    const emailMatch = (detail || title).match(/sent to\s+(\S+@\S+)/i);
    const invitedEmail = emailMatch ? emailMatch[1] : 'another email address';
    return `This invitation was sent to ${invitedEmail}. Please log in with that account to accept.`;
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
