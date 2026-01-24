/**
 * Location Test Fixtures
 *
 * Reusable helpers for location-related E2E tests.
 * Provides utilities for creating test hierarchies via API.
 */

import type { Page } from '@playwright/test';
import { getAuthToken } from './org.fixture';

/**
 * Location creation data
 */
export interface CreateLocationData {
  identifier: string;
  name: string;
  description?: string;
  parent_location_id?: number | null;
  is_active?: boolean;
}

/**
 * Created location response
 */
export interface CreatedLocation {
  id: number;
  identifier: string;
  name: string;
  description: string;
  parent_location_id: number | null;
  is_active: boolean;
}

/**
 * Get the base API URL for E2E tests
 */
function getApiBaseUrl(): string {
  return 'http://localhost:8080/api/v1';
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
      parent_location_id: data.parent_location_id ?? null,
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
 * Delete a location via API
 */
export async function deleteLocationViaAPI(page: Page, id: number): Promise<void> {
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
 * Delete all locations for clean test state
 * Deletes in reverse order (children first) to avoid parent constraint issues
 */
export async function deleteAllLocationsViaAPI(page: Page): Promise<void> {
  const locations = await getLocationsViaAPI(page);

  // Sort by depth (deepest first) to delete children before parents
  // Locations without parent_location_id are root (depth 0)
  const sortedByDepth = locations.sort((a, b) => {
    const depthA = a.parent_location_id ? 1 : 0;
    const depthB = b.parent_location_id ? 1 : 0;
    return depthB - depthA;
  });

  for (const location of sortedByDepth) {
    try {
      await deleteLocationViaAPI(page, location.id);
    } catch {
      // Ignore errors (may already be deleted via cascade)
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
 * Create a complete test hierarchy
 * Returns all created locations for use in tests
 */
export async function createTestHierarchy(page: Page): Promise<TestHierarchy> {
  // Create root locations
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

  // Create Floor 1 and Floor 2 under Warehouse A
  const floor1 = await createLocationViaAPI(page, {
    identifier: 'floor-1',
    name: 'Floor 1',
    description: 'First floor',
    parent_location_id: warehouseA.id,
  });

  const floor2 = await createLocationViaAPI(page, {
    identifier: 'floor-2',
    name: 'Floor 2',
    description: 'Second floor',
    parent_location_id: warehouseA.id,
  });

  // Create sections under floors
  const sectionA = await createLocationViaAPI(page, {
    identifier: 'section-a',
    name: 'Section A',
    description: 'Storage section A',
    parent_location_id: floor1.id,
  });

  const sectionB = await createLocationViaAPI(page, {
    identifier: 'section-b',
    name: 'Section B',
    description: 'Storage section B',
    parent_location_id: floor1.id,
  });

  const sectionC = await createLocationViaAPI(page, {
    identifier: 'section-c',
    name: 'Section C',
    description: 'Storage section C',
    parent_location_id: floor2.id,
  });

  // Create storage area under Warehouse B
  const storageArea = await createLocationViaAPI(page, {
    identifier: 'storage-area',
    name: 'Storage Area',
    description: 'General storage',
    parent_location_id: warehouseB.id,
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
