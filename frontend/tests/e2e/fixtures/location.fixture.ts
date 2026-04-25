/**
 * Location Test Fixtures
 *
 * Reusable helpers for location-related E2E tests.
 * Provides utilities for creating test hierarchies via API.
 */

import type { Page } from '@playwright/test';
import { getAuthToken } from './org.fixture';

/**
 * Location creation data (public-API write shape per TRA-447).
 * Parent is referenced by natural identifier, not surrogate ID.
 */
export interface CreateLocationData {
  identifier: string;
  name: string;
  description?: string;
  parent_identifier?: string | null;
  is_active?: boolean;
}

/**
 * Trimmed view of the public PublicLocationView response (TRA-447).
 * Surrogate IDs are intentionally omitted — fixtures use natural identifiers
 * end-to-end so e2e tests exercise the same contract external SDK consumers see.
 */
export interface CreatedLocation {
  identifier: string;
  name: string;
  description: string;
  parent: string | null;
  path: string;
  depth: number;
  is_active: boolean;
}

/**
 * Get the base API URL for E2E tests.
 * Honors PLAYWRIGHT_BASE_URL when running against a remote deployment
 * (preview, gke, staging, prod). Falls back to localhost:8080 for local runs.
 */
function getApiBaseUrl(): string {
  const base = process.env.PLAYWRIGHT_BASE_URL || 'http://localhost:8080';
  return `${base.replace(/\/$/, '')}/api/v1`;
}

/**
 * Create a location via API
 */
export async function createLocationViaAPI(
  page: Page,
  data: CreateLocationData
): Promise<CreatedLocation> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.post(`${baseUrl}/locations`, {
    headers: {
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
    },
    data: {
      identifier: data.identifier,
      name: data.name,
      description: data.description || '',
      parent_identifier: data.parent_identifier ?? null,
      is_active: data.is_active ?? true,
    },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to create location: ${response.status()} - ${text}`);
  }

  const result = await response.json();
  return result.data;
}

/**
 * Delete a location via the public API (DELETE /locations/{identifier}).
 * Uses the natural-key route so fixtures don't depend on internal surrogate IDs.
 */
export async function deleteLocationByIdentifierViaAPI(
  page: Page,
  identifier: string
): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.delete(
    `${baseUrl}/locations/${encodeURIComponent(identifier)}`,
    {
      headers: {
        Authorization: `Bearer ${token}`,
      },
    }
  );

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to delete location: ${response.status()} - ${text}`);
  }
}

/**
 * Get all locations via API
 */
export async function getLocationsViaAPI(page: Page): Promise<CreatedLocation[]> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.get(`${baseUrl}/locations?limit=1000`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

  if (!response.ok()) {
    const text = await response.text();
    throw new Error(`Failed to get locations: ${response.status()} - ${text}`);
  }

  const result = await response.json();
  return result.data ?? [];
}

/**
 * Delete all locations for clean test state.
 * Sorts by depth descending (deepest first) so children are deleted before
 * their parents, avoiding FK constraint failures.
 */
export async function deleteAllLocationsViaAPI(page: Page): Promise<void> {
  const locations = await getLocationsViaAPI(page);

  const sortedByDepth = [...locations].sort((a, b) => b.depth - a.depth);

  for (const location of sortedByDepth) {
    try {
      await deleteLocationByIdentifierViaAPI(page, location.identifier);
    } catch {
      // Cascade may have removed children already; teardown errors are
      // intentionally swallowed so they don't mask real test failures.
    }
  }
}

/**
 * Test hierarchy structure for consistent test setup
 *
 * Structure:
 * Warehouse A (root)
 * ├── Floor 1
 * │   ├── Section A
 * │   └── Section B
 * └── Floor 2
 *     └── Section C
 * Warehouse B (root)
 * └── Storage Area
 */
export interface TestHierarchy {
  warehouseA: CreatedLocation;
  floor1: CreatedLocation;
  sectionA: CreatedLocation;
  sectionB: CreatedLocation;
  floor2: CreatedLocation;
  sectionC: CreatedLocation;
  warehouseB: CreatedLocation;
  storageArea: CreatedLocation;
}

/**
 * Create a complete test hierarchy.
 * Parent linkage uses parent_identifier (TRA-447 natural-key contract);
 * the parent's identifier is a string literal already in scope, so we
 * never need to read back a surrogate ID from a previous response.
 */
export async function createTestHierarchy(page: Page): Promise<TestHierarchy> {
  const warehouseA = await createLocationViaAPI(page, {
    identifier: 'warehouse-a',
    name: 'Warehouse A',
    description: 'Main warehouse facility',
  });

  const warehouseB = await createLocationViaAPI(page, {
    identifier: 'warehouse-b',
    name: 'Warehouse B',
    description: 'Secondary warehouse',
  });

  const floor1 = await createLocationViaAPI(page, {
    identifier: 'floor-1',
    name: 'Floor 1',
    description: 'First floor',
    parent_identifier: 'warehouse-a',
  });

  const floor2 = await createLocationViaAPI(page, {
    identifier: 'floor-2',
    name: 'Floor 2',
    description: 'Second floor',
    parent_identifier: 'warehouse-a',
  });

  const sectionA = await createLocationViaAPI(page, {
    identifier: 'section-a',
    name: 'Section A',
    description: 'Storage section A',
    parent_identifier: 'floor-1',
  });

  const sectionB = await createLocationViaAPI(page, {
    identifier: 'section-b',
    name: 'Section B',
    description: 'Storage section B',
    parent_identifier: 'floor-1',
  });

  const sectionC = await createLocationViaAPI(page, {
    identifier: 'section-c',
    name: 'Section C',
    description: 'Storage section C',
    parent_identifier: 'floor-2',
  });

  const storageArea = await createLocationViaAPI(page, {
    identifier: 'storage-area',
    name: 'Storage Area',
    description: 'General storage',
    parent_identifier: 'warehouse-b',
  });

  return {
    warehouseA,
    floor1,
    sectionA,
    sectionB,
    floor2,
    sectionC,
    warehouseB,
    storageArea,
  };
}

/**
 * Navigate to the Locations tab
 */
export async function navigateToLocations(page: Page): Promise<void> {
  // Click on Locations in the navigation
  await page.click('text="Locations"');
  // Wait for locations to load
  await page.waitForTimeout(500);
}

/**
 * Wait for the split pane to be visible (desktop mode)
 */
export async function waitForSplitPane(page: Page): Promise<void> {
  // Wait for the split pane container
  await page.waitForSelector('[data-testid="location-split-pane"]', { timeout: 10000 });
}

/**
 * Get the tree panel element
 */
export function getTreePanel(page: Page) {
  return page.locator('[data-testid="location-tree-panel"]');
}

/**
 * Get the details panel element
 */
export function getDetailsPanel(page: Page) {
  return page.locator('[data-testid="location-details-panel"]');
}
