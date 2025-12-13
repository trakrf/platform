import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';
import JSON5 from 'json5';
import type { Plugin } from 'vite';

// JSON5 Plugin to handle .json5 file imports
function json5Plugin(): Plugin {
  return {
    name: 'vite-plugin-json5',
    transform(code: string, id: string) {
      if (id.endsWith('.json5')) {
        try {
          const parsed = JSON5.parse(code);
          return {
            code: `export default ${JSON.stringify(parsed)}`,
            map: null
          };
        } catch (err) {
          this.error(`Failed to parse JSON5 file: ${id}\n${err}`);
        }
      }
    }
  };
}

export default defineConfig({
  plugins: [react(), json5Plugin()],
  test: {
    globals: true,
    environment: 'jsdom',
    // Disable parallel execution for ALL tests that communicate with hardware
    // Both integration and E2E tests access real CS108 hardware via bridge server
    // Running tests in parallel causes hardware conflicts and test failures
    pool: 'forks',
    poolOptions: {
      forks: {
        singleFork: true
      }
    },
    exclude: [
      '**/node_modules/**',
      '**/dist/**',
      '**/cypress/**',
      '**/.{idea,git,cache,output,temp}/**',
      '**/{karma,rollup,webpack,vite,vitest,jest,ava,babel,nyc,cypress,tsup,build}.config.*',
      '**/tests/e2e/**',  // Exclude E2E tests - they use Playwright
      '**/tests/e2e/to-fix/**',  // Exclude problematic tests in to-fix
      '**/lib/rfid/**/*.test.ts',  // Exclude lib/rfid tests as requested
      '**/lib/rfid/**/*.spec.ts',
      '**/examples/**',  // Exclude example files
      '**/tmp/**',  // Exclude tmp directory
      // TRA-192: Tests with incomplete store mocks - fix and remove from this list
      '**/src/components/assets/AssetCard.test.tsx',
      '**/src/components/assets/AssetForm.test.tsx',
      '**/src/components/assets/AssetFormModal.test.tsx',
      '**/src/components/assets/AssetSearchSort.test.tsx',
      '**/src/components/assets/AssetTable.test.tsx',
      '**/src/components/AssetsScreen.test.tsx',
      '**/src/components/inventory/InventoryTableRow.test.tsx',
      '**/src/components/__tests__/Header.test.tsx',
      '**/src/components/__tests__/InventoryScreen.test.tsx',
      '**/src/hooks/useScanToInput.test.ts',
      '**/src/lib/asset/transforms.test.ts',
    ],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@test-utils': path.resolve(__dirname, './test-utils')
    }
  }
});