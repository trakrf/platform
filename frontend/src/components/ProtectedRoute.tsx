import { useAuthStore } from '@/stores/authStore';
import { useEffect } from 'react';

interface ProtectedRouteProps {
  children: React.ReactNode;
}

export function ProtectedRoute({ children }: ProtectedRouteProps) {
  const { isAuthenticated } = useAuthStore();

  useEffect(() => {
    if (!isAuthenticated) {
      // Save current hash for redirect after login
      const currentHash = window.location.hash.slice(1); // Remove '#'

      // Only save if it's not login/signup (avoid loops)
      if (currentHash && currentHash !== 'login' && currentHash !== 'signup') {
        sessionStorage.setItem('redirectAfterLogin', currentHash);
      }

      // Redirect to login
      window.location.hash = '#login';
    }
  }, [isAuthenticated]);

  // Don't render children if not authenticated (prevents flash of content)
  if (!isAuthenticated) {
    return null;
  }

  return <>{children}</>;
}
