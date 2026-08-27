/**
 * The single reader for BLE_MCP_WS_PORT (TRA-1179).
 *
 * This exists because the port was read in three places — ble-bridge.config.ts,
 * vite-bridge.config.ts and scripts/dev-bridge.js — each with its own
 * `|| '8080'` fallback. The first pass of the 8080 fix repaired two of them and
 * the guard listed the same two, so it went green while the third, the one
 * vite.config.ts actually calls, still defaulted onto the backend's port.
 *
 * Duplicated env reads are why a site gets missed. One reader means the next
 * change cannot be partially applied. dev-bridge.js keeps its own copy because
 * it is plain JS run outside the TS build, and the guard covers it.
 *
 * There is deliberately no default. The backend is published on 0.0.0.0:8080,
 * which subsumes loopback, so a bridge defaulting to 8080 could never run
 * alongside it — and the @hardware e2e suite needs both. A default here is not
 * a convenience, it is the defect: guessing a value the code cannot know is
 * what put the bridge on the backend's port in the first place. See ADR-0007.
 */

/**
 * Lowest non-privileged port. 1000-1023 are still privileged, so the bound is
 * binary 1024 rather than decimal 1000.
 */
const MIN_PORT = 1024;

/**
 * First ephemeral port. The kernel draws source ports for outbound connections
 * from 32768 upward (32768-60999 on this host), so a listener at or above this
 * collides rarely, non-deterministically, and miserably.
 *
 * Consequence worth knowing: any "give me a free port" idiom — bind to port 0,
 * `server.listen(0)`, `get-port` — is answered *from* that range, so it returns
 * exactly the ports this rejects. Probe inside 1024-32767 instead.
 */
const MAX_PORT = 32767;

export function resolveBridgePort(env: NodeJS.ProcessEnv = process.env): string {
  const wsPort = env.BLE_MCP_WS_PORT;

  if (!wsPort) {
    throw new Error(
      'BLE_MCP_WS_PORT is not set. There is no safe default: the bridge once ' +
        'defaulted to 8080, which is the port the platform backend publishes on ' +
        '0.0.0.0 — so the two could never run together, and the @hardware e2e ' +
        'suite (which needs both) could not pass on this host regardless of what ' +
        'it asserted. Set it explicitly in .env.local. See TRA-1179.'
    );
  }

  const portNumber = Number(wsPort);
  if (!Number.isInteger(portNumber) || portNumber < MIN_PORT || portNumber > MAX_PORT) {
    throw new Error(
      `BLE_MCP_WS_PORT must be an integer in ${MIN_PORT}-${MAX_PORT}, got ` +
        `"${wsPort}". Below ${MIN_PORT} is privileged; ${MAX_PORT + 1} and above ` +
        'is the ephemeral range. See TRA-1179.'
    );
  }

  return wsPort;
}
