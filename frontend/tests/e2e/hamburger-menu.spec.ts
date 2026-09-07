/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Hamburger Menu E2E Tests
 * Tests mobile menu functionality, responsive behavior, and navigation
 * The hamburger menu only appears on mobile viewports (< lg breakpoint)
 */

import { test, expect, type Page } from '@playwright/test';

test.describe('Hamburger Menu (Mobile)', () => {
  let sharedPage: Page;

  test.beforeAll(async ({ browser }) => {
    sharedPage = await browser.newPage();
  });

  test.afterAll(async () => {
    if (sharedPage) {
      await sharedPage.close();
    }
  });

  test.describe('Basic Functionality', () => {
    test('should toggle menu open and closed on mobile', async () => {
      // Set mobile viewport first
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Find hamburger button (only visible on mobile)
      const hamburgerButton = sharedPage.locator('[data-testid="hamburger-button"]');
      await expect(hamburgerButton).toBeVisible();
      
      // Initially menu should be closed
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).not.toBeVisible();
      
      // Click hamburger - menu should open
      await hamburgerButton.click();
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
      
      // Close by clicking the OVERLAY, not a bare position on <body>.
      //
      // This was `sharedPage.click('body', { position: { x: 350, y: 100 } })`,
      // which raced the drawer's open animation: on a 375px viewport that point
      // can land on the drawer or on nothing depending on where the transition
      // has got to, and the click is then swallowed. It survived because
      // retries are 0 locally, so a failure looked deterministic and a pass
      // looked clean — it first showed as `flaky` on the CI arm that gates this
      // subset, where retries are 2 (TRA-1253).
      //
      // Waiting for the overlay is the pattern the sibling test below already
      // uses, and it removes the race rather than papering it with a timeout.
      const overlay = sharedPage.locator('[data-testid="mobile-menu-overlay"]');
      await expect(overlay).toBeVisible();
      await overlay.click({ force: true, position: { x: 300, y: 300 } });
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).not.toBeVisible();
      
      // Now test clicking button again to re-open
      await hamburgerButton.click();
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
    });
    
    test('should close menu when clicking overlay on mobile', async () => {
      // Set mobile viewport
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
      
      // Click overlay to close - use force to bypass any intercepting elements
      const overlay = sharedPage.locator('[data-testid="mobile-menu-overlay"]');
      await expect(overlay).toBeVisible();
      await overlay.click({ force: true, position: { x: 300, y: 300 } });
      
      // Menu should close
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).not.toBeVisible();
    });
    
    test('should navigate to different tabs on mobile', async () => {
      // Set mobile viewport
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
      
      // The mobile menu shows TabNavigation component
      // Look for tab buttons within the dropdown
      // Anchored on the testid, not the label. This read `has-text("Inventory")`
      // until TRA-1253 and the tab has been called "Scan" since TRA-1029, so the
      // locator matched nothing and the spec failed on a rename rather than on
      // any behaviour. `data-testid` survives relabelling; visible text does not.
      const scanTab = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-scan"]').first();
      const locateTab = sharedPage.locator('[data-testid="hamburger-dropdown"] button:has-text("Locate")').first();
      const settingsTab = sharedPage.locator('[data-testid="hamburger-dropdown"] button:has-text("Settings")').first();
      
      // Check that tabs exist
      await expect(scanTab).toBeVisible();
      await expect(locateTab).toBeVisible();
      await expect(settingsTab).toBeVisible();
      
      // Click on Settings tab
      await settingsTab.click();
      
      // Menu should close after navigation
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).not.toBeVisible();
      
      // Verify we navigated by checking for Settings-specific content
      await expect(sharedPage.getByText('Device Connection')).toBeVisible({ timeout: 5000 });
    });
    
    test('should highlight active tab in mobile menu', async () => {
      // Set mobile viewport
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
      
      // Scan should be active by default (based on the test IDs in TabNavigation)
      // Check within the dropdown container
      const scanTab = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-scan"]');
      await expect(scanTab).toBeVisible();

      // Check for active state - TabNavigation uses bg-blue-600 text-white for active
      const scanClasses = await scanTab.getAttribute('class');
      expect(scanClasses).toContain('bg-blue-600');
      expect(scanClasses).toContain('text-white');
      
      // Navigate to Settings - look within the dropdown
      const settingsTab = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-settings"]');
      await expect(settingsTab).toBeVisible();
      await settingsTab.click();
      
      // Re-open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      await expect(sharedPage.locator('[data-testid="hamburger-dropdown"]')).toBeVisible();
      
      // Now Settings should be highlighted
      const settingsTabReopened = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-settings"]');
      const settingsClasses = await settingsTabReopened.getAttribute('class');
      expect(settingsClasses).toContain('bg-blue-600');
      expect(settingsClasses).toContain('text-white');
      
      // And Scan should no longer be active
      const scanTabReopened = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-scan"]');
      const scanClassesAfter = await scanTabReopened.getAttribute('class');
      expect(scanClassesAfter).not.toContain('bg-blue-600');
    });
  });
  
  test.describe('Responsive Behavior', () => {
    test('hamburger button only visible on mobile viewports', async () => {
      // Test desktop viewport - hamburger should be hidden
      await sharedPage.setViewportSize({ width: 1280, height: 800 });
      await sharedPage.goto('/');
      
      const hamburgerButton = sharedPage.locator('[data-testid="hamburger-button"]');
      await expect(hamburgerButton).not.toBeVisible();
      
      // Test mobile viewport - hamburger should be visible
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await expect(hamburgerButton).toBeVisible();
    });
    
    test('menu drawer positioned correctly on mobile', async () => {
      // Set mobile viewport
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      
      // Check menu positioning (should be fixed left-0 top-0)
      const dropdown = sharedPage.locator('[data-testid="hamburger-dropdown"]');
      const box = await dropdown.boundingBox();
      
      expect(box).toBeTruthy();
      expect(box!.x).toBe(0); // Should be at left edge
      expect(box!.y).toBe(0); // Should be at top
      expect(box!.width).toBe(256); // w-64 = 16rem = 256px (from the implementation)
      expect(box!.height).toBeGreaterThan(400); // Should be close to full height
    });
    
    test('menu has proper touch targets on mobile', async () => {
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      await sharedPage.click('[data-testid="hamburger-button"]');
      
      // Check navigation buttons exist and are visible
      const navButtons = sharedPage.locator('[data-testid="hamburger-dropdown"] button');
      const buttonCount = await navButtons.count();
      
      // Should have at least 3 nav buttons (Scan, Locate, Settings)
      expect(buttonCount).toBeGreaterThanOrEqual(3);
      
      // Verify all buttons are visible and clickable
      for (let i = 0; i < Math.min(buttonCount, 3); i++) {
        await expect(navButtons.nth(i)).toBeVisible();
      }
    });
    
    test('overlay covers entire screen on mobile', async () => {
      await sharedPage.setViewportSize({ width: 375, height: 667 });
      await sharedPage.goto('/');
      
      // Open menu
      await sharedPage.click('[data-testid="hamburger-button"]');
      
      // Check overlay dimensions
      const overlay = sharedPage.locator('[data-testid="mobile-menu-overlay"]');
      const box = await overlay.boundingBox();
      
      expect(box).toBeTruthy();
      expect(box!.x).toBe(0);
      expect(box!.y).toBe(0);
      expect(box!.width).toBe(375); // Full viewport width
      expect(box!.height).toBe(667); // Full viewport height
    });
  });
  
  test.describe('Desktop Behavior', () => {
    test('sidebar visible on desktop without hamburger', async () => {
      // Set desktop viewport
      await sharedPage.setViewportSize({ width: 1280, height: 800 });
      await sharedPage.goto('/');

      // Hamburger button should not be visible
      const hamburgerButton = sharedPage.locator('[data-testid="hamburger-button"]');
      await expect(hamburgerButton).not.toBeVisible();

      // Desktop sidebar should be visible at xl breakpoint and above
      const sidebar = sharedPage.locator('[data-testid="desktop-sidebar"]');
      await expect(sidebar).toBeVisible();
      
      // Navigation tabs should be directly accessible
      // Same rename as above — "Inventory" became "Scan" in TRA-1029.
      const scanTab = sharedPage.locator('[data-testid="menu-item-scan"]').first();
      await expect(scanTab).toBeVisible();
    });
  });
});