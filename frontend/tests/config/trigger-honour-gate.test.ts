import { describe, it, expect } from 'vitest';
import {
  waitForReaderToAcceptTrigger,
  simulateTriggerPress,
  simulateTriggerRelease,
  waitForTriggerReset,
  STATE_THAT_HONOURS,
  TRIGGER_CONFIRMATION_TIMEOUT_MS,
  TRIGGER_HONOUR_TIMEOUT_MS,
} from '../e2e/helpers/trigger-utils';

/**
 * TRA-1245. `reader.ts:229` acts on a trigger edge in exactly one state per
 * action and logs-and-drops it in every other:
 *
 *     if (this.readerState === ReaderState.CONNECTED) { startScanning() }
 *     else { logger.debug(`Trigger pressed ignored - reader state is ...`) }
 *
 * A real finger survives that drop, because the trigger LEVEL stays asserted
 * and `convergeToTriggerState()` reconciles it when the reader settles. An
 * injected press does not survive a MODE CHANGE: every mode carries
 * `IDLE_SEQUENCE`, whose `GET_TRIGGER_STATE` poll the device answers in ~22ms
 * with the real switch position, and that answer overwrites the latch. So by
 * the time the reader settles there is no held trigger left to reconcile and
 * the scan never starts. Measured at 48/200 reps (24.0%) on 2026-09-02.
 *
 * See ADR 0016 and `tests/e2e/trigger-level-is-reread-on-mode-change.spec.ts`.
 *
 * So the helper must not inject into a state that will drop the edge. These
 * tests live in `tests/config/` rather than beside the helper because
 * `vitest.config.ts` excludes `**\/tests\/e2e\/**` — a test next to the helper
 * would never run, which is the failure mode this ticket is already about.
 */

/**
 * Confirmation budget for the tests that only care about the GATE.
 *
 * A scripted page has no store to move, so the confirmation after an injection
 * can never succeed and always runs its budget out. The real budget is 5s
 * (`TRIGGER_CONFIRMATION_TIMEOUT_MS`), and the scripted `waitForTimeout` is a
 * no-op, so leaving it at the default spends five seconds of tight polling per
 * test to prove something these tests are not asking about. The tests that ARE
 * about the budget name it explicitly.
 */
const CONFIRM_BUDGET_MS = 50;

type ScriptedPage = {
  evaluate: (fn: unknown) => Promise<unknown>;
  waitForTimeout: (ms: number) => Promise<void>;
  statesRead: number;
};

/**
 * A `Page` that reports a scripted sequence of reader states.
 *
 * `getReaderState()` reaches the store through `page.evaluate()`, so returning
 * the next scripted state from `evaluate` is enough to drive the helper without
 * a browser. The last state repeats once the script runs out, which is what
 * makes the timeout case expressible.
 */
function scriptedPage(states: string[]): ScriptedPage {
  const remaining = [...states];
  let last = states[0];
  return {
    statesRead: 0,
    async evaluate(): Promise<unknown> {
      // eslint-disable-next-line @typescript-eslint/no-this-alias
      const self = this as unknown as ScriptedPage;
      self.statesRead += 1;
      if (remaining.length > 0) last = remaining.shift() as string;
      return last;
    },
    async waitForTimeout(): Promise<void> {
      // Poll delay is irrelevant off-browser; the helper's own deadline ends it.
    },
  };
}

describe('the state each trigger action is honoured in', () => {
  it('names Connected for a press and Scanning for a release', () => {
    // Mirrors reader.ts:229 and its release branch. If the product ever moves
    // these, this is the line that should go red rather than a 24% flake.
    expect(STATE_THAT_HONOURS.press).toBe('Connected');
    expect(STATE_THAT_HONOURS.release).toBe('Scanning');
  });
});

