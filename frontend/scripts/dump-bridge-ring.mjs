/**
 * Dump the bridge's in-memory packet ring to JSONL, so it can be analysed and archived.
 *
 * `ring-unanswered-commands.mjs` takes a ring dump as its input and nothing in
 * this repo produced one — the runbook described the pagination protocol in prose
 * and expected it hand-driven, which at ~430k records per 200-rep arm nobody was
 * ever going to do. So the analysis that answers "did the retry land?" simply did
 * not get run at arm end. This is the missing half. Refs TRA-1242.
 *
 * ⚠ The ring lives in the bridge process's memory and dies with it. Capacity is
 * not the risk — 1,000,000 records is ~2.4 arms of headroom, sized deliberately —
 * PROCESS LIFETIME is. Dump before anything restarts the bridge.
 *
 * Usage:
 *   node scripts/dump-bridge-ring.mjs <out.jsonl> [--since <ISO8601>]
 *
 * `--since` bisects the cursor space rather than fetching and discarding, which
 * halves the round trips when the ring holds more than the arm you want.
 */

import { createWriteStream } from 'node:fs';
import net from 'node:net';
import { controlSocketPath } from './watch-soak-abort-criteria.mjs';

/** The bridge REFUSES a larger page rather than clamping, so this is a limit not a preference. */
export const PAGE = 1000;

/** One request, one newline-delimited reply. Rejects on anything but ok:true. */
export function callControlWithArgs(op, args = {}, { socketPath = controlSocketPath(), timeoutMs = 15000 } = {}) {
  return new Promise((resolve, reject) => {
    let buf = '';
    const sock = net.connect(socketPath);
    sock.setTimeout(timeoutMs);
    sock.on('connect', () => sock.write(`${JSON.stringify({ op, args })}\n`));
    sock.on('data', (chunk) => {
      buf += chunk;
      const nl = buf.indexOf('\n');
      if (nl === -1) return;
      sock.destroy();
      let reply;
      try {
        reply = JSON.parse(buf.slice(0, nl));
      } catch (e) {
        return reject(new Error(`unparseable reply: ${e.message}`));
      }
      // Arguments must be nested under `args`. The bridge USED to ignore a
      // top-level cursor and hand back a plausible next_cursor that never
      // advanced; a client looping on that once wrote 27 GB of page 1. It now
      // refuses with a reason instead (verified 2026-09-02), so surface the
      // reason — a failed call must never read as an empty ring.
      if (!reply.ok) return reject(new Error(reply.reason ?? 'control call failed'));
      resolve(reply.result);
    });
    sock.on('timeout', () => { sock.destroy(); reject(new Error('control socket timeout')); });
    sock.on('error', reject);
  });
}

/** Lowest cursor whose next entry is at or after `iso`, by bisection. */
export async function seekTo(call, iso, lo, hi) {
  const target = Date.parse(iso);
  while (lo < hi) {
    const mid = Math.floor((lo + hi) / 2);
    const { entries } = await call('read_stream', { cursor: mid, limit: 1 });
    if (!entries.length || Date.parse(entries[0].timestamp) >= target) hi = mid;
    else lo = mid + 1;
  }
  return lo;
}

/**
 * Page the ring into `write`, returning how many records were written.
 *
 * `call` and `write` are injected so the two SILENT-FAILURE paths below can be
 * tested without a bridge. Both of them produce a file that looks fine:
 *
 *   - a disabled buffer yields an empty dump, which downstream reads as a clean run
 *   - a stalled cursor loops forever writing the same page
 *
 * Neither announces itself, which is the whole reason they are guarded rather
 * than assumed.
 */
export async function dumpRing({ call, write, since = null, log = () => {} }) {
  const head = await call('read_stream', { limit: 1 });
  if (!head.buffer_enabled) {
    throw new Error('BLE_MCP_LOG_BUFFER_SIZE=0 — the bridge is recording nothing. Refusing to write an empty dump.');
  }
  const first = head.entries[0]?.id;
  if (first === undefined) throw new Error('ring is empty');

  let cursor = first - 1;
  if (since) {
    const probe = await call('read_stream', { cursor: first - 1, limit: PAGE });
    cursor = await seekTo(call, since, first - 1, Math.max(first - 1, probe.next_cursor + 1_000_000));
    log(`--since ${since} -> starting at cursor ${cursor}`);
  }

  let n = 0;
  let truncated = false;
  for (;;) {
    const page = await call('read_stream', { cursor, limit: PAGE });
    if (page.dropped_before != null) {
      truncated = true;
      log(`⚠ ring evicted entries before ${page.dropped_before} while dumping — the record is INCOMPLETE`);
    }
    if (!page.entries.length) break;
    for (const e of page.entries) write(`${JSON.stringify(e)}\n`);
    n += page.entries.length;
    // Guard on the CURSOR ADVANCING, not on a short page. A short page is
    // normal; a stalled cursor is the bug, and it is the one that spins forever.
    if (!(page.next_cursor > cursor)) {
      throw new Error(`cursor stalled at ${cursor} after ${n} records — aborting rather than looping`);
    }
    cursor = page.next_cursor;
    if (n % 50_000 === 0) log(`  ${n} records...`);
  }
  return { records: n, truncated };
}

async function main() {
  const [out, ...rest] = process.argv.slice(2);
  if (!out) {
    console.error('usage: dump-bridge-ring.mjs <out.jsonl> [--since <ISO8601>]');
    process.exit(2);
  }
  const since = rest.includes('--since') ? rest[rest.indexOf('--since') + 1] : null;
  const sink = createWriteStream(out);
  try {
    const { records, truncated } = await dumpRing({
      call: callControlWithArgs,
      write: (line) => sink.write(line),
      since,
      log: (m) => console.error(m),
    });
    await new Promise((r) => sink.end(r));
    console.error(`wrote ${records} records to ${out}${truncated ? ' (INCOMPLETE — see warning above)' : ''}`);
    if (truncated) process.exit(1);
  } catch (e) {
    await new Promise((r) => sink.end(r));
    console.error(String(e.message ?? e));
    process.exit(1);
  }
}

if (process.argv[1] && import.meta.url === `file://${process.argv[1]}`) {
  await main();
}
