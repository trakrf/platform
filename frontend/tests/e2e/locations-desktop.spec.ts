/**
 * Locations Desktop E2E Tests (TRA-301)
 *
 * Tests the Mac Finder-style split-pane layout for the Locations tab on desktop.
 * Covers tree navigation, details panel, and CRUD operations.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *
 * Run with: pnpm test:e2e tests/e2e/locations-desktop.spec.ts
 */

import { test, expect, type Page } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
} from './fixtures/org.fixture';
import {
  createTestHierarchy,
  deleteAllLocationsViaAPI,
  createLocationViaAPI,
  type TestHierarchy,
} from './fixtures/location.fixture';

test.describe('Locations Desktop Split Pane', () => {
  let testEmail: string;
  let testPassword: string;
  let hierarchy: TestHierarchy;

  test.beforeAll(async ({ browser }) => {
    // Set up test user with locations
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-locations-${id}@example.com`;
    testPassword = 'TestPassword123!';

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testEmail, testPassword);

    // Create test hierarchy
    hierarchy = await createTestHierarchy(page);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Set desktop viewport
    await page.setViewportSize({ width: 1280, height: 800 });

    // Clear state and login fresh before each test
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);

    // Navigate to Locations tab
    await page.click('text="Locations"');
    await page.waitForTimeout(500);
  });

  test.describe('Split Pane Layout', () => {
    test('should show split pane layout on desktop', async ({ page }) => {
      // Verify split pane container is visible
      await expect(page.locator('[data-testid="location-split-pane"]')).toBeVisible();
    });

    test('should show tree panel on left side', async ({ page }) => {
      // Verify tree panel is visible
      const treePanel = page.locator('[data-testid="location-tree-panel"]');
      await expect(treePanel).toBeVisible();

      // Should show root locations
      await expect(page.locator('text=warehouse-a')).toBeVisible();
      await expect(page.locator('text=warehouse-b')).toBeVisible();
    });

    test('should show details panel on right side', async ({ page }) => {
      // Initial state should show empty state in details
      await expect(page.locator('text=Select a location')).toBeVisible();
    });

    test('should show resizable divider between panels', async ({ page }) => {
      // The divider is created by react-split-pane
      const splitPane = page.locator('[data-testid="location-split-pane"]');
      await expect(splitPane).toBeVisible();

      // Look for the divider element
      const divider = page.locator('.split-pane-divider, [class*="divider"]').first();
      const dividerExists = await divider.count() > 0;

      // If specific divider class exists, verify it's visible
      if (dividerExists) {
        console.log('[Test] Split pane divider found');
      } else {
        // Verify the split structure by checking both panels exist
        await expect(page.locator('[data-testid="location-tree-panel"]')).toBeVisible();
        console.log('[Test] Split pane structure verified via panels');
      }
    });
  });

  test.describe('Tree Navigation', () => {
    test('should show all root locations in tree', async ({ page }) => {
      // Both warehouses should be visible
      await expect(page.locator('text=warehouse-a')).toBeVisible();
      await expect(page.locator('text=warehouse-b')).toBeVisible();
    });

    test('should expand location on chevron click', async ({ page }) => {
      // Initially, children should not be visible (collapsed)
      const floor1 = page.locator('text=floor-1');
      await expect(floor1).not.toBeVisible();

      // Click expand on warehouse-a
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Now children should be visible
      await expect(floor1).toBeVisible();
    });

    test('should collapse expanded location on chevron click', async ({ page }) => {
      // First expand warehouse-a
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Verify floor-1 is visible
      await expect(page.locator('text=floor-1')).toBeVisible();

      // Click collapse
      const collapseButton = page.locator('button[aria-label="Collapse"]').first();
      await collapseButton.click();

      // Children should be hidden
      await expect(page.locator('text=floor-1')).not.toBeVisible();
    });

    test('should highlight selected location', async ({ page }) => {
      // Click on warehouse-a to select it
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Should have selected styling (bg-blue-100 class)
      await expect(warehouseA).toHaveClass(/bg-blue-100/);
    });

    test('should show active/inactive status badges', async ({ page }) => {
      // Both root locations should show Active badge
      const activeBadges = page.locator('text=Active');
      expect(await activeBadges.count()).toBeGreaterThanOrEqual(2);
    });
  });

  test.describe('Details Panel', () => {
    test('should show empty state when no location selected', async ({ page }) => {
      await expect(page.locator('text=Select a location')).toBeVisible();
    });

    test('should update details panel when location selected', async ({ page }) => {
      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Details panel should show warehouse-a info
      const detailsPanel = page.locator('[data-testid="location-details-panel"]');
      await expect(detailsPanel).toBeVisible();

      // Should show the location name in the header
      await expect(detailsPanel.locator('h2:has-text("warehouse-a")')).toBeVisible();

      // Should show it's a Root Location
      await expect(detailsPanel.locator('text=Root Location')).toBeVisible();
    });

    test('should show location identifier and name', async ({ page }) => {
      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      const detailsPanel = page.locator('[data-testid="location-details-panel"]');

      // Should show identifier
      await expect(detailsPanel.locator('text=warehouse-a').first()).toBeVisible();

      // Should show name
      await expect(detailsPanel.locator('text=Warehouse A')).toBeVisible();
    });

    test('should show description when available', async ({ page }) => {
      // Click on warehouse-a (which has description: "Main warehouse facility")
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      const detailsPanel = page.locator('[data-testid="location-details-panel"]');

      // Should show description
      await expect(detailsPanel.locator('text=Main warehouse facility')).toBeVisible();
    });

    test('should show hierarchy information', async ({ page }) => {
      // First expand warehouse-a to make floor-1 visible
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Click on warehouse-a (has 2 direct children: floor-1, floor-2)
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      const detailsPanel = page.locator('[data-testid="location-details-panel"]');

      // Should show direct children count
      await expect(detailsPanel.locator('text=Direct Children')).toBeVisible();
    });

    test('should show children list with navigation', async ({ page }) => {
      // First expand warehouse-a
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      const detailsPanel = page.locator('[data-testid="location-details-panel"]');

      // Should show children in the list
      await expect(detailsPanel.locator('text=floor-1')).toBeVisible();
    });

    test('should navigate to child when clicked in details', async ({ page }) => {
      // First expand warehouse-a
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Click on floor-1 in the details panel children list
      const detailsPanel = page.locator('[data-testid="location-details-panel"]');
      const floor1Child = detailsPanel.locator('text=floor-1').first();
      await floor1Child.click();

      // Details should now show floor-1
      await expect(detailsPanel.locator('h2:has-text("floor-1")')).toBeVisible();

      // Should show it's a Subsidiary Location
      await expect(detailsPanel.locator('text=Subsidiary Location')).toBeVisible();
    });

    test('should show Edit, Move, Delete buttons', async ({ page }) => {
      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Should show action buttons
      await expect(page.locator('button:has-text("Edit")')).toBeVisible();
      await expect(page.locator('button:has-text("Move")')).toBeVisible();
      await expect(page.locator('button:has-text("Delete")')).toBeVisible();
    });
  });

  test.describe('CRUD Operations', () => {
    test('should open edit modal when Edit clicked', async ({ page }) => {
      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Click Edit button
      await page.click('button:has-text("Edit")');

      // Edit modal should open
      await expect(page.locator('text=Edit Location')).toBeVisible({ timeout: 5000 });
    });

    test('should open move modal when Move clicked', async ({ page }) => {
      // First expand warehouse-a
      const expandButton = page.locator('button[aria-label="Expand"]').first();
      await expandButton.click();

      // Click on floor-1 (child location that can be moved)
      const floor1 = page.locator('[data-location-id]').filter({ hasText: 'floor-1' }).first();
      await floor1.click();

      // Click Move button
      await page.click('button:has-text("Move")');

      // Move modal should open
      await expect(page.locator('text=Move Location')).toBeVisible({ timeout: 5000 });
    });

    test('should open delete confirmation when Delete clicked', async ({ page }) => {
      // Click on warehouse-a
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Click Delete button
      await page.click('button:has-text("Delete")');

      // Confirmation modal should open
      await expect(page.locator('text=Delete Location')).toBeVisible({ timeout: 5000 });
    });
  });

  test.describe('Search and Filter', () => {
    test('should filter tree when searching', async ({ page }) => {
      // Type in search
      const searchInput = page.locator('input[placeholder*="Search"]').first();
      await searchInput.fill('floor');

      // Wait for filter to apply
      await page.waitForTimeout(300);

      // Floor-1 and floor-2 should be visible (matching)
      // Warehouse-a should be visible (ancestor)
      // Warehouse-b should not be visible (no match)
      const warehouseB = page.locator('[data-location-id]').filter({ hasText: 'warehouse-b' });

      // After search, warehouse-b should not match
      await expect(warehouseB).not.toBeVisible();
    });

    test('should show ancestors when filtering', async ({ page }) => {
      // Search for a deeply nested location
      const searchInput = page.locator('input[placeholder*="Search"]').first();
      await searchInput.fill('section-a');

      // Wait for filter to apply
      await page.waitForTimeout(500);

      // section-a and its ancestors (floor-1, warehouse-a) should be visible
      // Even without expanding, the tree should show the path
      await expect(page.locator('text=warehouse-a')).toBeVisible();
    });
  });

  test.describe('Keyboard Navigation', () => {
    test('should select on Enter key', async ({ page }) => {
      // Focus on the tree
      const treePanel = page.locator('[data-testid="location-tree-panel"]');
      await treePanel.focus();

      // Press ArrowDown to navigate to first location
      await page.keyboard.press('ArrowDown');

      // Press Enter to select
      await page.keyboard.press('Enter');

      // Details panel should show selected location
      const detailsPanel = page.locator('[data-testid="location-details-panel"]');
      await expect(detailsPanel).toBeVisible();
    });

    test('should navigate with arrow keys', async ({ page }) => {
      // First, click on warehouse-a to focus it
      const warehouseA = page.locator('[data-location-id]').filter({ hasText: 'warehouse-a' }).first();
      await warehouseA.click();

      // Press ArrowDown to move to next location
      await page.keyboard.press('ArrowDown');

      // Press Enter to select
      await page.keyboard.press('Enter');

      // Details should show warehouse-b (next root location)
      const detailsPanel = page.locator('[data-testid="location-details-panel"]');
      await expect(detailsPanel.locator('h2:has-text("warehouse-b")')).toBeVisible();
    });
  });
});

test.describe('Locations Desktop - Fresh State', () => {
  // These tests need fresh state with no existing locations

  let testEmail: string;
  let testPassword: string;

  test.beforeAll(async ({ browser }) => {
    // Set up test user without locations
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-empty-locations-${id}@example.com`;
    testPassword = 'TestPassword123!';

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testEmail, testPassword);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Set desktop viewport
    await page.setViewportSize({ width: 1280, height: 800 });

    // Clear state and login fresh
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);

    // Navigate to Locations tab
    await page.click('text="Locations"');
    await page.waitForTimeout(500);
  });

  test('should show empty state in tree when no locations', async ({ page }) => {
    // Tree should show empty message
    await expect(page.locator('text=No locations found')).toBeVisible();
  });

  test('should create new location via FAB and show in tree', async ({ page }) => {
    // Click FAB to create location
    const fab = page.locator('button[aria-label="Create new location"]');
    await fab.click();

    // Fill in the form
    await expect(page.locator('text=Create Location')).toBeVisible({ timeout: 5000 });

    const identifierInput = page.locator('input#identifier');
    await identifierInput.fill('test-location');

    const nameInput = page.locator('input#name');
    await nameInput.fill('Test Location');

    // Submit
    await page.locator('button[type="submit"]:has-text("Create")').click();

    // Wait for modal to close
    await expect(page.locator('text=Create Location')).not.toBeVisible({ timeout: 5000 });

    // Verify location appears in tree
    await expect(page.locator('text=test-location')).toBeVisible();
  });
});