describe('waitForReaderToAcceptTrigger', () => {
  it('returns once a press would be honoured', async () => {
    const page = scriptedPage(['Busy', 'Busy', 'Connected']);
    await expect(
      waitForReaderToAcceptTrigger(page as never, 'press', 2000)
    ).resolves.toBeUndefined();
    expect(page.statesRead).toBeGreaterThanOrEqual(3);
  });

  it('returns once a release would be honoured', async () => {
    const page = scriptedPage(['Scanning']);
    await expect(
      waitForReaderToAcceptTrigger(page as never, 'release', 2000)
    ).resolves.toBeUndefined();
  });

  it('returns immediately when the reader already accepts the action', async () => {
    const page = scriptedPage(['Connected']);
    await waitForReaderToAcceptTrigger(page as never, 'press', 2000);
    expect(page.statesRead).toBe(1);
  });

  it('throws rather than injecting into a state that drops the edge', async () => {
    // The whole point. Before this ticket the helper injected anyway and the
    // failure surfaced later as "gauge should report dBm", which reads as a
    // broken product rather than a press delivered into the wrong state.
    const page = scriptedPage(['Busy']);
    await expect(
      waitForReaderToAcceptTrigger(page as never, 'press', 50)
    ).rejects.toThrow(/press/i);
  });

  it('names the observed state in the timeout, not just the wanted one', async () => {
    // A message that says only "expected Connected" sends the next person to
    // look at the trigger. Saying "was Busy" sends them to what actually held.
    const page = scriptedPage(['Busy']);
    await expect(
      waitForReaderToAcceptTrigger(page as never, 'press', 50)
    ).rejects.toThrow(/Busy/);
  });
});

/**
 * A `Page` that records whether a packet was injected, and what the reader
 * state was at the moment it happened.
 *
 * `injectTriggerPacket` is the only caller that passes ARGUMENTS to
 * `page.evaluate`; the state reads pass a function alone. That arity is what
 * separates them here, without having to imitate the browser.
 */
function recordingPage(states: string[]) {
  const remaining = [...states];
  let last = states[0];
  const injectedAt: string[] = [];
  const page = {
    injectedAt,
    statesRead: 0,
    // Reads taken up to the FIRST injection. Counting every read instead would
    // measure the store-confirmation spin that follows it, which polls a fake
    // store for its whole budget and drowns the number this is asking about.
    statesReadBeforeInjection: -1,
    async evaluate(_fn: unknown, args?: unknown): Promise<unknown> {
      if (args !== undefined) {
        injectedAt.push(last);
        if (page.statesReadBeforeInjection < 0) {
          page.statesReadBeforeInjection = page.statesRead;
        }
        return { success: true, message: 'INJECTED', eventReceived: true };
      }
      page.statesRead += 1;
      if (remaining.length > 0) last = remaining.shift() as string;
      return last;
    },
    async waitForTimeout(): Promise<void> {},
  };
  return page;
}

describe('simulateTriggerPress refuses to inject into a dropping state', () => {
  it('does not inject at all while the reader would drop the edge', async () => {
    // Before TRA-1245 this injected immediately and returned success, because
    // success meant "the trigger state updated", never "the press was acted on".
    const page = recordingPage(['Busy']);
    await expect(
      simulateTriggerPress(page as never, 1, 50)
    ).rejects.toThrow(/TRIGGER_NOT_HONOURABLE/);
    expect(page.injectedAt).toEqual([]);
  });

  it('injects only once the reader has reached the honouring state', async () => {
    const page = recordingPage(['Busy', 'Busy', 'Connected']);
    await simulateTriggerPress(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {
      // The trigger-state confirmation cannot succeed against a fake store;
      // ordering is the whole assertion here.
    });
    expect(page.injectedAt).toEqual(['Connected']);
  });
});

/**
 * The release half of the gate, which #647 got wrong and this ticket corrects.
 *
 * A press has exactly one honouring state and no way to recover a dropped edge,
 * so waiting for CONNECTED is the only sound thing to do. A release does not
 * share that shape: when no scan is running there is nothing for the worker to
 * act on, the edge is a no-op by design, and waiting for SCANNING waits for a
 * state that will never arrive.
 *
 * #647 waited anyway. `connection.spec.ts:130` and `inventory.spec.ts:111` were
 * unsatisfiable by construction and failed 2/2 reps, and `locate.spec.ts`'s
 * teardown burned 15s and threw into a catch on 101 of 101 reps — green, and
 * defective throughout.
 */
