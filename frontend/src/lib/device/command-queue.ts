/**
 * Serializes commands sent to the reader worker.
 *
 * ⚠ THE ORIGINAL JUSTIFICATION HAS EXPIRED. This used to read: the worker's
 * CommandManager is not re-entrant, a second executeCommand while one is in
 * flight throws "Command already active", and DeviceManager drives the worker
 * from four independent store subscriptions (uiStore, kitStore, settingsStore,
 * scanButton) any two of which could put two commands in flight at once — the
 * loser simply dropped, a mode change nobody reapplied (TRA-1121).
 *
 * TRA-1197 made CommandManager queue instead of throw, so the worker no longer
 * drops the loser. What remains true here is narrower and worth stating rather
 * than inheriting: this queue decides the ORDER the four subscriptions reach the
 * worker in, on the main-thread side of the Comlink boundary, and it gives each
 * caller its own result.
 *
 * ⚠ Whether that is still worth a second queue is an OPEN QUESTION, raised on
 * TRA-1197 rather than answered here — removing it is a behavioural change with
 * its own risk and no test that would catch the difference. Do not delete it on
 * the strength of "the worker queues now"; that is the premise, not the answer.
 *
 * Ordering is FIFO, and a failed command is reported to its own caller without
 * stopping the ones behind it — a poisoned chain would be worse than the race,
 * because one failure would silently mute the reader for the rest of the session.
 */
export class CommandQueue {
  private tail: Promise<unknown> = Promise.resolve();

  run<T>(command: () => Promise<T>): Promise<T> {
    // Chain off the tail whether or not it settled cleanly, so one rejection
    // cannot strand everything queued behind it.
    const result = this.tail.then(command, command);
    this.tail = result.catch(() => undefined);
    return result;
  }
}
