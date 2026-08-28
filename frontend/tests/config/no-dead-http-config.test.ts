import { describe, it, expect } from 'vitest';
import { readFileSync } from 'fs';
import { execSync } from 'child_process';
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

/**
 * Files where 8080 legitimately appears as a bridge port — history, and the
 * rationale that explains the move.
 *
 * An allowlist, not a target list. The previous version of this guard named the
 * two documents it knew were broken, which is how there was a second time:
 * TRA-1179 fixed the code and `.env.local.example`, the guard asserted only
 * `.env.local.example`, and the docs drifted unwatched for a week. A guard
 * scoped to the files someone already found is a record of the last incident,
 * not a defence against the next one.
 */
const BRIDGE_PORT_HISTORY_OK = [
  // This file: the patterns below are literals in its own source.
  'frontend/tests/config/no-dead-http-config.test.ts',
  // Explains WHY the default moved off 8080; must keep saying 8080.
  'frontend/tests/config/resolve-bridge-port.ts',
  'frontend/tests/config/bridge-port-not-backend-port.test.ts',
];

/** Superseded designs, kept verbatim on purpose. */
const HISTORY_DIR = /(^|\/)(archive|CHANGELOG)/i;

/**
 * 8080 presented as THE BRIDGE's port, in any tracked text file.
 *
 * Deliberately narrow patterns rather than a bare `8080`. The platform backend
 * publishes on `0.0.0.0:8080` and is referenced correctly in docker-compose, the
 * root README, the Dockerfile, the justfile and every e2e fixture's
 * `PLAYWRIGHT_BASE_URL` fallback. TRA-1186's rule is *8080-as-bridge-port*, not
 * *8080* — a guard that cannot tell those apart gets weakened the first time it
 * fires on a correct line.
 */
const BRIDGE_WORD = /(bridge|BLE_MCP|ble-mcp)/i;
const BACKEND_WORD = /(backend|api|playwright|docker|healthz|readyz)/i;

/**
 * Does this line present 8080 as THE BRIDGE's port?
 *
 * Deliberately not a bare `8080`. The platform backend publishes on
 * `0.0.0.0:8080` and is named correctly in docker-compose, the root README, the
 * justfile and several e2e headers. TRA-1186's rule is *8080-as-bridge-port*,
 * not *8080* — and a guard that fires on a correct line is a guard the next
 * person deletes instead of reading.
 *
 * Attribution is positional: English hangs a port off the noun BEFORE it
 * ("backend on :8080", "bridge server on …:8080"), so only the text preceding
 * the number decides, and a backend word standing between the bridge word and
 * the number breaks the association. `kits.spec.ts` is the case that forces
 * this — "backend on :8080 + BLE bridge with a reader" is entirely correct, and
 * a symmetric proximity match flags it.
 */
function presentsBridgeOn8080(line: string): boolean {
  if (/BLE_MCP_WS_PORT\s*[=:]\s*['"]?8080/.test(line)) return true;
  // A ws:// scheme is always the bridge; the backend speaks http.
  if (/wss?:\/\/[^\s'"`]*:8080/.test(line)) return true;

  for (const m of line.matchAll(/\b8080\b/g)) {
    const before = line.slice(Math.max(0, m.index - 60), m.index);
    const bridge = [...before.matchAll(new RegExp(BRIDGE_WORD, 'gi'))].pop();
    if (!bridge) continue;
    // Anything after the bridge word that re-attributes the port.
    if (BACKEND_WORD.test(before.slice(bridge.index))) continue;
    return true;
  }
  return false;
}

/**
 * Binary and generated files, which cannot instruct anybody to do anything.
 *
 * An EXCLUSION list, deliberately, because the inclusion version of this filter
 * is the bug it is guarding against. The first widened pass listed the
 * extensions worth scanning — and that list silently omitted `justfile`,
 * `.envrc`, `Dockerfile` and `deploy/edge/quadlets/*.container`, none of which
 * have one. The bridge repo shipped the same filter and it skipped
 * `.env.local.example`: TRA-1186's own subject file.
 *
 * So the shape recurs at every layer. TRA-1179 scoped the fix to the files it
 * knew, #600 scoped the guard to the files it knew, and the first version of
 * this sweep scoped the *scan* to the extensions it knew. Unbounded scanning
 * plus a narrow exclusion cannot be partially applied; an allowlist always can.
 */
const NOT_TEXT = /\.(png|jpe?g|gif|svg|ico|webp|woff2?|ttf|eot|pdf|zip|gz|tgz|wasm|lock|sum|tsv|csv)$/i;

/** Every tracked file that could carry an instruction to a human. */
function trackedTextFiles(): string[] {
  const out = execSync('git ls-files -z', { cwd: REPO_ROOT, encoding: 'utf-8', maxBuffer: 32 * 1024 * 1024 });
  return out
    .split('\0')
    .filter(Boolean)
    .filter((f) => !NOT_TEXT.test(f))
    .filter((f) => !BRIDGE_PORT_HISTORY_OK.includes(f) && !HISTORY_DIR.test(f));
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

  /**
   * TRA-1186. The 8080 -> 25153 sweep fixed `.env.local.example` and the code,
   * and MISSED the docs — `docs/frontend/MOCK_USAGE_GUIDE.md` told readers to
   * set `BLE_MCP_WS_PORT=8080` in four places and to probe the bridge at
   * `http://localhost:8080/`, and `tests/e2e/README.md` said the same.
   *
   * That is worse than a stale doc. 8080 is the port the platform BACKEND
   * publishes on 0.0.0.0, so anyone following those instructions points the
   * bridge at the backend and gets a connection that succeeds against entirely
   * the wrong service. The guard above covers the example env file; nothing
   * covered the documents people actually read first.
   *
   * Docs, not just code, because the sweep proved the docs are where it hides.
   */
  it('no tracked file presents 8080 as the bridge port', () => {
    const offenders: string[] = [];

    for (const file of trackedTextFiles()) {
      let text: string;
      try {
        text = readFileSync(path.join(REPO_ROOT, file), 'utf-8');
      } catch {
        continue; // deleted-but-tracked, or binary
      }
      text.split('\n').forEach((line, i) => {
        if (presentsBridgeOn8080(line)) {
          offenders.push(`${file}:${i + 1}  ${line.trim().slice(0, 90)}`);
        }
      });
    }

    expect(offenders, `8080 presented as the bridge port:\n${offenders.join('\n')}`).toEqual([]);
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