describe('a release is gated only when there is a scan to stop', () => {
  it('injects immediately when the reader is Connected and not scanning', async () => {
    // connection.spec.ts:130 and inventory.spec.ts:111. The reader is Connected
    // for the whole test by design; SCANNING is unreachable.
    const page = recordingPage(['Connected', 'Connected']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {
      // The store confirmation cannot succeed against a fake store; what is
      // under test is that the injection happened at all.
    });
    expect(page.injectedAt).toEqual(['Connected']);
  });

  it('injects when the reader is Disconnected', async () => {
    // locate.spec.ts's afterAll, after goto('/') tore the transport down. This
    // is the 101/101 swallowed throw.
    const page = recordingPage(['Disconnected', 'Disconnected']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {});
    expect(page.injectedAt).toEqual(['Disconnected']);
  });

  it('does not spend the budget polling when no scan is running', async () => {
    // The cost half of the defect: 15s burned per hit, ~6 hits per locate rep.
    // One read to answer "is a scan running", and no polling after it.
    const page = recordingPage(['Connected', 'Connected']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {});
    // One read for the helper's own initial trigger state, one for the gate.
    // The lower bound matters as much as the upper one: a release that threw
    // before injecting never sets this at all, and would sail past a bare
    // "at most 2".
    expect(page.statesReadBeforeInjection).toBeGreaterThanOrEqual(1);
    expect(page.statesReadBeforeInjection).toBeLessThanOrEqual(2);
  });

  it('still injects while the reader is Scanning', async () => {
    const page = recordingPage(['Scanning', 'Scanning']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {});
    expect(page.injectedAt).toEqual(['Scanning']);
  });

  it('waits out a transient state before deciding whether a scan is running', async () => {
    // BUSY is the one ambiguous answer: the reader may be on its way into
    // SCANNING or on its way out of it, and injecting before it settles is the
    // press bug wearing a different hat. Settle first, then decide.
    const page = recordingPage(['Busy', 'Busy', 'Busy', 'Connected']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {});
    expect(page.injectedAt).toEqual(['Connected']);
  });

  it('injects once a transient state settles into Scanning', async () => {
    const page = recordingPage(['Busy', 'Busy', 'Scanning']);
    await simulateTriggerRelease(page as never, 1, 2000, CONFIRM_BUDGET_MS).catch(() => {});
    expect(page.injectedAt).toEqual(['Scanning']);
  });

  it('throws when the reader never leaves a transient state', async () => {
    // A reader that is BUSY for the whole budget is wedged, not merely busy —
    // the same reading reader.ts takes of its own settle timeout. That is the
    // wedge this timeout was always for, and it is the one release case that
    // should still be loud.
    const page = recordingPage(['Busy']);
    await expect(
      simulateTriggerRelease(page as never, 1, 50)
    ).rejects.toThrow(/TRIGGER_NOT_HONOURABLE/);
    expect(page.injectedAt).toEqual([]);
  });

  it('names the observed state when it refuses a release', async () => {
    const page = recordingPage(['Busy']);
    await expect(
      simulateTriggerRelease(page as never, 1, 50)
    ).rejects.toThrow(/Busy/);
  });
});

describe('a press is gated exactly as before', () => {
  it('still refuses to inject while the reader is Connecting', async () => {
    // The press gate is what drove locate.spec.ts from 24.0% to 0/101 and is
    // deliberately untouched: every state but CONNECTED drops the edge.
    const page = recordingPage(['Connecting']);
    await expect(
      simulateTriggerPress(page as never, 1, 50)
    ).rejects.toThrow(/TRIGGER_NOT_HONOURABLE/);
    expect(page.injectedAt).toEqual([]);
  });

  it('still refuses to inject while the reader is Scanning', async () => {
    const page = recordingPage(['Scanning']);
    await expect(
      simulateTriggerPress(page as never, 1, 50)
    ).rejects.toThrow(/TRIGGER_NOT_HONOURABLE/);
    expect(page.injectedAt).toEqual([]);
  });
});

describe('waitForReaderToAcceptTrigger, release side', () => {
  it('returns without polling when the reader is not scanning', async () => {
    const page = scriptedPage(['Connected']);
    await waitForReaderToAcceptTrigger(page as never, 'release', 2000);
    expect(page.statesRead).toBe(1);
  });

  it('returns when the reader is disconnected', async () => {
    const page = scriptedPage(['Disconnected']);
    await expect(
      waitForReaderToAcceptTrigger(page as never, 'release', 2000)
    ).resolves.toBeUndefined();
  });
});

