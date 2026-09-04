import { test, expect } from '@playwright/test';

/**
 * Auth E2E Tests
 *
 * Tests login and signup flows including error handling.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *
 * Run with: pnpm test:e2e tests/e2e/auth.spec.ts
 */

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    // Clear any existing auth state
    await page.goto('/');
    await page.evaluate(() => {
      localStorage.clear();
      sessionStorage.clear();
    });
    // Hard reload to ensure fresh code
    await page.reload({ waitUntil: 'networkidle' });
  });

  test.describe('Login Screen', () => {
    test('should display login form', async ({ page }) => {
      await page.goto('/#login');

      // Verify login form elements are present
      await expect(page.getByRole('heading', { name: 'Log In' })).toBeVisible();
      await expect(page.locator('input#email')).toBeVisible();
      await expect(page.locator('input#password')).toBeVisible();
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    });

    test('should show validation errors for empty fields', async ({ page }) => {
      await page.goto('/#login');

      // Click submit without filling fields
      await page.locator('button[type="submit"]').click();

      // Should show validation errors
      await expect(page.locator('text=Email is required')).toBeVisible();
      await expect(page.locator('text=Password is required')).toBeVisible();
    });

    test('should show error for invalid email format', async ({ page }) => {
      await page.goto('/#login');

      // Enter invalid email
      await page.locator('input#email').fill('invalid-email');
      await page.locator('input#email').blur();

      // Should show validation error
      await expect(page.locator('text=Invalid email format')).toBeVisible();
    });

    test('should handle login failure with proper error message', async ({ page }) => {
      await page.goto('/#login');

      // Fill in credentials
      await page.locator('input#email').fill('test@example.com');
      await page.locator('input#password').fill('wrongpassword');

      // Submit form
      await page.locator('button[type="submit"]').click();

      // Should show loading state
      await expect(page.locator('button[type="submit"]')).toContainText('Logging in...');

      // Wait for error (backend should return RFC 7807 error)
      // The error should be displayed as text, not as an object
      const errorContainer = page.locator('.bg-red-900\\/20');
      await expect(errorContainer).toBeVisible({ timeout: 10000 });

      // Verify it's showing a string message, not "[object Object]"
      const errorText = await errorContainer.textContent();
      expect(errorText).not.toContain('[object Object]');
      expect(errorText).toBeTruthy();
    });

    test('should toggle password visibility', async ({ page }) => {
      await page.goto('/#login');

      const passwordInput = page.locator('input#password');
      const toggleButton = page.locator('button[type="button"]').first();

      // Initially should be password type
      await expect(passwordInput).toHaveAttribute('type', 'password');

      // Click toggle
      await toggleButton.click();
      await expect(passwordInput).toHaveAttribute('type', 'text');

      // Click again to hide
      await toggleButton.click();
      await expect(passwordInput).toHaveAttribute('type', 'password');
    });

    test('should navigate to signup', async ({ page }) => {
      await page.goto('/#login');

      // Click signup link
      await page.locator('a[href="#signup"]').click();

      // Should navigate to signup
      await expect(page).toHaveURL(/#signup/);
      await expect(page.getByRole('heading', { name: 'Sign Up' })).toBeVisible();
    });
  });

  test.describe('Signup Screen', () => {
    test('should display signup form', async ({ page }) => {
      await page.goto('/#signup');

      // Verify signup form elements are present
      await expect(page.getByRole('heading', { name: 'Sign Up' })).toBeVisible();
      await expect(page.locator('input#email')).toBeVisible();
      await expect(page.locator('input#password')).toBeVisible();
      await expect(page.locator('input#orgName')).toBeVisible();
      await expect(page.locator('button[type="submit"]')).toBeVisible();
    });

    test('should show validation errors for empty fields', async ({ page }) => {
      await page.goto('/#signup');

      // Click submit without filling fields
      await page.locator('button[type="submit"]').click();

      // Should show validation errors
      await expect(page.locator('text=Email is required')).toBeVisible();
      await expect(page.locator('text=Password is required')).toBeVisible();
      await expect(page.locator('text=Organization name is required')).toBeVisible();
    });

    test('should validate password length', async ({ page }) => {
      await page.goto('/#signup');

      // Enter short password
      await page.locator('input#password').fill('short');
      await page.locator('input#password').blur();

      // Should show validation error
      await expect(page.locator('text=Password must be at least 8 characters')).toBeVisible();
    });

    test('should validate organization name length', async ({ page }) => {
      await page.goto('/#signup');

      // Enter short org name
      await page.locator('input#orgName').fill('A');
      await page.locator('input#orgName').blur();

      // Should show validation error
      await expect(page.locator('text=Organization name must be at least 2 characters')).toBeVisible();
    });

    test('should handle signup failure with proper error message', async ({ page }) => {
      /*
       * Two separate reasons this could not pass, both fixed here (TRA-1246).
       *
       * 1. It filled email, password and orgName only. TRA-970/971 made name,
       *    website, phone and the non-prod acknowledgement required too, so the
       *    form failed client-side validation and never submitted — no request,
       *    no server error, nothing for the assertions below to find.
       *    org.fixture.ts carries a comment about exactly this happening to
       *    `signupTestUser`; the same rot sat undetected one file over.
       *
       * 2. It then asserted the transient "Creating account..." label, which
       *    SignupScreen renders only while `isLoading`. That assertion failed
       *    first and hid reason 1 for as long as it stood. It is gone: a label
       *    that exists for one round trip cannot be observed reliably from
       *    outside the app, and it is not what this test is named for.
       *
       * The duplicate is now created by this test rather than assumed to be in
       * the database, so the failure it asserts on is the one it arranges.
       */
      const takenEmail = `duplicate-${Date.now()}@example.com`;
      const fillSignupForm = async (email: string, orgName: string) => {
        await page.locator('input#email').fill(email);
        await page.locator('input#name').fill('E2E Test User');
        await page.locator('input#orgName').fill(orgName);
        await page.locator('input#website').fill('example.com');
        await page.locator('input#phone').fill('+1 555 123 4567');
        await page.locator('input#password').fill('password123');
        const ack = page.locator('input#ackNonProd');
        if (await ack.count()) await ack.check();
      };

      // Claim the address with a signup that succeeds.
      await page.goto('/#signup');
      await fillSignupForm(takenEmail, `Dup Test Org ${Date.now()}`);
      await page.locator('button[type="submit"]').click();
      await page.waitForURL(/#scan/, { timeout: 10000 });

      // Now sign up again with the same address, from a clean session.
      await page.evaluate(() => localStorage.clear());
      await page.goto('/#signup');
      await page.reload({ waitUntil: 'networkidle' });
      await fillSignupForm(takenEmail, `Dup Test Org 2 ${Date.now()}`);
      await page.locator('button[type="submit"]').click();

      // Wait for error (backend should return RFC 7807 error)
      // The error should be displayed as text, not as an object
      const errorContainer = page.locator('.bg-red-900\\/20');
      await expect(errorContainer).toBeVisible({ timeout: 10000 });

      // Verify it's showing a string message, not "[object Object]"
      const errorText = await errorContainer.textContent();
      expect(errorText).not.toContain('[object Object]');
      expect(errorText).toBeTruthy();
    });

    test('should toggle password visibility', async ({ page }) => {
      await page.goto('/#signup');

      const passwordInput = page.locator('input#password');
      const toggleButton = page.locator('button[type="button"]').first();

      // Initially should be password type
      await expect(passwordInput).toHaveAttribute('type', 'password');

      // Click toggle
      await toggleButton.click();
      await expect(passwordInput).toHaveAttribute('type', 'text');

      // Click again to hide
      await toggleButton.click();
      await expect(passwordInput).toHaveAttribute('type', 'password');
    });

    test('should navigate to login', async ({ page }) => {
      await page.goto('/#signup');

      // Click login link
      await page.locator('a[href="#login"]').click();

      // Should navigate to login
      await expect(page).toHaveURL(/#login/);
      await expect(page.getByRole('heading', { name: 'Log In' })).toBeVisible();
    });
  });

  test.describe('Error Object Rendering Bug', () => {
    test('should not render error object as React child', async ({ page }) => {
      // This test specifically verifies the fix for the bug where
      // RFC 7807 error objects were being rendered directly as React children

      await page.goto('/#login');

      // Fill in invalid credentials
      await page.locator('input#email').fill('test@example.com');
      await page.locator('input#password').fill('wrongpassword');

      // Submit form
      await page.locator('button[type="submit"]').click();

      // Wait for error response
      await page.waitForTimeout(2000);

      // Should not see React error about rendering objects
      const pageContent = await page.content();
      expect(pageContent).not.toContain('Objects are not valid as a React child');
      expect(pageContent).not.toContain('found: object with keys');

      // Should see actual error message
      const errorContainer = page.locator('.bg-red-900\\/20');
      if (await errorContainer.isVisible()) {
        const errorText = await errorContainer.textContent();
        // Should be a proper error message, not stringified object
        expect(errorText).not.toContain('{');
        expect(errorText).not.toContain('type');
        expect(errorText).not.toContain('request_id');
      }
    });
  });
});
