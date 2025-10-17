/* eslint-disable @typescript-eslint/no-explicit-any */
/**
 * Helper to wait for reader state without hardcoding enum values
 * Imports ReaderState enum once and shares the values
 */

import type { Page } from '@playwright/test';
import { readFileSync } from 'fs';
import { join } from 'path';

// Parse the ReaderState const from reader.ts at build time
// This avoids TypeScript enum issues in strip-only mode
const constantsPath = join(process.cwd(), 'src/worker/types/reader.ts');
const constantsContent = readFileSync(constantsPath, 'utf-8');

// Extract ReaderState const values
const readerStateMatch = constantsContent.match(/export const ReaderState = \{[\s\S]*?\} as const;/)?.[0];
if (!readerStateMatch) throw new Error('Could not find ReaderState const');

// Build a map of const values
const READER_STATE: Record<string, string> = {};
const enumLines = readerStateMatch.split('\n').slice(1, -1);
enumLines.forEach(line => {
  const match = line.match(/\s*(\w+):\s*['"](\w+)['"]/);
  if (match) {
    READER_STATE[match[1]] = match[2];
  }
});

// Export the parsed enum values for use in tests
export const ReaderState = READER_STATE;

/**
 * Wait for the reader to reach IDLE state
 * Uses the imported ReaderState.IDLE value
 */
export async function waitForReaderIdle(page: Page, timeout: number = 10000) {
  console.log('[Helper] Waiting for reader to reach IDLE state...');
  
  const idleState = ReaderState.IDLE;
  await page.waitForFunction((expectedState) => {
    const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
    const readerState = deviceStore?.getState().readerState;
    
    // Log current state for debugging
    if (readerState !== undefined) {
      console.log(`[Test] Reader state: ${readerState}`);
    }
    
    return readerState === expectedState;
  }, idleState, { timeout });
  
  console.log('[Helper] Reader is IDLE');
}

/**
 * Wait for the reader to reach any of the specified states
 */
export async function waitForReaderState(
  page: Page, 
  expectedStates: string[], 
  timeout: number = 10000
): Promise<string> {
  console.log(`[Helper] Waiting for reader state: ${expectedStates.join(' or ')}`);
  
  const state = await page.waitForFunction((states) => {
    const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
    const readerState = deviceStore?.getState().readerState;
    
    if (readerState !== undefined) {
      console.log(`[Test] Reader state: ${readerState}`);
    }
    
    if (states.includes(readerState)) {
      return readerState;
    }
    return false;
  }, expectedStates, { timeout });
  
  const result = await state.jsonValue() as string;
  console.log(`[Helper] Reader reached state: ${result}`);
  return result;
}

/**
 * Get the current reader state
 */
export async function getCurrentReaderState(page: Page): Promise<string> {
  return await page.evaluate(() => {
    const deviceStore = window.__ZUSTAND_STORES__?.deviceStore;
    return deviceStore?.getState().readerState || 'UNKNOWN';
  });
}

/**
 * Check if reader is in one of the expected states
 */
export async function expectReaderState(
  page: Page,
  expectedStates: string | string[]
): Promise<boolean> {
  const states = Array.isArray(expectedStates) ? expectedStates : [expectedStates];
  const currentState = await getCurrentReaderState(page);
  return states.includes(currentState);
}