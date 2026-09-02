/**
 * Log signatures shared by the run-shape driver and its summariser.
 *
 * Lives in its own module so the two scripts cannot drift apart: a needle that
 * exists in only one of them produces a detector that reads 0 in the summary
 * while the driver saw occurrences, or vice versa.
 */

import { readFileSync, existsSync } from 'node:fs';

export const SIGNALS = {
  triggerTimeout: 'Timeout waiting for event: TRIGGER_STATE_CHANGED',
  // BOTH limbs, and the reason is a near-miss worth keeping. The trigger case
  // awaits startScanning() on press and stopScanning() on release, and either
  // rethrowing skips the postWorkerEvent() below it. The first version of this
  // detector encoded only the press limb, so it reported 0 across four genuine
  // failures — which reads as "hypothesis refuted" rather than "detector looked
  // in one of the two places". The real line every time was the RELEASE limb.
  //
  // A detector that encodes half a hypothesis returns a confident zero.
  startScanFailed: '[Reader] Failed to start scanning:',
  stopScanFailed: '[Reader] Failed to stop scanning:',
  // CANARY, not a finding. 0 here means the capture was void and the other
  // counts are uninformative rather than zero. Without it, a broken capture and
  // a clean run are the same record: all-zero counts with logMissing:false.
  // That already happened — `--reporter=json` alone intercepts console output,
  // and the detector read 0 timeouts on repetitions that had just timed out.
  //
  // The margin here narrowed on 2026-08-27 and the canary is now load-bearing
  // rather than incidental. It used to count hundreds of lines per repetition,
  // because the old harness logged every packet in both directions. TRA-1187
  // replaced that harness with one that drives the production transport and
  // logs once per connect, on purpose — so this needle now matches roughly one
  // line per spec file rather than hundreds. Still non-zero for any captured
  // run, which is all the canary asserts, but do not read a low count as a
  // problem, and do not delete that log line thinking it is noise.
  harnessLines: '[Harness]',
  // ENVIRONMENTAL, not a defect. The bridge holds the BLE link for its process
  // lifetime, so if it dies mid-soak every subsequent repetition fails — a
  // valid-looking JSON report saying the suite failed, with no clue in it that
  // there was no transport at all. Those repetitions measured a missing bridge,
  // not the subsystem under test, and pooling them with real failures shows up
  // as failures that lack the signature — i.e. as counter-evidence against
  // whatever hypothesis is being tested. That happened here: a dead bridge
  // briefly read as 6 failures disproving a mechanism they never exercised.
  //
  // TWO needles, because the shape changed with the path. The old integration
  // route used ble-mcp-test's Node client, whose socket surfaced a Node error:
  // `connect ECONNREFUSED 127.0.0.1:8080`. Since TRA-1187 the route is
  // `CS108BLETransport -> navigator.bluetooth -> mock -> ws-transport`, and the
  // mock's transport uses whatever global WebSocket the runtime provides — under
  // vitest that is jsdom's, which reports a bare `WebSocket error` and no errno
  // at all. Verified 2026-08-27 by running connection.spec.ts with no bridge
  // listening: the failure was `Error: WebSocket error` with zero ECONNREFUSED
  // anywhere in the output.
  //
  // Keeping only the old needle would have made every dead-bridge repetition of
  // an overnight soak look like a genuine suite failure. Keep both: e2e and any
  // Node-side caller can still produce the errno form.
  transportRefused: 'ECONNREFUSED',
  transportUnreachable: 'WebSocket error',
  // CANARY for the ack-latency instrument, and it is a canary in the same sense
  // as `harnessLines`: a captured run with zero of these did not measure a clean
  // link, it measured nothing. Every write attempt emits one, so 0 across a
  // repetition that ran any command means the transport lines are not reaching
  // the captured log — a detector that cannot see what it measures reads as an
  // empty distribution, which is indistinguishable from a healthy one.
  ackSamples: '[ble-timing] write-ack',
  // A link close with a write outstanding is the signature the soak watches for.
  // Counted here so the driver's own record shows it; the JOIN that decides
  // whether it landed inside a write window needs the timestamps and lives in
  // scripts/ack-latency-report.mjs.
  linkCloses: '[ble-timing] link-close',
  connectSamples: '[ble-timing] connect',

  // ── The CS108's silent window, after TRA-1217 made it survivable ──────────
  //
  // ⚠ THE FIX DISABLED THE DETECTOR. Read this before trusting a clean arm.
  //
  // The device stops acknowledging RFID_POWER_OFF (0x8001) for long stretches —
  // 82 minutes across 63 reps on 2026-08-31 — while answering every 0x8002
  // one-for-one and streaming tag data. It used to announce itself: the teardown
  // failed, `locate.spec.ts`'s afterAll threw past its own `cleanup()`, the link
  // stayed claimed, and `linkCloses` above counted 63 of them.
  //
  // TRA-1217 fixed both halves. A mode change now tolerates the unanswered
  // power-off and `cleanup()` always runs — so a recurrence costs nothing, kills
  // no reps, and **emits no link-close**. Which leaves these two indistinguishable
  // on every count above:
  //
  //     the window recurred and was absorbed   ->  reps pass, linkCloses 0
  //     the window never happened              ->  reps pass, linkCloses 0
  //
  // An arm cannot tell you the fix worked if it cannot tell you the condition
  // occurred. These three needles are what separates them, and they are the
  // falsification test on TRA-1217 written out mechanically: if the fix works,
  // `powerOffTimeouts` stays NON-ZERO while `linkCloses` and rep failures go to
  // zero. If all of them go to zero together, something else moved and TRA-1217
  // is not what did it. TRA-1223.
  //
  // Per-attempt, so this is the raw count of device silence — 14-23 per rep
  // inside the 2026-08-31 window, 0 in all 137 clean reps. Perfect separation,
  // which is what makes it worth counting rather than a noisy proxy.
  powerOffTimeouts: '[CommandManager] Command timeout: RFID_POWER_OFF',
  // Once per mode change that spent its whole retry schedule and carried on
  // anyway. This is the rescue counter: every occurrence is a rep that would
  // have died before TRA-1217.
  toleratedPowerOffs: 'tolerated, continuing the sequence',
  // The teardown giving up. 63/63 in the failing reps, 0/137 in the clean ones.
  // Should now be ZERO even when the window recurs — if it fires alongside
  // `powerOffTimeouts`, the tolerance did not hold and the fix is incomplete.
  modeSwitchFailed: 'Mode switching failed during cleanup',

  // ── The host leaking the wire, which is NOT the device going silent ───────
  //
  // `CommandInFlightError`. Every other counter above is about what the DEVICE
  // did; this one is about the host claiming the in-flight slot and failing to
  // give it back, after which the next dispatch is refused against a command
  // that never reached the radio.
  //
  // TARGET: ZERO — including inside a wedge window, which is the only place it
  // has ever been observed. Not "low". A non-zero count means something claims
  // the slot without releasing it, which is what the error's docblock in
  // worker/cs108/command.ts now tells a reader to go looking for.
  //
  // ⚠ Counted, not parsed per op, and that breaks deliberately from
  // `commandTimeouts` / `commandRejections` a few lines down. Those parse
  // because a fixed list can only count what somebody enumerated (TRA-1226).
  // That argument does not carry here: one constructor emits one message, and
  // the half that actually kills a mode change —
  //
  //   [setMode] Failed to set Idle mode: CommandInFlightError: Command already
  //   active - executeCommand called concurrently
  //
  // — carries no op name at all. A per-op table would report `{}` for those and
  // read as coverage it does not have.
  //
  // ⚠ TWO LINES PER OCCURRENCE: the tolerated step's WARN and the failing
  // sequence's ERROR. The 2026-09-01 arm's 26 lines are 13 events. Zero is
  // zero either way, which is why the target survives the ambiguity — but do
  // not report this count as an event count.
  //
  // It exists because TRA-1239 was pre-registered against a hand-counted
  // baseline ("6 per 200, reps 5/6/39, where the device was refusing") that the
  // archive contradicts in both halves: 13, all in reps 137-143, and no refusal
  // involved. A number nobody can regenerate is a number that drifts. TRA-1239.
  commandInFlight: 'Command already active - executeCommand called concurrently',
};

