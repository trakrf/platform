import { describe, it, expect, afterEach, vi } from 'vitest';
import { readFileSync } from 'fs';
import { resolve } from 'path';

/**
 * TRA-1179 — the bridge must not silently land on the backend's port.
 *
 * The platform backend is published by docker on `0.0.0.0:8080`, which subsumes
 * loopback. The BLE bridge binds `127.0.0.1:8080`. They cannot both run, and the
 * `@hardware` e2e specs need BOTH — the backend to authenticate, the bridge to
 * reach the CS108. So that suite could not have passed on this host no matter
 * what any assertion in it said.
 *
 * Nobody hit it for months because the bridge used to live on another machine
 * (knuckles, 192.168.50.14). Two hosts meant two independent 8080s. When the
 * ESPHome backend removed the local-radio dependency and the bridge co-located
 * onto this host, it landed on a port the backend already owned. The config was
 * correct right up until a component moved, and moving it announced nothing.
 *
 * Verified 2026-08-26 by direct experiment, not inference:
 *   OSError: [Errno 98] error while attempting to bind on address
 *   ('127.0.0.1', 8080): address already in use
 *
 * The failure is invisible from either end. Backend first: the bridge dies at
 * startup on a bind error that reads as an environment fault. Bridge first: the
 * e2e specs hang at LOGIN and never reach BLE, which reads as auth or fixture
 * rot. Neither presentation names the port.
 */

const BACKEND_PORT = '8080';

const SOURCES = [
  'scripts/dev-bridge.js',
  'tests/config/ble-bridge.config.ts'
];

describe('bridge port must never default to the backend port', () => {
  afterEach(() => {
    vi.unstubAllEnvs();
  });

  /**
   * A default is the whole hazard here, not a convenience.
   *
   * `process.env.BLE_MCP_WS_PORT || '8080'` reconstructs the collision the
   * moment the env is unset — and it does it silently, producing a config that
   * looks deliberate. That is the shape ADR-0007 rejects: a missing value
   * substituted with a plausible fake rather than reported as an error.
   *
   * There is no correct default for a bridge port. Guessing one is what put us
   * on the backend's port in the first place.
   */
  it.each(SOURCES)('%s does not fall back to the backend port', src => {
    const text = readFileSync(resolve(__dirname, '..', '..', src), 'utf8');

    // Strip comments before scanning — a guard that trips on its own
    // explanatory prose is a guard nobody keeps (learned on TRA-1177's
    // no-dead-http-config guard, which matched the comment naming the very
    // thing it forbade).
    const code = text
      .replace(/\/\*[\s\S]*?\*\//g, '')
      .replace(/^\s*\/\/.*$/gm, '');

    expect(code).not.toContain(`|| '${BACKEND_PORT}'`);
    expect(code).not.toContain(`|| "${BACKEND_PORT}"`);
  });

  it('getBleBridgeConfig throws when BLE_MCP_WS_PORT is unset', async () => {
    vi.stubEnv('BLE_MCP_WS_PORT', '');

    const { getBleBridgeConfig } = await import('./ble-bridge.config');

    expect(() => getBleBridgeConfig()).toThrow(/BLE_MCP_WS_PORT/);
  });

  it('getBleBridgeConfig uses the configured port when set', async () => {
    vi.stubEnv('BLE_MCP_WS_PORT', '15104');

    const { getBleBridgeConfig } = await import('./ble-bridge.config');

    // Assert the exact value, not merely "not 8080" — a negative check passes
    // against any wrong-but-different port.
    expect(getBleBridgeConfig().bridge.wsPort).toBe('15104');
    expect(getBleBridgeConfig().bridge.wsUrl).toContain(':15104');
  });
});
