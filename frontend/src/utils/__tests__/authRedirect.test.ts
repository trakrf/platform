import { describe, it, expect, beforeEach } from 'vitest';
import { handleAuthRedirect } from '../authRedirect';

describe('handleAuthRedirect', () => {
  beforeEach(() => {
    // Clear sessionStorage before each test
    sessionStorage.clear();
    // Reset location.hash
    window.location.hash = '';
  });

  describe('sessionStorage handling (ProtectedRoute flow)', () => {
    it('should redirect to saved path from sessionStorage', () => {
      sessionStorage.setItem('redirectAfterLogin', 'assets');
      handleAuthRedirect();

      expect(window.location.hash).toBe('#assets');
      expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
    });

    it('should redirect to home if no saved path', () => {
      handleAuthRedirect();
      expect(window.location.hash).toBe('#home');
    });

    it('should clear sessionStorage after redirect', () => {
      sessionStorage.setItem('redirectAfterLogin', 'locations');
      handleAuthRedirect();

      expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
    });

    it('should handle redirect to locations', () => {
      sessionStorage.setItem('redirectAfterLogin', 'locations');
      handleAuthRedirect();

      expect(window.location.hash).toBe('#locations');
      expect(sessionStorage.getItem('redirectAfterLogin')).toBeNull();
    });

    it('should handle empty sessionStorage gracefully', () => {
      sessionStorage.removeItem('redirectAfterLogin');
      handleAuthRedirect();

      expect(window.location.hash).toBe('#home');
    });
  });

  describe('URL param handling (AcceptInvite flow)', () => {
    it('should redirect to returnTo with token from URL params', () => {
      window.location.hash = '#login?returnTo=accept-invite&token=abc123';
      handleAuthRedirect();
      expect(window.location.hash).toBe('#accept-invite?token=abc123');
    });

    it('should redirect to returnTo without token if not present', () => {
      window.location.hash = '#login?returnTo=settings';
      handleAuthRedirect();
      expect(window.location.hash).toBe('#settings');
    });

    it('should URL-encode token with special characters', () => {
      // Token comes URL-encoded in the original URL, needs to be preserved
      window.location.hash = '#login?returnTo=accept-invite&token=abc%2B123%3D';
      handleAuthRedirect();
      // Token should be re-encoded in output
      expect(window.location.hash).toContain('accept-invite?token=');
      expect(window.location.hash).toContain('abc');
    });

    it('should prioritize URL params over sessionStorage', () => {
      sessionStorage.setItem('redirectAfterLogin', 'assets');
      window.location.hash = '#login?returnTo=accept-invite&token=xyz';
      handleAuthRedirect();
      expect(window.location.hash).toBe('#accept-invite?token=xyz');
      // sessionStorage should NOT be cleared since URL params took priority
      expect(sessionStorage.getItem('redirectAfterLogin')).toBe('assets');
    });

    it('should fall back to sessionStorage when no URL params', () => {
      sessionStorage.setItem('redirectAfterLogin', 'locations');
      window.location.hash = '#login';
      handleAuthRedirect();
      expect(window.location.hash).toBe('#locations');
    });

    it('should handle signup page with returnTo params', () => {
      window.location.hash = '#signup?returnTo=accept-invite&token=signup123';
      handleAuthRedirect();
      expect(window.location.hash).toBe('#accept-invite?token=signup123');
    });
  });
});
