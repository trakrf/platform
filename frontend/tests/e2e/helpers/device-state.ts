/**
 * Device state helper functions for E2E tests
 * Provides a single source of truth for accessing device state and comparing against constants
 */

import type { Page } from '@playwright/test';
import type { WindowWithStores } from '../types';
import * as fs from 'fs';
import * as path from 'path';

// Dynamically load ReaderState const from source to maintain single source of truth
const constantsPath = path.resolve(process.cwd(), 'src/worker/types/reader.ts');
const constantsContent = fs.readFileSync(constantsPath, 'utf-8');
const readerStateMatch = constantsContent.match(/export const ReaderState = \{[\s\S]*?\} as const/)?.[0];
if (!readerStateMatch) throw new Error('Could not find ReaderState const');

// Extract the const values as an object
const ReaderState: Record<string, string> = {};
const constLines = readerStateMatch.split('\n').slice(1, -1); // Skip first and last lines
constLines.forEach(line => {
  const match = line.match(/\s*(\w+):\s*['"]([^'"]+)['"]/);
  if (match) {
    ReaderState[match[1]] = match[2];
  }
});

/**
 * Get the current reader state from the device store
 * Returns the actual ReaderState enum value that can be compared against constants
 */
export async function getReaderState(page: Page): Promise<string> {
  const state = await page.evaluate(() => {
    const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
    return deviceStore?.getState().readerState;
  });
  return state as string;
}

/**
 * Check if the reader is in a specific state
 */
export async function isReaderInState(page: Page, expectedState: string): Promise<boolean> {
  const currentState = await getReaderState(page);
  return currentState === expectedState;
}

/**
 * Wait for the reader to reach a specific state
 */
export async function waitForReaderState(
  page: Page, 
  expectedState: string, 
  timeout: number = 5000
): Promise<boolean> {
  const startTime = Date.now();
  
  while (Date.now() - startTime < timeout) {
    const currentState = await getReaderState(page);
    if (currentState === expectedState) {
      return true;
    }
    await page.waitForTimeout(100);
  }
  
  return false;
}

/**
 * Get the trigger state from the device store
 */
export async function getTriggerState(page: Page): Promise<boolean> {
  return await page.evaluate(() => {
    const deviceStore = (window as WindowWithStores).__ZUSTAND_STORES__?.deviceStore;
    return deviceStore?.getState().triggerState || false;
  });
}

/**
 * Check if inventory is running
 */
export async function isInventoryRunning(page: Page): Promise<boolean> {
  return await page.evaluate(() => {
    const tagStore = (window as WindowWithStores).__ZUSTAND_STORES__?.tagStore;
    return tagStore?.getState().inventoryRunning || false;
  });
}

/**
 * Get the current active tab
 */
export async function getActiveTab(page: Page): Promise<string> {
  return await page.evaluate(() => {
    const uiStore = (window as WindowWithStores).__ZUSTAND_STORES__?.uiStore;
    return uiStore?.getState().activeTab || 'home';
  });
}

/**
 * Get complete device state for debugging
 */
export async function getDeviceDebugState(page: Page): Promise<{
  readerState: string;
  triggerState: boolean;
  inventoryRunning: boolean;
  activeTab: string;
  isConnected: boolean;
}> {
  return await page.evaluate(() => {
    const stores = (window as WindowWithStores).__ZUSTAND_STORES__;
    const deviceState = stores?.deviceStore?.getState();
    const tagState = stores?.tagStore?.getState();
    const uiState = stores?.uiStore?.getState();
    
    return {
      readerState: deviceState?.readerState || 0,
      triggerState: deviceState?.triggerState || false,
      inventoryRunning: tagState?.inventoryRunning || false,
      activeTab: uiState?.activeTab || 'home',
      isConnected: deviceState?.isConnected || false
    };
  });
}

// Export the dynamically loaded ReaderState for use in tests
export { ReaderState };