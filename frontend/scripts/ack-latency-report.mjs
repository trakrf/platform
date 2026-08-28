#!/usr/bin/env node
/**
 * Ack-latency distribution report for the hardware soak.
 *
 * Pure function of `.suite-runs/runs.jsonl` and the logs it points at. Never
 * touches hardware, never mutates the record. Safe to run mid-soak, which is
 * the point — two of its three checks are stop-the-soak findings and waiting
 * until hour 17 to compute them wastes the run.
 *
 * WHY A DISTRIBUTION AND NOT A PERCENTILE
 *
 * Since ble-mcp-test 0.9.0, `writeValue()` resolves on the bridge's ack, so ack
 * latency `L` is spent INSIDE `WRITE_BUDGET_MS`. `withinBudget` gates the sleep
 * before a retry and never the write itself, so the budget bounds retry COUNT
 * and structurally cannot bound duration. Against `CommandManager`'s 2500ms the
 * final attempt overruns in two disjoint, non-monotonic windows:
 *
 *      584 <= L <   625     2 retries, ends up to 2624ms
 *     1126 <= L <= 1500     1 retry,   ends up to 3250ms
 *
 * L=700 is safe while L=600 is not, so no single threshold can watch this, and
 * a p99 over a bimodal sample is arithmetically correct while hiding the mode
 * that matters. Hence: buckets, window occupancy, and named edge tests.
 *
 * THE THREE CHECKS, and what each can falsify
 *
 *   1. CO-OCCURRENCE (stop the soak on ONE hit)
 *      A link close whose timestamp falls inside a failed write's window
 *      [t-ms, t]. That is the signature of a write left pending across a link
 *      drop — the condition ble-mcp-test's `failPendingWrites` exists to make
 *      impossible, and which reported as a full-cap ACK_TIMEOUT while it was
 *      unwired. One occurrence is a finding; it cannot be explained away as a
 *      slow radio, so it needs no rate.
 *
 *   2. THE 1500ms EDGE (>2% is the backstop when join data is lossy)
 *      Pre-registered before any data existed: density at the top edge of
 *      window 2 should be INDISTINGUISHABLE from the rest of the window. A
 *      visible edge means writes are still being failed by the cap rather than
 *      by the radio. Bare presence is deliberately NOT the trigger — a genuine
 *      ack timeout at genuinely 1500ms is a real radio event.
 *
 *   3. CONNECT CLUSTER NEAR 10s — INVERTED AS OF ble-mcp-test 0.12.0
 *      This used to mean "the known upstream hang". `connect()` rejected
 *      promptly only on close codes 4000-4999 and the Python bridge never sent
 *      one, so every real connect-time close waited out the full 10s timeout,
 *      and a cluster there was that path rather than the reader.
 *
 *      0.12.0 rejects immediately with `CLOSED_BEFORE_CONNECTED`. So the
 *      cluster now means something NEW and unexplained, and the old sentence
 *      would send its reader to a bug that no longer exists. The threshold is
 *      unchanged; only what it implies has flipped.
 *
 *      Two consequences for the baseline, both easy to misread:
 *      - a connect failure is now cheap, so against an unhealthy bridge
 *        attempts land ~250ms apart (the harness cooldown) instead of ~10.25s.
 *        A raw connect-failure COUNT is therefore not comparable across this
 *        version boundary; the rate per unit time is.
 *      - connect rejections now carry a `code`, which this instrument already
 *        records generically, so they classify themselves.
 *
 * Baseline instrument for TRA-1189 Phase 1.
 *
 * Usage:
 *   node scripts/ack-latency-report.mjs
 *   node scripts/ack-latency-report.mjs --log /path/to/one-run.log   (ad hoc)
 */

import { readFileSync, existsSync } from 'node:fs';
import path from 'node:path';

const RECORD_PATH = path.resolve(process.cwd(), '.suite-runs', 'runs.jsonl');

/** Overrun windows from CS108BLETransport's WRITE_BUDGET_MS comment. */
const WINDOWS = [
  { name: 'window 1', lo: 584, hi: 624 },
  { name: 'window 2', lo: 1126, hi: 1500 },
];

/** Top slice of window 2 that the ack cap would pile samples into. */
const EDGE_LO = 1476;
const EDGE_HI = 1500;
const EDGE_SHARE_LIMIT = 0.02;

const CONNECT_HANG_MS = 9500;

function parseFields(line) {
  const out = {};
  for (const [, k, v] of line.matchAll(/(\w+)=([^\s]+)/g)) out[k] = v;
  return out;
}

/**
 * Pull every instrument line out of one log.
 *
 * Returns the three event streams separately. Merging them is how a detector
 * that sees nothing gets read as a detector that saw zero occurrences.
 */
