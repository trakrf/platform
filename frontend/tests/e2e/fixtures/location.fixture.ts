/**
 * Location Test Fixtures
 *
 * Reusable helpers for location-related E2E tests.
 * Provides utilities for creating test hierarchies via API.
 */

import type { Page } from '@playwright/test';
import { getAuthToken } from './org.fixture';

/**
 * Location creation data (public-API write shape).
 * Parent is referenced by natural key (`parent_external_key`) per the
 * 2026-04-29 canonical-key pivot — surrogate IDs are not required.
 */
export interface CreateLocationData {
  external_key: string;
  name: string;
  description?: string;
  parent_external_key?: string | null;
  is_active?: boolean;
}

/**
 * Trimmed view of the public LocationView response.
 * Includes both surrogate `id` (needed for DELETE) and natural
 * `external_key` so parent linkage can stay natural-key end-to-end.
 */
export interface CreatedLocation {
  id: number;
  external_key: string;
  name: string;
  description: string | null;
  parent_external_key: string | null;
  parent_id: number | null;
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
      external_key: data.external_key,
      name: data.name,
      description: data.description || '',
      parent_external_key: data.parent_external_key ?? null,
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
 * Delete a location via the public API (DELETE /locations/{location_id}).
 * The public DELETE route only accepts the surrogate `id`.
 */
export async function deleteLocationByIdViaAPI(
  page: Page,
  id: number
): Promise<void> {
  const baseUrl = getApiBaseUrl();
  const token = await getAuthToken(page);

  const response = await page.request.delete(`${baseUrl}/locations/${id}`, {
    headers: {
      Authorization: `Bearer ${token}`,
    },
  });

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
 * Children must be removed before parents to avoid 409 conflicts;
 * sort by parent_external_key presence (children first) as a coarse proxy
 * — for deeper hierarchies retry the loop until empty.
 */
export async function deleteAllLocationsViaAPI(page: Page): Promise<void> {
  const locations = await getLocationsViaAPI(page);

  const sortedChildrenFirst = [...locations].sort((a, b) => {
    const aHasParent = a.parent_external_key ? 1 : 0;
    const bHasParent = b.parent_external_key ? 1 : 0;
    return bHasParent - aHasParent;
  });

  for (const location of sortedChildrenFirst) {
    try {
      await deleteLocationByIdViaAPI(page, location.id);
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
 * Parent linkage uses parent_external_key (natural-key contract);
 * the parent's external_key is a string literal already in scope, so we
 * never need to read back a surrogate ID from a previous response.
 */
export async function createTestHierarchy(page: Page): Promise<TestHierarchy> {
  const warehouseA = await createLocationViaAPI(page, {
    external_key: 'warehouse-a',
    name: 'Warehouse A',
    description: 'Main warehouse facility',
  });

  const warehouseB = await createLocationViaAPI(page, {
    external_key: 'warehouse-b',
    name: 'Warehouse B',
    description: 'Secondary warehouse',
  });

  const floor1 = await createLocationViaAPI(page, {
    external_key: 'floor-1',
    name: 'Floor 1',
    description: 'First floor',
    parent_external_key: 'warehouse-a',
  });

  const floor2 = await createLocationViaAPI(page, {
    external_key: 'floor-2',
    name: 'Floor 2',
    description: 'Second floor',
    parent_external_key: 'warehouse-a',
  });

  const sectionA = await createLocationViaAPI(page, {
    external_key: 'section-a',
    name: 'Section A',
    description: 'Storage section A',
    parent_external_key: 'floor-1',
  });

  const sectionB = await createLocationViaAPI(page, {
    external_key: 'section-b',
    name: 'Section B',
    description: 'Storage section B',
    parent_external_key: 'floor-1',
  });

  const sectionC = await createLocationViaAPI(page, {
    external_key: 'section-c',
    name: 'Section C',
    description: 'Storage section C',
    parent_external_key: 'floor-2',
  });

  const storageArea = await createLocationViaAPI(page, {
    external_key: 'storage-area',
    name: 'Storage Area',
    description: 'General storage',
    parent_external_key: 'warehouse-b',
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
