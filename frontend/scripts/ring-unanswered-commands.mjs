/**
 * Which `0x8002` commands did the reader not answer, and did the retry land?
 *
 * The soak instrument counts `[CommandManager] Command timeout: <op>` lines, and
 * that line fires **per attempt**. `RFID_FIRMWARE_COMMAND` is also the single op
 * code the CS108 uses for every RFID firmware command, so a count against it
 * answers neither "which command" nor "did it ultimately fail". On the
 * 2026-09-01 200-rep arm it read 47, and those 47 turned out to be 45 ABORTs
 * that the retry recovered plus 2 register writes — a picture the log alone
 * cannot produce.
 *
 * This reads the bridge's packet ring instead, where the payload names the
 * command and the reply is either there or it is not.
 *
 * ## The two payload shapes behind one op code
 *
 * ```
 *   70 <rw> <regLSB> <regMSB> <val0..3>   register read/write  (spec A.3, LSB first)
 *   40 03 00 00 00 00 00 00               ABORT                (spec A.8)
 * ```
 *
 * ⚠ Only frames that START with the `A7 B3` prefix are considered. At mtu=23 the
 * RX stream is mostly continuation fragments, and bytes 8..9 of a fragment
 * decode to a plausible wrong event code — which is the shape that yields a
 * confident wrong answer rather than an error.
 *
 * ## Why the retry question needs its own pass
 *
 * A retry cannot be found by grouping transmissions that happen to be close
 * together. The first version of this did exactly that, with a 3s window, and
 * reported that 241 stops needed a retry — but 194 of its inter-attempt gaps
 * were >=1600ms, which the schedule ([100,200,500,1000] behind a 200ms timeout)
 * cannot produce. It was counting two SEPARATE stops as one retried stop.
 *
 * So `retryOutcome` anchors on the mechanism: decide per transmission whether it
 * was answered, then ask, for each unanswered one, whether the NEXT transmission
 * of the same op was answered. No proximity heuristic, nothing to tune.
 *
 * Usage:
 *   node scripts/ring-unanswered-commands.mjs <ring.jsonl> [fromISO] [toISO]
 *
 * Window the run to the arm. A ring routinely spans more than the arm that
 * dumped it — the 2026-09-01 ring starts nine hours before rep 1, and counting
 * the whole file reports 90 unanswered where the arm's own window holds 47.
 */

import fs from 'node:fs';
import readline from 'node:readline';

/** Op code shared by every RFID firmware command. */
export const RFID_FIRMWARE_OP = 0x8002;
/** A refusal arrives under this code, never under the op it refuses (TRA-1229). */
export const ERROR_NOTIFICATION_OP = 0xA101;

/** The command budget a reply has to beat. */
export const DEFAULT_BUDGET_MS = 2500;
/** Whole retry schedule: four timeouts plus 100+200+500+1000ms of delays. */
export const DEFAULT_RETRY_WINDOW_MS = 2600;

/**
 * Turn one ring line into a frame, or `null` if it is not a whole packet.
 *
 * Returns `null` rather than throwing for anything unparseable: a ring is an
 * append-only capture and a truncated final line is normal, not a fault.
 */
export function parseFrame(line) {
  // NO substring prefilter on `"is_packet": true`.
  //
  // The first version had one, for speed over ~870k lines, and it keyed on the
  // producer's exact whitespace. The bridge writes Python's `json.dumps`
  // spacing; anything else — a re-serialised ring, a fixture built with
  // `JSON.stringify` — matches nothing and the tool reports zero frames, which
  // reads as "no packets in the window" rather than as a parse failure. A
  // filter that can only produce false negatives is worse than the parse it
  // saves. Caught by the test fixture, which is exactly the input shape it got
  // wrong.
  let record;
  try {
    record = JSON.parse(line);
  } catch {
    return null;
  }
  if (!record.is_packet || typeof record.text !== 'string') return null;
  if (!record.text.startsWith('A7 B3')) return null;
  const bytes = record.text.split(' ').map((h) => parseInt(h, 16));
  if (bytes.length < 10 || bytes.some(Number.isNaN)) return null;
  return {
    t: Date.parse(record.timestamp),
    dir: record.direction,
    op: (bytes[8] << 8) | bytes[9],
    bytes
  };
}

/**
 * Name the command a `0x8002` payload actually carries.
 *
 * A register address is reported as `W 0xNNNN` / `R 0xNNNN` because that is how
 * the sequences name it; ABORT is reported by name because it has no register.
 */
export function classifyFirmwarePayload(bytes) {
  if (bytes.length >= 18 && bytes[10] === 0x70) {
    const register = (bytes[13] << 8) | bytes[12];
    const rw = bytes[11] === 0x01 ? 'W' : 'R';
    return {
      kind: bytes[11] === 0x01 ? 'WRITE' : 'READ',
      register,
      label: `${rw} 0x${register.toString(16).padStart(4, '0').toUpperCase()}`
    };
  }
  if (bytes.length >= 12 && bytes[10] === 0x40 && bytes[11] === 0x03) {
    return { kind: 'ABORT', register: null, label: 'ABORT' };
  }
  return {
    kind: 'OTHER',
    register: null,
    label: `OTHER ${bytes.slice(10).map((b) => b.toString(16).padStart(2, '0')).join(' ')}`
  };
}

const isReply = (f) => f.dir === 'RX' && (f.op === RFID_FIRMWARE_OP || f.op === ERROR_NOTIFICATION_OP);
const isOpTx = (f) => f.dir === 'TX' && f.op === RFID_FIRMWARE_OP;

/**
 * For every `0x8002` transmission, was it answered before the host moved on?
 *
 * The window closes at the next transmission of the same op, not only at the
 * budget: once the host has sent another `0x8002` any reply is ambiguous, since
 * every RFID firmware command shares this one code.
 */
