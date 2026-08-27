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
 * The default is 25153, and the rule is that it must never be a port a
 * co-resident service owns. The original defect was not defaultness — it was
 * defaulting to 8080, which the platform backend publishes on 0.0.0.0, so the
 * bridge could never run alongside it and the @hardware e2e suite (which needs
 * both) could not pass on this host.
 *
 * 25153 is chosen to be unowned: clear of the alternate-HTTP clusters, clear of
 * ESPHome's 6053, below the ephemeral range, and — unlike the 15104 first
 * picked — carrying no IDS reputation. See TRA-1179.
 *
 * The residual cost of having a default at all, stated so nobody has to
 * rediscover it: an asymmetrically configured pair now fails as a connection
 * error rather than a config error. Set one side to a custom port and forget
 * the other, and you get "cannot connect" instead of "you did not set this."
 */

/**
 * Lowest non-privileged port. 1000-1023 are still privileged, so the bound is
 * binary 1024 rather than decimal 1000.
 */
const MIN_PORT = 1024;

/**
 * The bridge's conventional port. Must never be one a co-resident service
 * owns — that, not the existence of a default, was the 8080 defect.
 */
const DEFAULT_PORT = '25153';

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
  const wsPort = env.BLE_MCP_WS_PORT || DEFAULT_PORT;

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
