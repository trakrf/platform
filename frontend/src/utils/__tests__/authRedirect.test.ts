import { describe, it, expect, beforeEach } from 'vitest';
import { handleAuthRedirect } from '../authRedirect';

describe('handleAuthRedirect', () => {
  beforeEach(() => {
    // Clear sessionStorage before each test
    sessionStorage.clear();
    // Reset location.hash
    window.location.hash = '';
  });

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