export function analyseTransmissions(frames, { budgetMs = DEFAULT_BUDGET_MS } = {}) {
  const ordered = [...frames].sort((a, b) => a.t - b.t);
  const out = [];
  for (let i = 0; i < ordered.length; i += 1) {
    const f = ordered[i];
    if (!isOpTx(f)) continue;
    let reply = null;
    for (let j = i + 1; j < ordered.length; j += 1) {
      const g = ordered[j];
      if (g.t - f.t > budgetMs) break;
      if (isOpTx(g)) break;
      if (isReply(g)) {
        reply = g;
        break;
      }
    }
    out.push({
      t: f.t,
      ...classifyFirmwarePayload(f.bytes),
      answered: reply !== null,
      latencyMs: reply ? reply.t - f.t : null
    });
  }
  return out;
}

/** sent / unanswered per command label, most-unanswered first. */
export function tallyByLabel(transmissions) {
  const rows = new Map();
  for (const tx of transmissions) {
    const row = rows.get(tx.label) ?? { label: tx.label, sent: 0, unanswered: 0 };
    row.sent += 1;
    if (!tx.answered) row.unanswered += 1;
    rows.set(tx.label, row);
  }
  return [...rows.values()].sort((a, b) => b.unanswered - a.unanswered || b.sent - a.sent);
}

/**
 * Of the transmissions that went unanswered, how many were rescued by the retry?
 *
 * `neverRetried` is the row that matters most and is easiest to overlook: a
 * command with no retry schedule produces no follow-up at all, so it lands here
 * rather than in `retriedAndFailed`. Reading only the two retry rows would
 * report a clean recovery rate for a command that never retried once.
 */
export function retryOutcome(transmissions, { retryWindowMs = DEFAULT_RETRY_WINDOW_MS } = {}) {
  const unanswered = transmissions.filter((tx) => !tx.answered);
  const result = { unanswered: unanswered.length, retriedAndAnswered: 0, retriedAndFailed: 0, neverRetried: 0, gapsMs: [] };
  for (const miss of unanswered) {
    const next = transmissions.find((tx) => tx.t > miss.t && tx.t - miss.t <= retryWindowMs);
    if (!next) {
      result.neverRetried += 1;
      continue;
    }
    result.gapsMs.push(next.t - miss.t);
    if (next.answered) result.retriedAndAnswered += 1;
    else result.retriedAndFailed += 1;
  }
  result.gapsMs.sort((a, b) => a - b);
  return result;
}

/** Read a ring, keeping only whole frames inside the window. */
export async function readRing(ringPath, { from = -Infinity, to = Infinity } = {}) {
  const rl = readline.createInterface({ input: fs.createReadStream(ringPath), crlfDelay: Infinity });
  const frames = [];
  for await (const line of rl) {
    const frame = parseFrame(line);
    if (!frame) continue;
    if (frame.t < from || frame.t > to) continue;
    frames.push(frame);
  }
  return frames;
}

const quantile = (sorted, p) => (sorted.length ? sorted[Math.floor(sorted.length * p)] : null);

async function main() {
  const [ringPath, fromISO, toISO] = process.argv.slice(2);
  if (!ringPath) {
    console.error('usage: node scripts/ring-unanswered-commands.mjs <ring.jsonl> [fromISO] [toISO]');
    process.exitCode = 2;
    return;
  }
  const from = fromISO ? Date.parse(fromISO) : -Infinity;
  const to = toISO ? Date.parse(toISO) : Infinity;
  if (Number.isNaN(from) || Number.isNaN(to)) {
    console.error('fromISO/toISO must parse as dates');
    process.exitCode = 2;
    return;
  }
  if (!fromISO || !toISO) {
    console.error('⚠ no window given — a ring usually spans more than the arm that dumped it.');
  }

  const frames = await readRing(ringPath, { from, to });
  const transmissions = analyseTransmissions(frames);
  const unanswered = transmissions.filter((tx) => !tx.answered);

  console.log(`0x8002 transmissions: ${transmissions.length}   answered: ${transmissions.length - unanswered.length}   UNANSWERED: ${unanswered.length}`);

  console.log('\nlabel                    sent  unanswered     rate');
  for (const row of tallyByLabel(transmissions)) {
    if (row.unanswered === 0 && row.sent < 500) continue;
    console.log(
      `  ${row.label.padEnd(22)} ${String(row.sent).padStart(6)}  ${String(row.unanswered).padStart(6)}  ${(100 * row.unanswered / row.sent).toFixed(3).padStart(7)}%`
    );
  }

  const retry = retryOutcome(transmissions);
  console.log('\nof the unanswered:');
  console.log(`  retry followed and WAS answered : ${retry.retriedAndAnswered}`);
  console.log(`  retry followed and was NOT      : ${retry.retriedAndFailed}`);
  console.log(`  no retry transmitted at all     : ${retry.neverRetried}`);
  if (retry.gapsMs.length) {
    console.log(`  retry fired after: min=${retry.gapsMs[0]} p50=${quantile(retry.gapsMs, 0.5)} max=${retry.gapsMs[retry.gapsMs.length - 1]} ms`);
  }

  const latencies = transmissions.filter((tx) => tx.answered).map((tx) => tx.latencyMs).sort((a, b) => a - b);
  if (latencies.length) {
    console.log(
      `\nanswered latency ms: n=${latencies.length} min=${latencies[0]} p50=${quantile(latencies, 0.5)} p99=${quantile(latencies, 0.99)} max=${latencies[latencies.length - 1]}`
    );
    console.log(`  answers slower than 500ms: ${latencies.filter((x) => x > 500).length}`);
  }
}

if (import.meta.url === `file://${process.argv[1]}`) {
  main();
}
