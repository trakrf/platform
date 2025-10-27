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
      await expect(page.locator('input#organizationName')).toBeVisible();
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
      await page.locator('input#organizationName').fill('A');
      await page.locator('input#organizationName').blur();

      // Should show validation error
      await expect(page.locator('text=Organization name must be at least 2 characters')).toBeVisible();
    });

    test('should handle signup failure with proper error message', async ({ page }) => {
      await page.goto('/#signup');

      // Fill in credentials with existing email (should fail)
      await page.locator('input#email').fill('existing@example.com');
      await page.locator('input#password').fill('password123');
      await page.locator('input#organizationName').fill('Test Organization');

      // Submit form
      await page.locator('button[type="submit"]').click();

      // Should show loading state
      await expect(page.locator('button[type="submit"]')).toContainText('Signing up...');

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
