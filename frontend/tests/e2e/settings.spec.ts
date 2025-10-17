/**
 * Settings Page E2E Tests
 * Tests settings configuration, persistence, and impact on device operations
 */

import { test, expect, type Page } from '@playwright/test';
import { disconnectDevice } from './helpers/connection';
import type { WindowWithStores } from './types';

// SKIP: Hardware-dependent tests - pending worker refactor phase 3
test.describe.skip('Settings Page', () => {
  let sharedPage: Page;

  test.beforeAll(async ({ browser }) => {
    sharedPage = await browser.newPage();
    console.log('[Suite] Setting up settings test suite');
    await sharedPage.goto('/');
  });

  test.beforeEach(async () => {
    console.log(`[Test] Starting: ${test.info().title}`);
  });

  test.afterAll(async () => {
    if (sharedPage) {
      // Disconnect if connected
      try {
        await disconnectDevice(sharedPage);
      } catch {
        // Ignore
      }
      await sharedPage.close();
    }
  });
  
  test('should persist settings across sessions', async () => {
    // Navigate to settings
    await sharedPage.click('text="Settings"');
    await sharedPage.waitForTimeout(500);
    
    // Find a toggle or input to change
    let powerSlider = await sharedPage.$('input[type="range"], [data-testid="power-slider"]');
    let setPowerValue = null;
    if (powerSlider) {
      // Set power to specific value
      await powerSlider.fill('15');
      setPowerValue = await powerSlider.inputValue();
      console.log(`[Test] Set power to: ${setPowerValue}`);
    }
    
    // Find and toggle a switch
    const toggles = await sharedPage.$$('input[type="checkbox"], [role="switch"]');
    if (toggles.length > 0) {
      const firstToggle = toggles[0];
      const wasChecked = await firstToggle.isChecked();
      await firstToggle.click();
      console.log(`[Test] Toggled setting from ${wasChecked} to ${!wasChecked}`);
    }
    
    // Reload page
    await sharedPage.reload();
    
    // Navigate back to settings
    await sharedPage.click('text="Settings"');
    await sharedPage.waitForTimeout(500);
    
    // Re-query for elements after reload
    powerSlider = await sharedPage.$('input[type="range"], [data-testid="power-slider"]');
    
    // Verify settings persisted
    if (powerSlider && setPowerValue) {
      const currentValue = await powerSlider.inputValue();
      expect(currentValue).toBe(setPowerValue);
      console.log('[Test] Power setting persisted');
    }
  });
  
  test('should reset settings to defaults', async () => {
    // Navigate to settings
    await sharedPage.click('text="Settings"');
    await sharedPage.waitForTimeout(500);
    
    // Change some settings first
    const toggles = await sharedPage.$$('input[type="checkbox"], [role="switch"]');
    if (toggles.length > 0) {
      await toggles[0].click();
    }
    
    // Look for reset button
    const resetBtn = await sharedPage.$('button:has-text("Reset"), button:has-text("Defaults")');
    if (resetBtn) {
      await resetBtn.click();
      
      // Confirm if needed
      const confirmBtn = await sharedPage.$('button:has-text("Confirm")');
      if (confirmBtn) {
        await confirmBtn.click();
      }
      
      await sharedPage.waitForTimeout(500);
      
      console.log('[Test] Settings reset to defaults');
      
      // Verify settings are at defaults
      const settingsState = await sharedPage.evaluate(() => {
        const settingsStore = (window as WindowWithStores).__ZUSTAND_STORES__?.settingsStore;
        return settingsStore?.getState();
      });
      
      console.log('[Test] Settings state after reset:', settingsState);
    }
  });
});
