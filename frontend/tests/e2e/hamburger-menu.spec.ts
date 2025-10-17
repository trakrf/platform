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
      
      // Click outside to close (using body click at safe position)
      await sharedPage.click('body', { position: { x: 350, y: 100 } });
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
      const inventoryTab = sharedPage.locator('[data-testid="hamburger-dropdown"] button:has-text("Inventory")').first();
      const locateTab = sharedPage.locator('[data-testid="hamburger-dropdown"] button:has-text("Locate")').first();
      const settingsTab = sharedPage.locator('[data-testid="hamburger-dropdown"] button:has-text("Settings")').first();
      
      // Check that tabs exist
      await expect(inventoryTab).toBeVisible();
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
      
      // Home should be active by default (based on the test IDs in TabNavigation)
      // Check within the dropdown container
      const homeTab = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-home"]');
      await expect(homeTab).toBeVisible();
      
      // Check for active state - TabNavigation uses bg-blue-600 text-white for active
      const homeClasses = await homeTab.getAttribute('class');
      expect(homeClasses).toContain('bg-blue-600');
      expect(homeClasses).toContain('text-white');
      
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
      
      // And Home should no longer be active
      const homeTabReopened = sharedPage.locator('[data-testid="hamburger-dropdown"] [data-testid="menu-item-home"]');
      const homeClassesAfter = await homeTabReopened.getAttribute('class');
      expect(homeClassesAfter).not.toContain('bg-blue-600');
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
      
      // Should have at least 3 nav buttons (Inventory, Locate, Settings)
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
      
      // Desktop sidebar should be visible (hidden lg:flex)
      const sidebar = sharedPage.locator('.hidden.lg\\:flex').first();
      await expect(sidebar).toBeVisible();
      
      // Navigation tabs should be directly accessible
      const inventoryTab = sharedPage.locator('button:has-text("Inventory")').first();
      await expect(inventoryTab).toBeVisible();
    });
  });
});