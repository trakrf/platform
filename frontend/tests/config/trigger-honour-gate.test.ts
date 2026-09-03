import { describe, it, expect } from 'vitest';
import {
  waitForReaderToAcceptTrigger,
  STATE_THAT_HONOURS,
} from '../e2e/helpers/trigger-utils';

/**
 * TRA-1245. `reader.ts:229` acts on a trigger edge in exactly one state per
 * action and logs-and-drops it in every other:
 *
 *     if (this.readerState === ReaderState.CONNECTED) { startScanning() }
 *     else { logger.debug(`Trigger pressed ignored - reader state is ...`) }
 *
 * A real thumb survives that drop, because the trigger LEVEL stays asserted and
 * `convergeToTriggerState()` reconciles it when the reader settles. An injected
 * press cannot: the state reverts to false ~500ms later, so by the time the
 * reader settles there is no held trigger left to reconcile, and the scan never
 * starts. Measured at 48/200 reps (24.0%) on 2026-09-02.
 *
 * So the helper must not inject into a state that will drop the edge. These
 * tests live in `tests/config/` rather than beside the helper because
 * `vitest.config.ts` excludes `**\/tests\/e2e\/**` — a test next to the helper
 * would never run, which is the failure mode this ticket is already about.
 */

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
    const page = scriptedPage(['Connected', 'Scanning']);
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
