import { useState, useEffect, useRef } from 'react';
import { useAuthStore } from '@/stores';
import { Eye, EyeOff, Building2, Loader2 } from 'lucide-react';
import { handleAuthRedirect } from '@/utils/authRedirect';
import { authApi } from '@/lib/api/auth';
import type { InvitationInfo } from '@/lib/api/auth';
import toast from 'react-hot-toast';

// Extract invite context from URL
function getInviteContext(): { isInviteFlow: boolean; token: string | null } {
  const hash = window.location.hash;
  const queryIndex = hash.indexOf('?');
  if (queryIndex === -1) {
    return { isInviteFlow: false, token: null };
  }
  const searchParams = new URLSearchParams(hash.substring(queryIndex + 1));
  const returnTo = searchParams.get('returnTo');
  const token = searchParams.get('token');
  const isInviteFlow = returnTo === 'accept-invite' && !!token;
  return { isInviteFlow, token };
}

export default function SignupScreen() {
  const { isInviteFlow, token: inviteToken } = getInviteContext();

  const [email, setEmail] = useState('');
  const [orgName, setOrgName] = useState('');
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{
    email?: string;
    orgName?: string;
    password?: string;
    general?: string;
  }>({});
  const emailInputRef = useRef<HTMLInputElement>(null);
  const passwordInputRef = useRef<HTMLInputElement>(null);

  // Invite flow state
  const [inviteInfo, setInviteInfo] = useState<InvitationInfo | null>(null);
  const [inviteLoading, setInviteLoading] = useState(isInviteFlow);
  const [inviteFetchError, setInviteFetchError] = useState<string | null>(null);

  const { signup, isLoading } = useAuthStore();

  // Fetch invite info on mount if in invite flow
  useEffect(() => {
    if (isInviteFlow && inviteToken) {
      setInviteLoading(true);
      authApi
        .getInvitationInfo(inviteToken)
        .then((res) => {
          const info = res.data.data;
          setInviteInfo(info);
          setEmail(info.email);
          setInviteFetchError(null);
        })
        .catch(() => {
          setInviteFetchError('This invitation is invalid or has expired.');
        })
        .finally(() => setInviteLoading(false));
    }
  }, [isInviteFlow, inviteToken]);

  // Auto-focus appropriate field on mount
  useEffect(() => {
    if (!isInviteFlow) {
      emailInputRef.current?.focus();
    } else if (!inviteLoading && !inviteFetchError) {
      // Focus password when email is pre-filled
      passwordInputRef.current?.focus();
    }
  }, [isInviteFlow, inviteLoading, inviteFetchError]);

  // Validation functions
  const validateEmail = (email: string) => {
    if (!email) return 'Email is required';
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return 'Invalid email format';
    return null;
  };

  const validatePassword = (password: string) => {
    if (!password) return 'Password is required';
    if (password.length < 8) return 'Password must be at least 8 characters';
    return null;
  };

  const validateOrgName = (name: string) => {
    const trimmed = name.trim();
    if (!trimmed) return 'Organization name is required';
    if (trimmed.length < 2) return 'Organization name must be at least 2 characters';
    if (trimmed.length > 100) return 'Organization name must be 100 characters or less';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Clear previous errors
    setErrors({});

    // Validate fields - orgName only required when NOT in invite flow
    const emailError = validateEmail(email);
    const orgNameError = !isInviteFlow ? validateOrgName(orgName) : null;
    const passwordError = validatePassword(password);

    if (emailError || orgNameError || passwordError) {
      setErrors({
        email: emailError || undefined,
        orgName: orgNameError || undefined,
        password: passwordError || undefined,
      });
      return;
    }

    try {
      if (isInviteFlow && inviteToken) {
        // Signup with invitation token - no org name needed
        await signup(email, password, undefined, inviteToken);

        // Success - redirect to dashboard with welcome message
        toast.success(`Welcome to ${inviteInfo?.org_name || 'the organization'}!`);
        window.location.hash = '#home';
      } else {
        // Regular signup with org name
        await signup(email, password, orgName.trim());
        handleAuthRedirect();
      }
    } catch (err: unknown) {
      // Extract error message from RFC 7807 Problem Details format
      const data = (err as { response?: { data?: Record<string, unknown> } }).response?.data;
      const errorObj = (data?.error as Record<string, unknown>) || data;
      let errorMessage =
        (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
        (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
        (typeof data?.error === 'string' && (data.error as string).trim()) ||
        (typeof (err as Error).message === 'string' && (err as Error).message.trim()) ||
        'Signup failed';

      // Ensure it's always a string (defensive coding)
      if (typeof errorMessage !== 'string') {
        errorMessage = JSON.stringify(errorMessage);
      }

      // Handle invitation-specific errors
      if (errorMessage.toLowerCase().includes('was sent to')) {
        // Redirect back to accept-invite with error
        window.location.hash = `#accept-invite?token=${inviteToken}&error=email_mismatch`;
        return;
      }
      if (errorMessage.toLowerCase().includes('expired') || errorMessage.toLowerCase().includes('invalid')) {
        window.location.hash = `#accept-invite?token=${inviteToken}&error=invalid`;
        return;
      }

      setErrors({ general: errorMessage });
    }
  };

  // Loading state for invite flow
  if (isInviteFlow && inviteLoading) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <Loader2 className="w-8 h-8 mx-auto mb-4 text-blue-400 animate-spin" />
          <p className="text-gray-400">Loading invitation...</p>
        </div>
      </div>
    );
  }

  // Error state for invite flow
  if (isInviteFlow && inviteFetchError) {
    return (
      <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
        <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md text-center">
          <h1 className="text-2xl font-semibold text-white mb-4">Invalid Invitation</h1>
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

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <h1 className="text-2xl font-semibold text-white mb-6">
          {isInviteFlow ? 'Create Account' : 'Sign Up'}
        </h1>

        {/* Invite banner - show org name */}
        {isInviteFlow && inviteInfo && (
          <div className="bg-blue-900/30 border border-blue-800 rounded-lg p-4 mb-6">
            <div className="flex items-center gap-3">
              <Building2 className="w-5 h-5 text-blue-400 flex-shrink-0" />
              <div>
                <p className="text-blue-200 text-sm">Joining organization</p>
                <p className="text-white font-medium">{inviteInfo.org_name}</p>
                <p className="text-blue-400 text-xs capitalize">as {inviteInfo.role}</p>
              </div>
            </div>
          </div>
        )}

        <form onSubmit={handleSubmit} className="space-y-4">
          {/* Email input */}
          <div>
            <label htmlFor="email" className="block text-sm font-medium text-gray-300 mb-2">
              Email
            </label>
            <input
              ref={emailInputRef}
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              onBlur={() => {
                const error = validateEmail(email);
                if (error) setErrors((prev) => ({ ...prev, email: error }));
              }}
              className={`w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500 ${
                isInviteFlow ? 'bg-gray-600 cursor-not-allowed' : ''
              }`}
              disabled={isLoading || isInviteFlow}
              readOnly={isInviteFlow}
            />
            {errors.email && <p className="text-red-400 text-sm mt-1">{errors.email}</p>}
            {isInviteFlow && (
              <p className="text-gray-500 text-xs mt-1">
                This email is set by the invitation and cannot be changed.
              </p>
            )}
          </div>

          {/* Organization name input - ONLY show when NOT in invite flow */}
          {!isInviteFlow && (
            <div>
              <label htmlFor="orgName" className="block text-sm font-medium text-gray-300 mb-2">
                Organization Name
              </label>
              <input
                id="orgName"
                type="text"
                value={orgName}
                onChange={(e) => setOrgName(e.target.value)}
                onBlur={() => {
                  const error = validateOrgName(orgName);
                  if (error) setErrors((prev) => ({ ...prev, orgName: error }));
                }}
                className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                disabled={isLoading}
                placeholder="Your company or team name"
              />
              {errors.orgName && <p className="text-red-400 text-sm mt-1">{errors.orgName}</p>}
              <p className="text-gray-500 text-xs mt-1">
                If your company is already using TrakRF, ask your admin for an invite instead of
                creating a new organization.
              </p>
            </div>
          )}

          {/* Password input with toggle */}
          <div>
            <label htmlFor="password" className="block text-sm font-medium text-gray-300 mb-2">
              Password
            </label>
            <div className="relative">
              <input
                ref={passwordInputRef}
                id="password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                onBlur={() => {
                  const error = validatePassword(password);
                  if (error) setErrors((prev) => ({ ...prev, password: error }));
                }}
                className="w-full px-4 py-2 pr-10 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                disabled={isLoading}
              />
              <button
                type="button"
                onClick={() => setShowPassword(!showPassword)}
                className="absolute right-3 top-1/2 -translate-y-1/2 text-gray-400 hover:text-gray-300"
                disabled={isLoading}
              >
                {showPassword ? <EyeOff className="w-5 h-5" /> : <Eye className="w-5 h-5" />}
              </button>
            </div>
            {errors.password && <p className="text-red-400 text-sm mt-1">{errors.password}</p>}
          </div>

          {/* General error */}
          {errors.general && (
            <div className="bg-red-900/20 border border-red-800 rounded-lg p-3">
              <p className="text-red-400 text-sm">{errors.general}</p>
            </div>
          )}

          {/* Submit button */}
          <button
            type="submit"
            disabled={isLoading}
            className="w-full bg-blue-600 hover:bg-blue-700 text-white py-2 px-4 rounded-lg font-medium transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
          >
            {isLoading
              ? 'Creating account...'
              : isInviteFlow
                ? 'Create Account & Join'
                : 'Sign Up'}
          </button>
        </form>

        {/* Navigation to login */}
        <p className="text-gray-400 text-sm mt-6 text-center">
          Already have an account?{' '}
          <a
            href={
              isInviteFlow && inviteToken
                ? `#login?returnTo=accept-invite&token=${encodeURIComponent(inviteToken)}`
                : '#login'
            }
            className="text-blue-400 hover:text-blue-300"
          >
            Log in
          </a>
        </p>
      </div>
    </div>
  );
}
