/**
 * Handles post-authentication redirect logic
 * Checks sessionStorage for saved redirect path, falls back to home
 */
export function handleAuthRedirect(): void {
  const redirect = sessionStorage.getItem('redirectAfterLogin');

  if (redirect) {
    // Clear before redirecting to avoid loops
    sessionStorage.removeItem('redirectAfterLogin');
    window.location.hash = `#${redirect}`;
  } else {
    window.location.hash = '#home';
  }
}