/**
 * The CommandManager's timeout line, up to the op name it appends.
 *
 * Shared by `powerOffTimeouts` above and by `countCommandTimeouts` below, so the
 * needle and the parser cannot drift into counting different lines.
 */
export const COMMAND_TIMEOUT_PREFIX = '[CommandManager] Command timeout: ';

/**
 * Every command timeout in a log, broken down by op name (TRA-1226).
 *
 * ## Why this is a parser and not more needles
 *
 * `powerOffTimeouts` above counts ONE op code. On the 2026-08-31 arm that meant
 * 203 command timeouts of which 138 were counted: `GET_TRIGGER_STATE` (0xA001)
 * ran 63 and `RFID_FIRMWARE_COMMAND` (0x8002) ran 2, and **neither reached any
 * summary, RUN-IDENTITY, or cross-arm comparison.** The 0x8002 one is what
 * failed rep 1 — it put the reader into Error, so a deferred `targetEPC` push
 * was abandoned and Locate ran against the previously applied mask.
 *
 * Adding two more needles would have fixed those two and left the next one
 * invisible. A fixed list can only count what somebody thought to enumerate and
 * reads a confident 0 for the rest — the same defect as TRA-1224's allowlist,
 * one level up. That is not hypothetical here: TRA-1223 asserted the device
 * ignored "exactly one op code" while its own first-occurrence table carried
 * 0xA001 at 76 TX / 14 RX, and no instrument contradicted it because no
 * instrument was looking.
 *
 * So the op name is parsed out of the line. **A newly-silent op code appears
 * without anyone having predicted it**, which is the property that matters.
 *
 * ⚠ Returns `{}` for a log that carried no timeouts — a real measurement — and
 * the caller must keep that distinct from the `null` a runner gets when it
 * cannot observe these at all. Same null-vs-zero rule as everywhere else here.
 */
/**
 * The producer's own running total, on its UNCONDITIONAL `fault-count` line.
 *
 * ⚠ Deliberately NOT the descriptive `[CS108 Error] <desc>` line, which is rate
 * limited. Reading the total off a limited line undercounts whenever the storm
 * ends inside the suppression window — measured at exactly 2x on a 3-rep mini
 * arm, 18 frames reported as 9. Refs TRA-1231.
 *
 * Exported so the parser below and any future needle read the same shape, and
 * so `every-signal-needle-has-a-producer` can find the string in `src/`.
 */