function parseLog(text, source) {
  const writes = [];
  const closes = [];
  const connects = [];

  for (const line of text.split('\n')) {
    const at = line.indexOf('[ble-timing] ');
    if (at === -1) continue;
    const rest = line.slice(at + '[ble-timing] '.length);
    const kind = rest.split(' ', 1)[0];
    const f = parseFields(rest);

    if (kind === 'write-ack') {
      writes.push({
        source,
        t: Number(f.t),
        ms: Number(f.ms),
        attempt: f.attempt,
        outcome: f.outcome,
      });
    } else if (kind === 'link-close') {
      closes.push({ source, t: Number(f.t), inflight: f.inflight === '1', queued: Number(f.queued) });
    } else if (kind === 'connect') {
      connects.push({ source, t: Number(f.t), ms: Number(f.ms), outcome: f.outcome });
    }
  }
  return { writes, closes, connects };
}

function loadSources() {
  const argLog = process.argv.includes('--log')
    ? process.argv[process.argv.indexOf('--log') + 1]
    : null;

  if (argLog) {
    if (!existsSync(argLog)) {
      console.error(`No such log: ${argLog}`);
      process.exit(1);
    }
    return [{ label: path.basename(argLog), text: readFileSync(argLog, 'utf8') }];
  }

  if (!existsSync(RECORD_PATH)) {
    console.error(`No record at ${RECORD_PATH} — run characterise-suite-runs.mjs first.`);
    process.exit(1);
  }

  const records = readFileSync(RECORD_PATH, 'utf8')
    .split('\n')
    .filter(Boolean)
    .map((line) => JSON.parse(line));

  const sources = [];
  for (const r of records) {
    const logPath = r.outputLog ?? r.stdoutLog;
    // A record whose log is gone is UNREADABLE, not empty. Said out loud below
    // rather than folded into the totals, because a shrinking denominator that
    // nobody mentions is how a distribution quietly stops describing the run.
    if (!logPath || !existsSync(logPath)) {
      sources.push({ label: `${r.shape}#${r.rep}`, text: null });
      continue;
    }
    sources.push({ label: `${r.shape}#${r.rep}`, text: readFileSync(logPath, 'utf8') });
  }
  return sources;
}

function histogram(samples) {
  // Fixed edges, chosen so both windows land on bucket boundaries rather than
  // being split across them — a bucket that straddles a window edge reports
  // occupancy that is neither.
  const edges = [0, 100, 200, 300, 400, 500, 584, 625, 750, 1000, 1126, 1250, 1400, 1476, 1501, 2000, 3000, Infinity];
  const counts = new Array(edges.length - 1).fill(0);
  for (const ms of samples) {
    for (let i = 0; i < counts.length; i++) {
      if (ms >= edges[i] && ms < edges[i + 1]) {
        counts[i]++;
        break;
      }
    }
  }
  const total = samples.length || 1;
  const widest = Math.max(...counts);
  const lines = [];
  for (let i = 0; i < counts.length; i++) {
    const hi = edges[i + 1] === Infinity ? '∞' : String(edges[i + 1]);
    const bar = widest ? '█'.repeat(Math.round((counts[i] / widest) * 40)) : '';
    const pct = ((counts[i] / total) * 100).toFixed(2);
    lines.push(`  ${String(edges[i]).padStart(5)}–${hi.padEnd(5)} ${String(counts[i]).padStart(7)} ${pct.padStart(6)}%  ${bar}`);
  }
  return lines.join('\n');
}

