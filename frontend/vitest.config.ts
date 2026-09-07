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
    // KEEP `singleFork`. The reason is CI WALL-CLOCK, not hardware (TRA-1095).
    //
    // This comment used to say "disable parallel execution for ALL tests that
    // communicate with hardware — both integration and E2E tests access real
    // CS108 hardware via bridge server". Every clause of that was false, and it
    // steered the diagnosis on four tickets (TRA-1050, TRA-1052, TRA-1079,
    // TRA-1093), each of which read it and treated the constraint as
    // load-bearing:
    //
    //   - E2E tests are excluded by THIS FILE, a few lines below
    //     ('**/tests/e2e/**' — "they use Playwright").
    //   - Integration tests are not in this run either. `pnpm test` is
    //     `vitest run src/ tests/config/` (package.json), so nothing under
    //     tests/integration/ is reached.
    //   - `test:integration` self-serializes with its own
    //     `--no-file-parallelism` on the command line, so the hardware suite
    //     does not depend on this setting at all.
    //
    // So this serializes ~166 pure unit files for a reason that applies to none
    // of them. It still must not be deleted:
    //
    //   TRA-1093 measured per-file isolation at 11.9s vs 16.1s on a 24-core dev
    //   box (faster), but **87s vs 15s on a 2-core CI-sized runner** — a 5.7x
    //   regression. It also makes lazy-route resolution slower (cold module
    //   graph per fork, 66ms vs 18ms), and needs
    //   `import '@testing-library/jest-dom'` added to the shared setup, since 9
    //   files currently free-ride on another file's import.
    //
    // The numbers are recorded here so the next person does not re-derive them.
    // Whether `pool`/`poolOptions` should live in a config the unit run and the
    // integration run both read is a separate question — TRA-1196.
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