/**
 * `waitForTriggerReset` compared the reader state against `ReaderState.IDLE`.
 *
 * **There is no `IDLE` in `ReaderState`.** The members are `Disconnected`,
 * `Connecting`, `Configuring`, `Connected`, `Busy`, `Scanning`, `Error` —
 * `IDLE` belongs to `ReaderMode`, a different enum read from a different store
 * field. So the comparison was `readerState === undefined`, which is never true
 * for a connected reader: the helper could only ever burn its full timeout and
 * return false, no matter how completely the trigger had reset.
 *
 * The resting state it meant is `CONNECTED` — `reader.ts` documents it as
 * "Connected and idle, ready for operations", which is the "idle" the original
 * author was reaching for.
 *
 * It had no callers when this was found, so nothing was failing — it was a trap
 * armed for the next caller. Same shape as `locate.spec.ts`'s `'SCANNING'` vs
 * `'Scanning'` comparison: a condition that cannot be satisfied, in a helper
 * whose whole job is to wait for it. TRA-1245.
 */
function resetPage(states: Array<{ triggerState: boolean; readerState: string; inventoryRunning: boolean }>) {
  const remaining = [...states];
  let last = states[0];
  return {
    async evaluate(): Promise<unknown> {
      if (remaining.length > 0) last = remaining.shift()!;
      return last;
    },
    async waitForTimeout(): Promise<void> {},
  };
}

describe('waitForTriggerReset', () => {
  it('reports a reset once the reader is back at Connected', async () => {
    const page = resetPage([
      { triggerState: true, readerState: 'Scanning', inventoryRunning: true },
      { triggerState: false, readerState: 'Busy', inventoryRunning: false },
      { triggerState: false, readerState: 'Connected', inventoryRunning: false },
    ]);
    await expect(waitForTriggerReset(page as never, 2000)).resolves.toBe(true);
  });

  it('does not report a reset while a scan is still running', async () => {
    const page = resetPage([
      { triggerState: true, readerState: 'Scanning', inventoryRunning: true },
    ]);
    await expect(waitForTriggerReset(page as never, 50)).resolves.toBe(false);
  });

  it('does not report a reset while the trigger is still held', async () => {
    const page = resetPage([
      { triggerState: true, readerState: 'Connected', inventoryRunning: false },
    ]);
    await expect(waitForTriggerReset(page as never, 50)).resolves.toBe(false);
  });
});

/**
 * TRA-1247. The confirmation that follows an injection used to run a fixed
 * 500ms wall and then report `STATE_NOT_UPDATED` — naming the store, when the
 * thing that had not finished was usually the reader's bring-up.
 *
 * The discriminator that decided this shape is answered in
 * `src/worker/cs108/reader.test.ts` under "a trigger notification arriving
 * while BUSY": the level IS latched while the reader is BUSY, so the press is
 * never lost, and what a confirmation can be short of is the convergence that
 * runs when bring-up ends. 500ms could not cover that — the Locate mask write
 * alone measured ~3.7s (TRA-1225).
 *
 * A timeout that misreports what it measured is what kept TRA-1080's follow-up
 * looking at the trigger path for a month.
 */
describe('the confirmation timeout names what actually ran out', () => {
  it('is 5s, not the 15s the honour gate uses', () => {
    // The bound carries a product requirement: a config sequence taking five
    // seconds is a defect worth surfacing, and a 15s bound would absorb it.
    expect(TRIGGER_CONFIRMATION_TIMEOUT_MS).toBe(5000);
    expect(TRIGGER_CONFIRMATION_TIMEOUT_MS).toBeLessThan(TRIGGER_HONOUR_TIMEOUT_MS);
  });

  it('blames bring-up when the reader is still transient', async () => {
    // One read for the helper's own initial trigger state, one for the gate,
    // and from there the reader is Busy running its config sequence — the
    // shape this ticket is about.
    const page = recordingPage(['Connected', 'Connected', 'Busy']);
    const result = await simulateTriggerPress(page as never, 1, 2000, CONFIRM_BUDGET_MS);

    expect(page.injectedAt).toEqual(['Connected']);
    expect(result.success).toBe(false);
    expect(result.message).toMatch(/BRING_UP_INCOMPLETE/);
    expect(result.message).toMatch(/Busy/);
    expect(result.message).toMatch(new RegExp(`${CONFIRM_BUDGET_MS}ms`));
  });

  it('blames the store only when the reader has settled', async () => {
    // Reader settled and the level still not reflected: now it really is the
    // path between the worker handler and deviceStore.
    const page = recordingPage(['Connected']);
    const result = await simulateTriggerPress(page as never, 1, 2000, CONFIRM_BUDGET_MS);

    expect(result.success).toBe(false);
    expect(result.message).toMatch(/STATE_NOT_UPDATED/);
    expect(result.message).not.toMatch(/BRING_UP_INCOMPLETE/);
  });
});