export const ERROR_NOTIFICATION_TOTAL_RE = /\[CS108 Error\] fault-count total=(\d+)/g;

/**
 * How many `0xA101` ERROR_NOTIFICATION frames the worker saw in a rep (TRA-1229).
 *
 * ## Why this reads a total instead of counting lines
 *
 * `0xA101` is how the CS108 refuses a command: the rejection comes back under
 * 0xA101 and never under the op code being rejected. On the 2026-09-01 arm the
 * device sent 1543 of them inside one 86-minute window, one per unanswered
 * command, 34 ms after the command they answered. The arm reported none of it:
 * `reader.ts` discarded the frames, and the handler's rate limiter would have
 * capped what survived at a handful of lines per code anyway.
 *
 * The discard is gone, but the rate limiter stays — an 18-per-minute fault
 * storm should not bury the rep log. So counting lines would undercount by two
 * orders of magnitude, which is the failure this file exists to prevent:
 * a confident number measured off the wrong population.
 *
 * The producer therefore carries its own unconditional total on every line it
 * does emit, and a rate-limited line still reports an accurate count.
 *
 * ## Why the highest total is NOT the answer (TRA-1236)
 *
 * The counter lives on `ErrorNotificationHandler`, which is per WORKER SESSION.
 * A wedged rep reconnects repeatedly, so each session writes its own
 * `total=1,2,3…` sequence and the highest number in the log belongs to whichever
 * session ran longest — not to the rep. Taking a global maximum read one session
 * and called it the rep.
 *
 * It was ~10x low precisely in the wedged reps, which are the ones an arm exists
 * to measure, and correct everywhere else:
 *
 *   rep 137  errNotif 4  rejections 36
 *   rep 140  errNotif 3  rejections 39
 *
 * Found by an inequality that cannot hold — every rejection IS an 0xA101 and not
 * every 0xA101 produces a rejection, so `rejections <= errorNotifications` is
 * forced, and the arm reported 247 > 208.
 *
 * So: walk the totals and sum the maximum of each monotonically increasing run.
 * A total that is less than OR EQUAL TO its predecessor starts a new run —
 * equality counts because a single session never emits the same total twice, the
 * counter being incremented before the line is written. Equality can therefore
 * only mean the counter was reset.
 *
 * ⚠ Returns 0 for a clean rep — a real measurement — never `null`. Same
 * null-vs-zero rule as the rest of this table.
 */
export function countErrorNotifications(text) {
  let total = 0;
  let runMax = 0;
  for (const m of text.matchAll(ERROR_NOTIFICATION_TOTAL_RE)) {
    const n = Number(m[1]);
    if (!Number.isFinite(n)) continue;
    if (n <= runMax) {
      // The counter went backwards or repeated: a new worker session started.
      // Bank the run that just ended before opening the next one.
      total += runMax;
      runMax = n;
    } else {
      runMax = n;
    }
  }
  return total + runMax;
}

/**
 * The prefix a refused command logs, mirroring `COMMAND_TIMEOUT_PREFIX`.
 *
 * Exported so the parser and the producer cannot drift, and so
 * `every-signal-needle-has-a-producer` can find the string in `src/`.
 */
export const COMMAND_REJECTION_PREFIX = '[CommandManager] Command rejected: ';

/**
 * Every command the DEVICE refused, by op name (TRA-1230).
 *
 * ## Why this exists separately from countCommandTimeouts
 *
 * `[CommandManager] Command timeout: <name>` is logged from exactly one place —
 * the `setTimeout` callback. TRA-1229 settles a refused command from its
 * `0xA101` reply in ~34ms, which CLEARS that timeout, so the line never fires.
 * Every needle keyed to it then reads a confident zero: on the 2026-09-01 arm
 * shape that turns `powerOffTimeouts 1115` into `0` and the per-op table into
 * `{}`, through a window in which the device refused ~1500 commands.
 *
 * A timeout and a refusal are different device behaviours — "no answer came"
 * versus "the answer was no" — and collapsing them would lose the distinction
 * the whole TRA-1223 investigation turned on. So they are counted separately
 * and read together.
 *
 * Parsed per op for the same reason as the timeout table: a fixed list counts
 * only what somebody enumerated. `GET_TRIGGER_STATE` reached no summary for
 * weeks because nobody had thought to add it.
 *
 * ⚠ Returns `{}` for a rep that carried no refusals — a real measurement — and
 * the caller must keep that distinct from the `null` a runner gets when it
 * cannot observe these at all.
 */
export function countCommandRejections(text) {
  const counts = {};
  // SPLIT on the literal prefix rather than interpolating into a RegExp, for
  // the reason spelled out on countCommandTimeouts: a pattern assembled from a
  // string is correct only while nobody edits the string.
  const parts = text.split(COMMAND_REJECTION_PREFIX);
  for (let i = 1; i < parts.length; i += 1) {
    // `<OP_NAME> — <desc> (0xNNNN)` — the op name runs to the first space.
    const op = parts[i].split(/[\s\n]/, 1)[0];
    if (op) counts[op] = (counts[op] ?? 0) + 1;
  }
  return counts;
}