function main() {
  const sources = loadSources();
  const writes = [];
  const closes = [];
  const connects = [];
  let unreadable = 0;

  for (const s of sources) {
    if (s.text === null) {
      unreadable++;
      continue;
    }
    const parsed = parseLog(s.text, s.label);
    writes.push(...parsed.writes);
    closes.push(...parsed.closes);
    connects.push(...parsed.connects);
  }

  console.log('# Ack-latency report\n');
  console.log(`sources          ${sources.length} (${unreadable} unreadable — log gone, NOT counted as zero)`);
  console.log(`write attempts   ${writes.length}`);
  console.log(`link closes      ${closes.length}`);
  console.log(`connect attempts ${connects.length}\n`);

  // CANARY. Zero samples is a void capture, not a clean run.
  if (writes.length === 0) {
    console.log('⚠ CANARY: zero write-ack samples.');
    console.log('  The instrument did not reach the captured log. Every number below is');
    console.log('  uninformative rather than zero. Check that transport console output is');
    console.log('  captured — `--reporter=json` alone intercepts it.\n');
    process.exitCode = 1;
    return;
  }

  const latencies = writes.map((w) => w.ms);
  console.log('## Distribution (ms)\n');
  console.log(histogram(latencies));
  console.log('\n  No percentile is printed, deliberately. See this file\'s header.\n');

  console.log('## Window occupancy\n');
  for (const w of WINDOWS) {
    const inWindow = latencies.filter((ms) => ms >= w.lo && ms <= w.hi);
    const pct = ((inWindow.length / latencies.length) * 100).toFixed(2);
    console.log(`  ${w.name}  ${w.lo}–${w.hi}ms   ${String(inWindow.length).padStart(6)}  ${pct.padStart(6)}%`);
  }
  console.log('');

  // ── Check 2: the pre-registered 1500ms edge test ──────────────────────────
  const w2 = WINDOWS[1];
  const inW2 = latencies.filter((ms) => ms >= w2.lo && ms <= w2.hi);
  const atEdge = latencies.filter((ms) => ms >= EDGE_LO && ms <= EDGE_HI);
  const edgeShareOfAll = atEdge.length / latencies.length;
  const bodyLo = w2.lo;
  const bodyHi = EDGE_LO - 1;
  const inBody = latencies.filter((ms) => ms >= bodyLo && ms <= bodyHi);
  const edgeDensity = atEdge.length / (EDGE_HI - EDGE_LO + 1);
  const bodyDensity = inBody.length / (bodyHi - bodyLo + 1);

  console.log('## 1500ms edge (pre-registered: edge density ≈ body density)\n');
  console.log(`  edge ${EDGE_LO}–${EDGE_HI}   n=${atEdge.length}   density=${edgeDensity.toFixed(4)} /ms`);
  console.log(`  body ${bodyLo}–${bodyHi}   n=${inBody.length}   density=${bodyDensity.toFixed(4)} /ms`);
  console.log(`  window 2 total n=${inW2.length}`);
  console.log(`  edge share of ALL samples ${(edgeShareOfAll * 100).toFixed(2)}% (limit ${(EDGE_SHARE_LIMIT * 100).toFixed(0)}%)`);
  if (edgeShareOfAll > EDGE_SHARE_LIMIT) {
    console.log('\n  ⚠ FINDING: the edge is occupied beyond the agreed backstop.');
    console.log('    Ack timeouts are still being produced by the cap. Ping the bridge session.');
    process.exitCode = 1;
  } else if (atEdge.length && bodyDensity > 0 && edgeDensity > bodyDensity * 3) {
    console.log('\n  ⚠ FINDING: edge density is visibly above the window body.');
    console.log('    Under the backstop share, but the shape is the one that was pre-registered as absent.');
    process.exitCode = 1;
  } else {
    console.log('\n  OK — no visible edge.');
  }
  console.log('');

  // ── Check 1: co-occurrence. ONE is a finding. ─────────────────────────────
  const failed = writes.filter((w) => w.outcome !== 'ok');
  const hits = [];
  for (const w of failed) {
    for (const c of closes) {
      if (c.source === w.source && c.t >= w.t - w.ms && c.t <= w.t) {
        hits.push({ w, c });
      }
    }
  }
  console.log('## Close-inside-write-window (STOP THE SOAK on one hit)\n');
  console.log(`  failed writes ${failed.length}   closes ${closes.length}   co-occurrences ${hits.length}`);
  if (hits.length) {
    console.log('\n  ⚠ FINDING — a write was outstanding when the link closed:');
    for (const { w, c } of hits.slice(0, 10)) {
      console.log(`    ${w.source}  write t=${w.t} ms=${w.ms} outcome=${w.outcome}  close t=${c.t} inflight=${c.inflight ? 1 : 0} queued=${c.queued}`);
    }
    console.log('\n  Stop the run and ping the bridge session. This is the condition');
    console.log('  failPendingWrites exists to make impossible.');
    process.exitCode = 1;
  } else {
    console.log('\n  OK — none.');
  }

  const inflightCloses = closes.filter((c) => c.inflight);
  if (inflightCloses.length) {
    console.log(`\n  Note: ${inflightCloses.length} close(s) reported inflight=1 without a matching`);
    console.log('  failed write in window. Same class, weaker evidence — worth reporting.');
  }
  console.log('');

  // ── Check 3: connect attribution ──────────────────────────────────────────
  const connectFailures = connects.filter((c) => c.outcome !== 'ok');
  const hung = connectFailures.filter((c) => c.ms >= CONNECT_HANG_MS);
  console.log('## Connect attribution\n');
  console.log(`  attempts ${connects.length}   failures ${connectFailures.length}   ≥${CONNECT_HANG_MS}ms ${hung.length}`);
  if (hung.length) {
    console.log('\n  ⚠ FINDING: connect failures at or past the old 10s timeout.');
    console.log('    Before ble-mcp-test 0.12.0 this was the known upstream onclose gap and');
    console.log('    could be dismissed. 0.12.0 rejects immediately with CLOSED_BEFORE_CONNECTED,');
    console.log('    so a cluster here is NOT that bug and is not explained. Check the `outcome`');
    console.log('    codes below before attributing anything to the radio.');
    const byCode = new Map();
    for (const c of hung) byCode.set(c.outcome, (byCode.get(c.outcome) ?? 0) + 1);
    for (const [code, n] of byCode) console.log(`      ${code} ${n}`);
    process.exitCode = 1;
  }

  // Attempts per unit time, because the raw count is not comparable across the
  // 0.12.0 boundary: the old 10s hang was an accidental backoff, and removing it
  // multiplies attempts against an unhealthy bridge without anything being worse.
  if (connects.length > 1) {
    const span = Math.max(...connects.map((c) => c.t)) - Math.min(...connects.map((c) => c.t));
    if (span > 0) {
      const perMin = (connects.length / (span / 60000)).toFixed(2);
      const failPerMin = (connectFailures.length / (span / 60000)).toFixed(2);
      console.log(`\n  attempts/min ${perMin}   failures/min ${failPerMin}   over ${(span / 60000).toFixed(1)} min`);
    }
  }
  console.log('');
}

main();
