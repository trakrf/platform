/**
 * Handles post-authentication redirect logic
 *
 * Priority order:
 * 1. URL query params (from AcceptInviteScreen flow: #login?returnTo=...&token=...)
 * 2. sessionStorage (from ProtectedRoute flow)
 * 3. Default to home
 */
export function handleAuthRedirect(): void {
  // Priority 1: Check URL params (from AcceptInviteScreen flow)
  const hash = window.location.hash.slice(1); // Remove leading #
  const queryIndex = hash.indexOf('?');

  if (queryIndex !== -1) {
    const params = new URLSearchParams(hash.slice(queryIndex + 1));
    const returnTo = params.get('returnTo');
    const token = params.get('token');

    if (returnTo) {
      // Build redirect with preserved token if present
      const redirectHash = token
        ? `#${returnTo}?token=${encodeURIComponent(token)}`
        : `#${returnTo}`;
      window.location.hash = redirectHash;
      return;
    }
  }

  // Priority 2: Check sessionStorage (from ProtectedRoute flow)
  const redirect = sessionStorage.getItem('redirectAfterLogin');
  if (redirect) {
    sessionStorage.removeItem('redirectAfterLogin');
    window.location.hash = `#${redirect}`;
    return;
  }

  // Priority 3: Default to home
  window.location.hash = '#home';
}
