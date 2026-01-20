/**
 * Locations Mobile E2E Tests (TRA-301)
 *
 * Tests the expandable card layout for the Locations tab on mobile/tablet.
 * Covers card expand/collapse, nested children, and CRUD operations.
 *
 * Prerequisites:
 * - Backend API running on http://localhost:8080
 * - Frontend dev server running on http://localhost:5173
 *
 * Run with: pnpm test:e2e tests/e2e/locations-mobile.spec.ts
 */

import { test, expect } from '@playwright/test';
import {
  uniqueId,
  clearAuthState,
  signupTestUser,
  loginTestUser,
} from './fixtures/org.fixture';
import {
  createTestHierarchy,
  type TestHierarchy,
} from './fixtures/location.fixture';

test.describe('Locations Mobile Expandable Cards', () => {
  let testEmail: string;
  let testPassword: string;
  let hierarchy: TestHierarchy;

  test.beforeAll(async ({ browser }) => {
    // Set up test user with locations
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-locations-mobile-${id}@example.com`;
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
    // Set mobile viewport (iPhone-like)
    await page.setViewportSize({ width: 375, height: 667 });

    // Clear state and login fresh before each test
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);

    // Navigate to Locations tab
    await page.click('text="Locations"');
    await page.waitForTimeout(500);
  });

  test.describe('Layout Verification', () => {
    test('should NOT show split pane layout on mobile', async ({ page }) => {
      // Split pane should not exist on mobile
      await expect(page.locator('[data-testid="location-split-pane"]')).not.toBeVisible();
    });

    test('should NOT show tree panel on mobile', async ({ page }) => {
      // Tree panel is desktop only
      await expect(page.locator('[data-testid="location-tree-panel"]')).not.toBeVisible();
    });

    test('should show mobile view container', async ({ page }) => {
      // Mobile view should be visible
      await expect(page.locator('[data-testid="location-mobile-view"]')).toBeVisible();
    });

    test('should show search bar on mobile', async ({ page }) => {
      // Search input should be visible
      const searchInput = page.locator('input[placeholder*="Search"]');
      await expect(searchInput).toBeVisible();
    });
  });

  test.describe('Expandable Cards', () => {
    test('should show all root locations as collapsed cards', async ({ page }) => {
      // Both root locations should be visible as cards
      await expect(page.locator('text=warehouse-a')).toBeVisible();
      await expect(page.locator('text=warehouse-b')).toBeVisible();

      // Cards should be collapsed - floor details should NOT be visible
      await expect(page.locator('text=Floor 1')).not.toBeVisible();
    });

    test('should show identifier, name, and status in collapsed card', async ({ page }) => {
      // Each card should show identifier
      await expect(page.locator('text=warehouse-a')).toBeVisible();
      // Each card should show name
      await expect(page.locator('text=Warehouse A')).toBeVisible();
      // Each card should show status badge
      await expect(page.locator('text=Active').first()).toBeVisible();
    });

    test('should expand card on header tap', async ({ page }) => {
      // Initially, description should not be visible
      await expect(page.locator('text=Main warehouse facility')).not.toBeVisible();

      // Click on the card header to expand
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Now description should be visible
      await expect(page.locator('text=Main warehouse facility')).toBeVisible();
    });

    test('should show action buttons when expanded', async ({ page }) => {
      // Expand warehouse-a card
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Action buttons should be visible
      await expect(page.locator('button:has-text("Edit")')).toBeVisible();
      await expect(page.locator('button:has-text("Move")')).toBeVisible();
      await expect(page.locator('button:has-text("Delete")')).toBeVisible();
    });

    test('should collapse card on header tap when expanded', async ({ page }) => {
      // Expand the card
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Verify expanded (description visible)
      await expect(page.locator('text=Main warehouse facility')).toBeVisible();

      // Click again to collapse
      await cardHeader.click();

      // Description should be hidden
      await expect(page.locator('text=Main warehouse facility')).not.toBeVisible();
    });

    test('should show nested children when parent expanded', async ({ page }) => {
      // Expand warehouse-a card
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Children section should show floor-1 and floor-2
      await expect(page.locator('text=Children (2)')).toBeVisible();
      await expect(page.locator('text=floor-1')).toBeVisible();
      await expect(page.locator('text=floor-2')).toBeVisible();
    });

    test('should show Root Location type in expanded card', async ({ page }) => {
      // Expand warehouse-a card (root location)
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Should show Root Location type
      await expect(page.locator('text=Root Location')).toBeVisible();
    });

    test('should show Subsidiary type for child location', async ({ page }) => {
      // First expand warehouse-a to see children
      const warehouseCard = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await warehouseCard.click();

      // Now expand floor-1 card
      const floor1Card = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'floor-1' })
        .locator('button').first();
      await floor1Card.click();

      // Should show Subsidiary type
      await expect(page.locator('text=Subsidiary')).toBeVisible();
    });
  });

  test.describe('Search and Filter', () => {
    test('should filter cards when searching', async ({ page }) => {
      // Type in search
      const searchInput = page.locator('input[placeholder*="Search"]').first();
      await searchInput.fill('floor');

      // Wait for filter to apply
      await page.waitForTimeout(300);

      // warehouse-a should be visible (has matching descendants)
      await expect(page.locator('text=warehouse-a')).toBeVisible();

      // warehouse-b should NOT be visible (no match)
      await expect(page.locator('[data-testid="location-expandable-card"]').filter({ hasText: 'warehouse-b' })).not.toBeVisible();
    });

    test('should show no matches message when search has no results', async ({ page }) => {
      // Type non-matching search
      const searchInput = page.locator('input[placeholder*="Search"]').first();
      await searchInput.fill('nonexistent-location-xyz');

      // Wait for filter to apply
      await page.waitForTimeout(300);

      // Should show no matches message
      await expect(page.locator('text=No matching locations')).toBeVisible();
    });

    test('should show ancestor when descendant matches search', async ({ page }) => {
      // Search for a deeply nested location
      const searchInput = page.locator('input[placeholder*="Search"]').first();
      await searchInput.fill('section-a');

      // Wait for filter to apply
      await page.waitForTimeout(300);

      // warehouse-a should be visible (ancestor of section-a)
      await expect(page.locator('text=warehouse-a')).toBeVisible();
    });
  });

  test.describe('CRUD Operations', () => {
    test('should open edit modal from expanded card', async ({ page }) => {
      // Expand warehouse-a card
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Click Edit button
      await page.click('button:has-text("Edit")');

      // Edit modal should open
      await expect(page.locator('text=Edit Location')).toBeVisible({ timeout: 5000 });
    });

    test('should open move modal from expanded card', async ({ page }) => {
      // First expand warehouse-a to see children
      const warehouseCard = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await warehouseCard.click();

      // Now expand floor-1 card to access its Move button
      const floor1Card = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'floor-1' })
        .locator('button').first();
      await floor1Card.click();

      // Click Move button on floor-1
      const moveButton = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'floor-1' })
        .locator('button:has-text("Move")');
      await moveButton.click();

      // Move modal should open
      await expect(page.locator('text=Move Location')).toBeVisible({ timeout: 5000 });
    });

    test('should open delete confirmation from expanded card', async ({ page }) => {
      // Expand warehouse-a card
      const cardHeader = page.locator('[data-testid="location-expandable-card"]')
        .filter({ hasText: 'warehouse-a' })
        .locator('button').first();
      await cardHeader.click();

      // Click Delete button
      await page.click('button:has-text("Delete")');

      // Confirmation modal should open
      await expect(page.locator('text=Delete Location')).toBeVisible({ timeout: 5000 });
    });
  });
});

test.describe('Locations Mobile - Fresh State', () => {
  // These tests need fresh state with no existing locations

  let testEmail: string;
  let testPassword: string;

  test.beforeAll(async ({ browser }) => {
    // Set up test user without locations
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-empty-mobile-${id}@example.com`;
    testPassword = 'TestPassword123!';

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testEmail, testPassword);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Set mobile viewport
    await page.setViewportSize({ width: 375, height: 667 });

    // Clear state and login fresh
    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);

    // Navigate to Locations tab
    await page.click('text="Locations"');
    await page.waitForTimeout(500);
  });

  test('should show empty state when no locations', async ({ page }) => {
    // Should show empty state message
    await expect(page.locator('text=No locations yet')).toBeVisible();
    await expect(page.locator('text=Tap the + button')).toBeVisible();
  });

  test('should create new location via FAB and show as card', async ({ page }) => {
    // Click FAB to create location
    const fab = page.locator('button[aria-label="Create new location"]');
    await fab.click();

    // Fill in the form
    await expect(page.locator('text=Create Location')).toBeVisible({ timeout: 5000 });

    const identifierInput = page.locator('input#identifier');
    await identifierInput.fill('mobile-test-loc');

    const nameInput = page.locator('input#name');
    await nameInput.fill('Mobile Test Location');

    // Submit
    await page.locator('button[type="submit"]:has-text("Create")').click();

    // Wait for modal to close
    await expect(page.locator('text=Create Location')).not.toBeVisible({ timeout: 5000 });

    // Verify location appears as a card
    await expect(page.locator('text=mobile-test-loc')).toBeVisible();
    await expect(page.locator('text=Mobile Test Location')).toBeVisible();
  });

  test('should show FAB on mobile', async ({ page }) => {
    // FAB should be visible on mobile
    const fab = page.locator('button[aria-label="Create new location"]');
    await expect(fab).toBeVisible();
  });
});