export function countCommandTimeouts(text) {
  const counts = {};
  // SPLIT on the literal prefix rather than interpolating it into a RegExp.
  //
  // The first version built `new RegExp(PREFIX.replace(/[[\]]/g, '\\$&') + ...)`
  // and escaped only the brackets it happened to know about. CodeQL flagged it:
  // a backslash in the prefix would not be escaped and would corrupt the
  // pattern. Widening the escape set to the full metacharacter list fixes that
  // instance and leaves the shape — a regex assembled from a string, correct
  // only while nobody edits the string. Splitting on the literal cannot be
  // wrong about escaping because it never escapes anything, and it is the same
  // idiom `readSignals` already uses to count needles a few lines up.
  //
  // Only the op-name matcher stays a regex, and it is a static literal.
  const parts = text.split(COMMAND_TIMEOUT_PREFIX);
  for (let i = 1; i < parts.length; i++) {
    // Op names are the CS108Event `name` fields: SCREAMING_SNAKE_CASE, anchored
    // to the character immediately after the prefix.
    const op = /^[A-Z0-9_]+/.exec(parts[i]);
    if (op) counts[op[0]] = (counts[op[0]] ?? 0) + 1;
  }
  return counts;
}

/**
 * `countCommandTimeouts` for a log on disk.
 *
 * `null` when the log is gone — it measured nothing, and an empty map would say
 * the opposite.
 */
export function readCommandTimeouts(logPath) {
  if (!logPath || !existsSync(logPath)) return null;
  try {
    return countCommandTimeouts(readFileSync(logPath, 'utf8'));
  } catch {
    return null;
  }
}

/**
 * What the reader said it was (TRA-1232).
 *
 * ## Why an arm has to carry this
 *
 * Every capture we hold is unattributed. The 2026-09-01 campaign produced four
 * transport captures of a device-side defect and none of them can say what
 * firmware it was observed on; that had to be reconstructed from notes after
 * the fact, which is the "quoted rather than measured" failure the campaign
 * spent itself correcting. Flashing the reader destroys the attribution
 * permanently, so this is the one part of TRA-1232 with a deadline on it.
 *
 * ## Why it is not RUN-IDENTITY
 *
 * The ticket asked for it there. RUN-IDENTITY is written by
 * `watch-soak-abort-criteria.mjs`, which talks to the BRIDGE — and the bridge
 * has no path to the reader's firmware. It reports its own version and the
 * device MAC and nothing else. The reader is the only thing that knows, so the
 * worker logs it and this parses it back, per rep, the same shape as
 * `readReadCycles`.
 *
 * Keep `READER_DETAILS_PREFIX` in step with `READER_DETAILS_LOG_PREFIX` in
 * `src/worker/cs108/system/identity.ts`.
 */
export const READER_DETAILS_PREFIX = '[Reader] Reader details: ';

/**
 * Extract the reader's identity from a captured rep log, or `null`.
 *
 * `null` is a measurement: this rep never heard from the reader. An empty
 * object would say the opposite — that we asked and it has no firmware
 * versions — and is exactly the substitution the rest of this module refuses to
 * make.
 *
 * Takes the LAST line rather than the first. The worker emits one each time a
 * value lands, and the values land at two different moments: three at connect,
 * two more once the radio is powered. The first line is a partial read missing
 * the most valuable of the three versions.
 */
export function readReaderDetails(logPath) {
  if (!logPath || !existsSync(logPath)) return null;
  let text;
  try {
    text = readFileSync(logPath, 'utf8');
  } catch {
    return null;
  }

  // SPLIT on the literal prefix, for the reason spelled out on
  // countCommandTimeouts: a pattern assembled from a string is correct only
  // while nobody edits the string.
  const parts = text.split(READER_DETAILS_PREFIX);
  if (parts.length < 2) return null;

  const line = parts[parts.length - 1].split('\n', 1)[0];
  try {
    const parsed = JSON.parse(line);
    // A JSON scalar is not a reader. Only an object is an answer.
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : null;
  } catch {
    // A capture cut mid-write leaves the JSON unclosed. That rep recorded
    // nothing usable, and it must read as nothing rather than take down the
    // summary every other rep in the arm depends on.
    return null;
  }
}

