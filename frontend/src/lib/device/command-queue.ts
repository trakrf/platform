/**
 * Serializes commands sent to the reader worker.
 *
 * The worker's CommandManager is not re-entrant — a second executeCommand while
 * one is in flight throws "Command already active - executeCommand called
 * concurrently". DeviceManager drives the worker from four independent store
 * subscriptions (uiStore, kitStore, settingsStore, scanButton), and any two of
 * them firing close together used to put two commands in flight at once. The
 * loser was simply dropped: a mode change nobody reapplied, leaving the reader
 * doing what it had been doing before (TRA-1121).
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
