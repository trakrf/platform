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
 * The repo-root template, added TRA-1179.
 *
 * This guard was rooted at frontend/, so `.env.local.example` sat outside it —
 * and shipped `BLE_MCP_HTTP_PORT=8081` and `BLE_MCP_HTTP_TOKEN=` for two
 * tickets after TRA-1161 deleted both variables. The live config was clean and
 * the guard was green the whole time.
 *
 * A template is the worst place for dead config to survive, because it is not
 * merely stale — it is *copied*. Every fresh clone reconstructs whatever it
 * says, which is also how the 8080 collision would have propagated to the next
 * machine after we fixed this one.
 */
const REPO_ROOT = path.resolve(__dirname, '../../..');

const ROOT_FILES = ['.env.local.example'];

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
  it.each(ROOT_FILES)('%s does not seed BLE_MCP_HTTP_* into a fresh clone', (file) => {
    const code = codeOnly(readFileSync(path.join(REPO_ROOT, file), 'utf-8'));

    // Assignments only — the prose above them explains why they are gone and
    // has to name them, same reasoning as codeOnly() below.
    expect(code).not.toMatch(/^\s*BLE_MCP_HTTP_PORT\s*=/m);
    expect(code).not.toMatch(/^\s*BLE_MCP_HTTP_TOKEN\s*=/m);
  });

  it('.env.local.example does not seed the backend port for the bridge', () => {
    const code = codeOnly(readFileSync(path.join(REPO_ROOT, '.env.local.example'), 'utf-8'));

    expect(code).not.toMatch(/^\s*BLE_MCP_WS_PORT\s*=\s*8080\s*$/m);
  });

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