/**
 * The needles a PLAYWRIGHT repetition can actually produce (TRA-1206).
 *
 * The soak driver gained an e2e backend so it could run the suite TRA-1200
 * measures. The runner is the easy half; this table is the hard half, because
 * most of `SIGNALS` above is vitest-shaped and a needle that cannot fire on a
 * path must not be counted as zero on it.
 *
 * WHAT IS MISSING FROM HERE, AND WHY — each one is a needle whose emitter cannot
 * reach a Playwright rep's captured log, not a needle nobody cared about:
 *
 *   harnessLines     `[Harness]` is written by
 *                    tests/integration/cs108/CS108WorkerTestHarness.ts and by
 *                    nothing else. That file is integration-only; no browser
 *                    ever loads it.
 *   triggerTimeout   Same file — it is CS108WorkerTestHarness that rejects with
 *                    `Timeout waiting for event: ...`.
 *
 *   powerOffTimeouts   `[CommandManager] …` is logged by the worker, which DOES
 *   toleratedPowerOffs run in the browser under e2e — but these are `logger.warn`,
 *                      and `shouldForwardConsoleLine` keeps a non-error line only
 *                      if it contains one of `[ble-timing]`, `Error`, `Failed`,
 *                      `BLE`, `Connect`, `WebSocket`, `force`, `cleanup`,
 *                      `disconnect`. None of those appears in either message,
 *                      so the forwarder drops them and the needle would read a
 *                      confident 0 on every e2e rep however loud the device was.
 *                      INCIDENTAL, not structural — widening the forwarder would
 *                      make them fire. Do that deliberately if an e2e arm ever
 *                      needs them, and measure it rather than assuming (TRA-1209
 *                      is the precedent: the `[ble-timing]` needles sat at a
 *                      confident 0 here for exactly this reason).
 *   modeSwitchFailed   Logged by locate.spec.ts, an integration spec. No browser
 *                      loads it — structural, like the two at the top.
 *
 * STRUCTURAL — `harnessLines`, `triggerTimeout`, `modeSwitchFailed`: the emitter
 * is a file no browser loads, so no change to the e2e path can make them fire.
 * Do not "fix" those. The two `[CommandManager]` needles are the other kind, and
 * the distinction is why each says which it is rather than just "absent".
 *
 * The three `[ble-timing]` needles used to be listed here too, and they were the
 * other kind — absent for an INCIDENTAL reason. They are `console.info` from
 * src/lib/device/transport/cs108-ble-transport.ts, which under e2e runs inside
 * the browser, and the console forwarder's filter matched `BLE`/`Connect`
 * case-sensitively against a lowercase `[ble-timing] connect`. TRA-1209 fixed
 * the forwarder, so they are counted again. The distinction is the reason the
 * original null was recorded rather than a zero: a zero would have been read as
 * "the transport did nothing" and the question would never have been asked.
 */
export const E2E_SIGNALS = {
  // CANARY, and the e2e counterpart to `harnessLines` — see `CAPTURE_CANARY`.
  // `[Connection]` is logged by tests/e2e/helpers/connection.ts on the Node
  // side, so it reaches the captured log directly rather than through the
  // browser console. Every hardware e2e spec connects through that helper.
  //
  // A rep with zero of these never reached the connect helper at all: the dev
  // server was down, the browser failed to launch, the file failed to load.
  // That is a void capture in the same sense `[Harness]` means it — nothing was
  // observed, so every other count in the row is uninformative rather than low.
  e2eConnectLines: '[Connection]',
  // Logged by src/worker/cs108/reader.ts, which runs in the browser under e2e.
  // These DO survive the forwarder: its first limb passes any text containing
  // `Failed`, and both needles do. Reliable for the shared page every hardware
  // spec connects through, because the listener is registered inside
  // connectToDevice() and lives as long as the page.
  startScanFailed: SIGNALS.startScanFailed,
  stopScanFailed: SIGNALS.stopScanFailed,
  // Already documented above as e2e-capable: "Keep both: e2e and any Node-side
  // caller can still produce the errno form." The browser reports the bare
  // `WebSocket error` shape and the forwarder passes it (`WebSocket` is in its
  // allowlist, and it is capitalised the same way there).
  transportRefused: SIGNALS.transportRefused,
  transportUnreachable: SIGNALS.transportUnreachable,
  // Restored by TRA-1209. `console.info` from cs108-ble-transport.ts, which runs
  // in the browser under e2e; they reach the captured log only because
  // `shouldForwardConsoleLine` now matches the `[ble-timing]` prefix explicitly.
  // Without that limb these count 0 on every rep however healthy the link is —
  // which is why they are named in E2E_BROWSER_NEEDLES below.
  //
  // Measured on hardware after the fix: connect 1, write-ack 28, link-close 0.
  //
  // THAT ZERO IS GENUINE, and worth writing down because it looks like the bug
  // that was just fixed. `link-close` is emitted only by `handleDisconnect()`,
  // which is the `gattserverdisconnected` listener — and
  // `CS108BLETransport.disconnect()` REMOVES that listener before tearing the
  // link down. So a deliberate disconnect never emits one. The needle fires on
  // an UNEXPECTED drop, which is the condition it exists to catch ("a link close
  // while a write is outstanding").
  //
  // This is not our local quirk — it is the peer's published contract, and
  // citing it beats re-deriving it, so a future change is found where the rule
  // lives rather than inferred from a zero somebody distrusts:
  //
  //   ble-mcp-test docs/design/2026-08-27-client-contract.md:204
  //     `gattserverdisconnected` fires on a TRANSPORT-LEVEL DROP only, never on
  //     an explicit gatt.disconnect().
  //
  // ⚠ That contract line is an assertion about what real Chrome does, and it is
  // not flagged as a deliberate divergence. So a clean rep scoring 0 confirms
  // THE MOCK MATCHES ITS CONTRACT; it does not confirm the contract matches
  // Chrome. Nothing either repo runs tests the second claim — checking live
  // `navigator.bluetooth` needs the gesture-bound interactive arm TRA-1187
  // deferred to a human at a keyboard. A green e2e run is never evidence of
  // mock fidelity.
  ackSamples: SIGNALS.ackSamples,
  linkCloses: SIGNALS.linkCloses,
  connectSamples: SIGNALS.connectSamples,
};

