import { describe, it, expect } from 'vitest';
import { CommandQueue } from './command-queue';

/**
 * The worker's CommandManager USED to be non-re-entrant: a second executeCommand
 * while one is in flight threw "Command already active". TRA-1197 made it queue,
 * so that is no longer why this exists — see command-queue.ts for what still is.
 * The behaviour asserted below (FIFO order, no poisoned chain) is unchanged
 * either way. DeviceManager has four
 * independent store subscribers (uiStore, kitStore, settingsStore, scanButton)
 * that all issue worker commands, and nothing sequenced them — so navigating to
 * Locate and typing a target a moment later put setMode(Barcode) and
 * setMode(Locate) in flight together and the second was simply lost (TRA-1121).
 */
describe('CommandQueue', () => {
  it('never runs two commands at once', async () => {
    const queue = new CommandQueue();
    let inFlight = 0;
    let maxInFlight = 0;

    const command = async () => {
      inFlight++;
      maxInFlight = Math.max(maxInFlight, inFlight);
      await new Promise((r) => setTimeout(r, 5));
      inFlight--;
    };

    await Promise.all([queue.run(command), queue.run(command), queue.run(command)]);

    expect(maxInFlight).toBe(1);
  });

  it('runs commands in the order they were queued', async () => {
    const queue = new CommandQueue();
    const order: number[] = [];

    await Promise.all([
      queue.run(async () => { await new Promise((r) => setTimeout(r, 10)); order.push(1); }),
      queue.run(async () => { order.push(2); }),
      queue.run(async () => { order.push(3); })
    ]);

    expect(order).toEqual([1, 2, 3]);
  });

  it('returns each command its own result', async () => {
    const queue = new CommandQueue();

    const results = await Promise.all([
      queue.run(async () => 'a'),
      queue.run(async () => 'b')
    ]);

    expect(results).toEqual(['a', 'b']);
  });

  it('reports a failure to its own caller', async () => {
    const queue = new CommandQueue();

    await expect(queue.run(async () => { throw new Error('reader busy'); }))
      .rejects.toThrow('reader busy');
  });

  // A poisoned chain would be worse than the race it replaced: one failed mode
  // change would silently stop every later command.
  it('keeps running after a command fails', async () => {
    const queue = new CommandQueue();

    await queue.run(async () => { throw new Error('reader busy'); }).catch(() => { /* handled */ });

    await expect(queue.run(async () => 'still working')).resolves.toBe('still working');
  });
});