test.describe('Locations Mobile - Tablet Viewport', () => {
  // Test tablet-sized viewport (still mobile layout but larger)

  let testEmail: string;
  let testPassword: string;

  test.beforeAll(async ({ browser }) => {
    const page = await browser.newPage();
    const id = uniqueId();
    testEmail = `test-tablet-${id}@example.com`;
    testPassword = 'TestPassword123!';

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await signupTestUser(page, testEmail, testPassword);
    await createTestHierarchy(page);

    await page.close();
  });

  test.beforeEach(async ({ page }) => {
    // Set tablet viewport (below 1024px threshold)
    await page.setViewportSize({ width: 768, height: 1024 });

    await page.goto('/');
    await clearAuthState(page);
    await page.reload({ waitUntil: 'networkidle' });
    await loginTestUser(page, testEmail, testPassword);

    await page.click('text="Locations"');
    await page.waitForTimeout(500);
  });

  test('should show mobile layout on tablet (below 1024px)', async ({ page }) => {
    // Should show mobile view, not split pane
    await expect(page.locator('[data-testid="location-mobile-view"]')).toBeVisible();
    await expect(page.locator('[data-testid="location-split-pane"]')).not.toBeVisible();
  });

  test('should switch to desktop layout at 1024px', async ({ page }) => {
    // Resize to desktop breakpoint
    await page.setViewportSize({ width: 1024, height: 768 });
    await page.waitForTimeout(300);

    // Should now show split pane
    await expect(page.locator('[data-testid="location-split-pane"]')).toBeVisible();
    await expect(page.locator('[data-testid="location-mobile-view"]')).not.toBeVisible();
  });
});