/**
 * Which e2e needles have a producer that runs IN THE BROWSER.
 *
 * Those lines reach a Playwright rep's captured log only by way of the console
 * forwarder in `tests/e2e/helpers/console-forwarding.ts`. A needle here that the
 * forwarder drops does not read as "dropped" — it reads as a confident `0`,
 * indistinguishable from "the reader never did that". That is exactly how the
 * three `[ble-timing]` needles were lost (TRA-1209).
 *
 * Declaring the coupling is what makes it checkable:
 * `tests/config/e2e-console-forwarding.test.ts` asserts the forwarder passes
 * every needle named here. Adding a browser-emitted needle without teaching the
 * forwarder is now a failing test rather than a silent zero.
 *
 * Two deliberate omissions, so neither reads as an oversight:
 *   e2eConnectLines   `[Connection]` is logged by the Playwright process itself,
 *                     not by the page, so it never touches the forwarder.
 *   transportRefused  `ECONNREFUSED` is a Node errno. A browser has no errno to
 *                     report — it produces the bare `WebSocket error` shape,
 *                     which is what `transportUnreachable` covers. Any
 *                     ECONNREFUSED in an e2e log came from a Node-side caller.
 */
export const E2E_BROWSER_NEEDLES = [
  'startScanFailed',
  'stopScanFailed',
  'transportUnreachable',
  'ackSamples',
  'linkCloses',
  'connectSamples',
];

/**
 * Which distinct campaigns does this record file contain?
 *
 * Both analysis scripts read `.suite-runs/runs.jsonl` wholesale — no argument,
 * no filter — and that file ACCUMULATES across invocations while repetition
 * numbers restart at 1 each time. So the natural state of a working bench is a
 * file holding several unrelated arms.
 *
 * On 2026-08-29 it held TRA-1193's 200 vitest rows while a 150-rep e2e arm was
 * about to be summarised. They were moved aside by hand. Nothing in the tooling
 * would have objected otherwise, and the resulting summary would have pooled two
 * runners into one failure rate and one density distribution — confidently
 * wrong, and indistinguishable on sight from a correct one.
 *
 * ⚠ This WARNS rather than filtering or aborting, and the distinction is the
 * point. Pooling is sometimes exactly what is wanted: comparing two arms is a
 * real thing to do. The defect was never that rows were mixed, it was that
 * mixing was SILENT. Filtering here would replace one silent behaviour with
 * another, and a reader who did not know rows had been dropped would be no
 * better off than one who did not know they had been pooled.
 *
 * Grouped by runner AND note, because two arms of the same runner are still two
 * arms — an instrument-validation pass and the measurement it validates share a
 * runner and must not share a denominator.
 */
export function describeCohorts(records) {
  const groups = new Map();
  for (const r of records ?? []) {
    // runnerOf, not r.runner: pre-TRA-1206 records carry no runner and ARE
    // vitest, so reading the raw field would flag every historical archive as
    // mixed against its own successors.
    const key = `${runnerOf(r)}\x00${r?.note ?? ''}`;
    const existing = groups.get(key);
    if (existing) existing.count += 1;
    else groups.set(key, { runner: runnerOf(r), note: r?.note ?? null, count: 1 });
  }
  const list = [...groups.values()];
  return { homogeneous: list.length <= 1, groups: list };
}

/**
 * The banner to print above a report drawn from a mixed record, or '' when there
 * is nothing to say.
 *
 * Empty on the common case by design: a warning that fires on every clean run is
 * one nobody reads by the time it matters.
 */
export function cohortWarning(records) {
  const { homogeneous, groups } = describeCohorts(records);
  if (homogeneous) return '';
  const rows = groups
    .map((g) => `  ${String(g.count).padStart(4)}  runner=${g.runner}  note=${g.note ?? '(none)'}`)
    .join('\n');
  return (
    `⚠️  This record mixes more than one campaign. Every rate and distribution below\n` +
    `pools all of them into one denominator, which is almost certainly not what you\n` +
    `want — reps from different runners do not measure the same thing.\n\n` +
    `${rows}\n\n` +
    `Move the rows you are not analysing out of .suite-runs/runs.jsonl first; they\n` +
    `are already archived under ~/soak-archives/ if they were worth keeping.`
  );
}

/**
 * Read-cycle VALUES, as distinct from every needle above.
 *
 * Everything in `SIGNALS` counts occurrences of a string. These are numbers
 * pulled out of one, and the difference matters for exactly one reason: a count
 * of zero means "the thing never happened", but a VALUE of zero is a
 * measurement — and here it is the most important one there is. `first == 0` is
 * TRA-1150's dominant wedge signature, 31 of its 33 wedges, a scan path that is
 * dead rather than thin. A missing value defaulted to 0 fabricates the exact
 * failure this instrument exists to detect.
 *
 * WHY THIS EXISTS. TRA-1200's arm ran against a field ~17% sparser than the
 * reference it was compared to, because the reader had been pulled back from the
 * tag stack to gun a barcode. Nothing recorded that, so it surfaced only by
 * parsing 150 logs and untarring the reference archive after the run was over.
 * The same shortfall halted Cell A on 2026-08-23 and was found the same way,
 * after the fact. Twice is a missing instrument, not bad luck.
 *
 * ⚠ UNIQUE IS THE FIELD PROXY. READS IS NOT.
 * Read volume is confounded with the variable a CPU-swap arm measures: a faster
 * host issues stop-scanning sooner, so fewer reads accumulate inside the fixed
 * `waitForTimeout(2000)` scan window (inventory.spec.ts). Two hosts facing an
 * identical pile can disagree on reads by 40%. Unique-tag count is what survives
 * that. Both are captured, but any judgement about whether the FIELD matched
 * keys on unique.
 *
 * e2e only, by structure rather than preference: `[Test] First read:` is written
 * by tests/e2e/inventory.spec.ts, and no vitest rep has an application to read
 * tags with. Same asymmetry as `appPreflight`.
 */
