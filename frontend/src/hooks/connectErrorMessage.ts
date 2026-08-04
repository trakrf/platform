/**
 * Turn a failed `connect()` into something a user can act on.
 *
 * TRA-1078. Header and SettingsScreen each carried a byte-identical copy of
 * this ladder, which is the same duplicated-browser-support-logic problem the
 * rest of this ticket removes — it just sat where a grep for
 * `navigator.bluetooth` could not find it.
 *
 * This is the reactive half of support detection, and it exists because the
 * capability gate cannot cover everything. `useBluetoothSupport` answers
 * "does this browser expose Web Bluetooth?", which Brave passes and then fails
 * anyway. Only the attempt itself knows that, and it says so precisely — the
 * old code caught that DOMException and replaced it with "Failed to connect to
 * reader".
 *
 * Cases are added as real exceptions are observed, never invented. Anything
 * unrecognised keeps the generic message rather than guessing at a diagnosis.
 */

const GENERIC = 'Failed to connect to reader';

/**
 * Read `.message` structurally rather than gating on `instanceof Error`.
 * Everything interesting here arrives as a DOMException, and whether that is an
 * instanceof Error varies by engine — it is not one under jsdom. Matching on
 * the shape works in both, and cannot silently fall through to the generic
 * message in a browser we did not test.
 */
function messageOf(error: unknown): string {
  if (typeof error === 'object' && error !== null && 'message' in error) {
    const { message } = error as { message: unknown };
    return typeof message === 'string' ? message : '';
  }
  return '';
}

export function connectErrorMessage(error: unknown): string {
  const message = messageOf(error);

  // Brave ships Web Bluetooth disabled behind a brave://flags toggle. Verified
  // on a real Brave install 2026-08-04: navigator.bluetooth is present, so no
  // support banner fires, and this is the user's only signal.
  if (message.includes('globally disabled')) {
    return 'Brave blocks Web Bluetooth by default. Turn it on at brave://flags (search for Bluetooth), or use Chrome, Edge, or Opera.';
  }

  // Dismissing the browser's device chooser lands here too. Reporting that as a
  // failure blames the reader for something the user chose to do.
  if (message.includes('User cancelled') || message.includes('cancelled the requestDevice')) {
    return 'Connection cancelled — no reader was selected.';
  }

  if (message.includes('timeout')) {
    return 'Connection timed out. Please try again.';
  }

  if (message.includes('disconnected')) {
    return 'Reader disconnected unexpectedly';
  }

  return GENERIC;
}
