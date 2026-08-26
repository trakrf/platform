import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import path from 'path';
import { getBleBridgeConfig } from './ble-bridge.config';

/**
 * TRA-1177 row H. TRA-1161 deleted ble-mcp-test's HTTP/MCP surface outright —
 * no port 8081, no Express, no token, no `origin: '*'`. The readers here
 * outlived the variables: `process.env.BLE_MCP_HTTP_PORT || '8081'` against a
 * variable that exists nowhere means the fallback *always* fires, producing a
 * confident URL for a port nothing will ever serve.
 *
 * That was not merely cosmetic. dev-bridge.js gated startup on fetching
 * http://host:8081/health and exiting 1 when it failed — which it always did —
 * so `pnpm dev:bridge` could not start at all.
 *
 * This guard exists because the broken state looked correct for weeks. A
 * literal :8081 beside a plausible variable name is not something anyone
 * catches by reading.
 */

const FRONTEND_ROOT = path.resolve(__dirname, '../..');

const FILES = [
  'tests/config/ble-bridge.config.ts',
  'tests/e2e/e2e.config.ts',
  'scripts/dev-bridge.js',
];

/**
 * Strip comments before scanning.
 *
 * The guard is about code, not prose: these files carry comments explaining
 * *why* the variables are gone, and those necessarily name them. A guard that
 * forbade the names outright would pressure the next person to delete the
 * explanation in order to get their build green, which is the opposite of what
 * this is for.
 */
function codeOnly(source: string): string {
  return source
    .replace(/\/\*[\s\S]*?\*\//g, '')
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

describe('the deleted ble-mcp-test HTTP surface', () => {
  it.each(FILES)('%s does not read BLE_MCP_HTTP_* or hardcode :8081', (file) => {
    const code = codeOnly(readFileSync(path.join(FRONTEND_ROOT, file), 'utf-8'));

    expect(code).not.toMatch(/BLE_MCP_HTTP_PORT/);
    expect(code).not.toMatch(/BLE_MCP_HTTP_TOKEN/);
    expect(code).not.toMatch(/8081/);
  });

  it('does not expose an http url, port or token on the bridge config', () => {
    const bridge = getBleBridgeConfig().bridge as Record<string, unknown>;

    expect(bridge.httpUrl).toBeUndefined();
    expect(bridge.httpPort).toBeUndefined();
    expect(bridge.token).toBeUndefined();
  });
});