export const READ_CYCLE_PATTERN =
  /\[Test\] (First|Second) read: (\d+) reads, (\d+) unique tags/g;

/** The keys `readReadCycles` reports, in report order. */
export const READ_CYCLE_FIELDS = ['firstReads', 'firstUnique', 'secondReads', 'secondUnique'];

const noReadCycles = () => Object.fromEntries(READ_CYCLE_FIELDS.map((k) => [k, null]));

/**
 * Extract the read-cycle values from a captured e2e run log.
 *
 * Returns every field explicitly, each a number or `null`. Never omits a key,
 * never substitutes 0.
 *
 * A rep that died before its second cycle legitimately has `secondReads: null`
 * while `firstReads` holds a real number. That asymmetry is data — it locates
 * how far the rep got.
 */
export function readReadCycles(logPath, runner = 'vitest') {
  if (runner !== 'e2e') return noReadCycles();
  if (!logPath || !existsSync(logPath)) return noReadCycles();
  let text;
  try {
    text = readFileSync(logPath, 'utf8');
  } catch {
    return noReadCycles();
  }
  const out = noReadCycles();
  // Fresh regex per call: READ_CYCLE_PATTERN is /g and therefore stateful, so a
  // shared lastIndex would make the second call in a process skip its first
  // match. The count-by-split needles above have no such hazard, which is why
  // this is the only place it needs saying.
  for (const m of text.matchAll(new RegExp(READ_CYCLE_PATTERN.source, 'g'))) {
    const prefix = m[1] === 'First' ? 'first' : 'second';
    out[`${prefix}Reads`] = Number(m[2]);
    out[`${prefix}Unique`] = Number(m[3]);
  }
  return out;
}

/**
 * Read-cycle values for a record, recomputed from its retained log when the
 * record predates this instrument.
 *
 * Mirrors `resolveSignals` and for the same reason: every record written before
 * this change lacks these fields, and "absent" must not read as "measured zero".
 * Recomputing means archived runs — TRA-1200's own 150 reps included — become
 * analysable for density without being re-run.
 *
 * `source` is part of the answer, not decoration. A caller that cannot tell a
 * recorded value from a reconstructed one cannot tell which runs had the
 * instrument at all.
 */
export function resolveReadCycles(record) {
  const stored = record?.readCycles;
  if (stored && READ_CYCLE_FIELDS.every((k) => stored[k] !== undefined)) {
    return { readCycles: stored, source: 'record' };
  }
  const log = record?.outputLog ?? record?.stdoutLog;
  if (!log || !existsSync(log)) {
    return { readCycles: stored ?? noReadCycles(), source: 'unverifiable' };
  }
  return { readCycles: readReadCycles(log, runnerOf(record)), source: 'recomputed' };
}

/**
 * The needle that answers "did this rep produce ANY observable output", per runner.
 *
 * `harnessLines` is named for the STRING. Its two consumers — the watchdog's
 * void-capture abort and the summariser's usable-record filter — want the ROLE,
 * and the string filling that role differs per runner. Reading the vitest needle
 * on an e2e record is how a working check becomes a silent no-op: `harnessLines`
 * is null there, `null ?? 1` is 1, and the abort never fires again.
 */
const CAPTURE_CANARY = {
  vitest: 'harnessLines',
  e2e: 'e2eConnectLines',
};

const SIGNAL_TABLES = { vitest: SIGNALS, e2e: E2E_SIGNALS };

/** The needle table for a runner. Throws rather than defaulting, because a
 * typo'd runner silently measured against the wrong table is the whole failure
 * class this module exists inside. */
export function signalsFor(runner) {
  const table = SIGNAL_TABLES[runner];
  if (!table) {
    throw new Error(
      `Unknown runner: ${runner}. Expected one of ${Object.keys(SIGNAL_TABLES).join('|')}.`
    );
  }
  return table;
}

/**
 * Which runner produced a record.
 *
 * Absent means vitest, and that is a fact about the archive rather than a
 * convenience: every record written before TRA-1206 was a vitest run, so there
 * is no historical row the default can be wrong about. Reading an old record as
 * anything else would recompute its signals against a table it was never
 * measured under.
 */
export function runnerOf(record) {
  return record?.runner ?? 'vitest';
}

/**
 * Count each signature in a captured run log.
 *
 * Returns `{ logMissing: true }` when the log is gone — distinct from a log
 * that exists and contains nothing, which is what the canary catches.
 *
 * `runner` defaults to vitest so every pre-TRA-1206 call site is unchanged, and
 * the vitest result is byte-identical to what it always was — no e2e key is
 * added to it. The asymmetry is deliberate: a vitest record must stay comparable
 * against TRA-1189's 528 reps and TRA-1193's 200, and an e2e-only field on it
 * would be an absence dressed as data in the other direction.
 *
 * An e2e result DOES carry the vitest-only needles, explicitly `null`. A
 * consumer reading `signals.harnessLines` on an e2e record has to deal with
 * "unavailable"; it must never be handed a `0` that reads as "measured, and it
 * never happened".
 */
