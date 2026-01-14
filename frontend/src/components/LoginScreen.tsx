import { useState, useEffect, useRef } from 'react';
import { useAuthStore } from '@/stores';
import { Eye, EyeOff } from 'lucide-react';
import { handleAuthRedirect } from '@/utils/authRedirect';

// Extract email from URL params (for invite flow pre-population)
function getEmailFromUrl(): string {
  const hash = window.location.hash;
  const queryIndex = hash.indexOf('?');
  if (queryIndex === -1) return '';
  const searchParams = new URLSearchParams(hash.substring(queryIndex + 1));
  return searchParams.get('email') || '';
}

export default function LoginScreen() {
  const initialEmail = getEmailFromUrl();
  const [email, setEmail] = useState(initialEmail);
  const [password, setPassword] = useState('');
  const [showPassword, setShowPassword] = useState(false);
  const [errors, setErrors] = useState<{ email?: string; password?: string; general?: string }>({});
  const emailInputRef = useRef<HTMLInputElement>(null);
  const passwordInputRef = useRef<HTMLInputElement>(null);

  const { login, isLoading } = useAuthStore();

  // Auto-focus appropriate field on mount
  useEffect(() => {
    if (initialEmail) {
      // Email pre-filled, focus password
      passwordInputRef.current?.focus();
    } else {
      emailInputRef.current?.focus();
    }
  }, [initialEmail]);

  // Validation on blur
  const validateEmail = (email: string) => {
    if (!email) return 'Email is required';
    if (!/^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(email)) return 'Invalid email format';
    return null;
  };

  const validatePassword = (password: string) => {
    if (!password) return 'Password is required';
    return null;
  };

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();

    // Clear previous errors
    setErrors({});

    // Validate all fields
    const emailError = validateEmail(email);
    const passwordError = validatePassword(password);

    if (emailError || passwordError) {
      setErrors({
        email: emailError || undefined,
        password: passwordError || undefined,
      });
      return;
    }

    try {
      await login(email, password);

      // Handle redirect after successful login
      handleAuthRedirect();
    } catch (err: unknown) {
      // Extract error message from RFC 7807 Problem Details format
      // Handle empty strings by checking truthy AND non-empty
      const data = (err as any).response?.data;
      const errorObj = data?.error || data; // Handle both nested and flat structures
      let errorMessage =
        (typeof errorObj?.detail === 'string' && errorObj.detail.trim()) ||
        (typeof errorObj?.title === 'string' && errorObj.title.trim()) ||
        (typeof data?.error === 'string' && data.error.trim()) ||
        (typeof (err as Error).message === 'string' && (err as Error).message.trim()) ||
        'Login failed';

      // Ensure it's always a string (defensive coding)
      if (typeof errorMessage !== 'string') {
        errorMessage = JSON.stringify(errorMessage);
      }

      setErrors({ general: errorMessage });
    }
  };

  return (
    <div className="min-h-screen bg-gray-900 flex items-center justify-center p-4">
      <div className="bg-gray-800 p-8 rounded-lg w-full max-w-md">
        <h1 className="text-2xl font-semibold text-white mb-6">Log In</h1>

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
                if (error) setErrors(prev => ({ ...prev, email: error }));
              }}
              className="w-full px-4 py-2 border border-gray-600 bg-gray-700 text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              disabled={isLoading}
            />
            {errors.email && (
              <p className="text-red-400 text-sm mt-1">{errors.email}</p>
            )}
          </div>

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
                  if (error) setErrors(prev => ({ ...prev, password: error }));
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
            {errors.password && (
              <p className="text-red-400 text-sm mt-1">{errors.password}</p>
            )}
            <div className="text-right mt-1">
              <a href="#forgot-password" className="text-sm text-blue-400 hover:text-blue-300">
                Forgot password?
              </a>
            </div>
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
            {isLoading ? 'Logging in...' : 'Log In'}
          </button>
        </form>

        {/* Navigation to signup */}
        <p className="text-gray-400 text-sm mt-6 text-center">
          Don&apos;t have an account?{' '}
          <a href="#signup" className="text-blue-400 hover:text-blue-300">
            Sign up
          </a>
        </p>
      </div>
    </div>
  );
}
