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
    // Keep the suite hermetic — see test-utils/vitest.setup.ts (TRA-1050).
    setupFiles: ['./test-utils/vitest.setup.ts'],
    // Vitest inherits the developer's shell, and dev machines export
    // VITE_API_URL pointing at a live backend. Pin it so `src/lib/api/client.ts`
    // can never pick that up, and so local runs match CI.
    env: {
      VITE_API_URL: 'http://127.0.0.1:9/api/v1',
    },
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
      '**/src/components/assets/AssetSearchSort.test.tsx',
      '**/src/components/assets/AssetTable.test.tsx',
      '**/src/components/AssetsScreen.test.tsx',
      '**/src/components/inventory/InventoryTableRow.test.tsx',
      '**/src/components/__tests__/Header.test.tsx',
      '**/src/components/__tests__/InventoryScreen.test.tsx',
      '**/src/hooks/useScanToInput.test.ts',
      '**/src/lib/asset/transforms.test.ts',
      // TRA-1215: tests/data/ was never missing. It existed all along and was
      // swallowed by a bare `data/` in .gitignore, so the two specs excluded
      // here ran nowhere while their exclusion read as a known, benign gap.
      //
      // parser.test.ts is re-enabled and passes. handler.test.ts is NOT, and
      // the reason is now a measurement rather than an assumption: with its
      // fixtures present it runs 21 tests, 19 pass, and 2 fail for a real
      // reason unrelated to test data —
      //
      //   canHandle > accepts 0x8100 packets in LOCATE mode
      //   handle    > emits LOCATE_UPDATE events in LOCATE mode
      //
      // The handler emits TAG_READ where both expect LOCATE_UPDATE. That is
      // either a stale spec or a live defect on the Locate path, and deciding
      // which is its own ticket, not a guess folded into a provenance PR.
      '**/src/worker/cs108/rfid/inventory/handler.test.ts',
    ],
  },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@test-utils': path.resolve(__dirname, './test-utils')
    }
  }
});