export function readSignals(logPath, runner = 'vitest') {
  const table = signalsFor(runner);
  if (!logPath || !existsSync(logPath)) return { logMissing: true };
  let text;
  try {
    text = readFileSync(logPath, 'utf8');
  } catch {
    return { logMissing: true };
  }
  const counts = { logMissing: false };
  for (const [name, needle] of Object.entries(table)) {
    counts[name] = text.split(needle).length - 1;
  }
  // Structurally absent, stated. Only ever widens a NON-vitest runner's record.
  for (const name of Object.keys(SIGNALS)) {
    if (!(name in counts)) counts[name] = null;
  }
  // Per-op command timeouts (TRA-1226). null on a runner that cannot observe
  // them, for the same reason as every other absent needle above: `{}` there
  // would read as "the device answered everything", which is a claim this
  // record has no standing to make.
  counts.commandTimeouts = runner === 'vitest' ? countCommandTimeouts(text) : null;
  // `0xA101` fault frames (TRA-1229). Same null-on-a-blind-runner rule: a 0
  // here means the device raised no rejections, and that is only sayable by a
  // runner that could have seen them.
  counts.errorNotifications = runner === 'vitest' ? countErrorNotifications(text) : null;
  // Per-op REFUSALS (TRA-1230). Counted apart from timeouts because they are
  // different device behaviours and because the timeout needle cannot see them.
  counts.commandRejections = runner === 'vitest' ? countCommandRejections(text) : null;
  // What the reader said it was (TRA-1232). Not gated on the runner, because
  // `readReaderDetails` already answers null when the line is absent and that
  // is the honest reading on either path.
  //
  // ⚠ In practice a vitest rep gets a value and an e2e rep will read null. The
  // line is a `logger.info` from the worker, which under e2e means a real Web
  // Worker's console, and it would have to survive BOTH Playwright's handling
  // of worker console messages and `shouldForwardConsoleLine` — whose KEEP list
  // contains no substring of it. Neither of those has been checked on a
  // browser, so no limb has been added to the forwarder on the strength of
  // guessing: a filter widened for a line that never arrives is a change that
  // measures nothing and looks like coverage. Check it on preview before
  // claiming an e2e arm attributes itself.
  counts.readerDetails = readReaderDetails(logPath);
  return counts;
}

/**
 * How many capture-canary lines a record saw — or null when that is unknowable.
 *
 * null means "no answer", and every caller must treat it as such rather than as
 * a zero or as health. A record whose log went missing did not measure a clean
 * capture; it measured nothing, and the honest reading of nothing is null.
 */
export function captureCanaryCount(record, signals = record?.signals) {
  if (!signals || signals.logMissing) return null;
  const value = signals[CAPTURE_CANARY[runnerOf(record)]];
  return typeof value === 'number' ? value : null;
}

/**
 * Signals for a record, recomputed from its retained log when the record itself
 * predates a signature.
 *
 * The log is the evidence; the record's `signals` are a snapshot taken at write
 * time. When a new needle is added later, every existing record is missing it —
 * and treating "field absent" as "count zero" would silently answer a question
 * the run never asked. Recomputing keeps old runs analysable without rewriting
 * the record, which stays append-only.
 */
export function resolveSignals(record) {
  const stored = record.signals;
  // Every CURRENT needle must be present, not one nominated field. The gate used
  // to name `harnessLines` alone, which meant the next needle added would find
  // it satisfied and return the stored snapshot — a record silently missing the
  // very signal the new needle was added to detect, reported as a zero. That is
  // the failure this function's docstring exists to prevent, so it cannot be
  // keyed on a single field that happens to be current today.
  // `commandTimeouts` is checked explicitly because it is NOT a member of
  // SIGNALS — it is a parsed map, not a needle, so the loop above cannot see it.
  // Without this clause a pre-TRA-1226 record passes the gate and is returned
  // with no per-op breakdown at all, on a run whose log is sitting right there:
  // precisely the "silently missing the very signal the new needle was added to
  // detect" failure this comment block was written about the last time.
  //
  // ⚠ VITEST ONLY, and that restriction is load-bearing rather than tidy.
  // Recomputation below calls `readSignals(log)` with no runner, i.e. against
  // the VITEST table. For an e2e record that turns every vitest-only null into a
  // 0 — the null-vs-zero conflation this module exists to prevent. So an e2e
  // record must not be made stale by a field it can never carry a value for.
  // Caught by `soak-driver-runner-backends.test.ts`, which was written for
  // exactly this hazard the last time someone widened the gate.
  const needsCommandTimeouts = runnerOf(record) === 'vitest';
  const isCurrent =
    stored &&
    !stored.logMissing &&
    Object.keys(SIGNALS).every((name) => stored[name] !== undefined) &&
    (!needsCommandTimeouts || stored.commandTimeouts !== undefined);
  if (isCurrent) {
    return { signals: stored, source: 'record' };
  }
  const log = record.outputLog ?? record.stdoutLog;
  const fresh = readSignals(log);
  if (fresh.logMissing) {
    return { signals: stored ?? fresh, source: 'unverifiable' };
  }
  return { signals: fresh, source: 'recomputed' };
}
