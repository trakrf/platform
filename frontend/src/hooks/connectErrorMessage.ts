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

/**
 * @param firstConnectHint Platform-specific setup step to offer *only* when
 *   nothing above recognised the failure. Callers pass
 *   `useBluetoothSupport().setupPrerequisite?.connectHint`, which is non-null on
 *   Windows alone (TRA-1100). It rides the generic message and nothing else: the
 *   diagnosed branches below already know what went wrong, and appending a
 *   pairing suggestion to "you cancelled" or "your Bluetooth is off" is noise.
 */
export function connectErrorMessage(error: unknown, firstConnectHint?: string): string {
  const message = messageOf(error);

  // Brave ships Web Bluetooth disabled behind a brave://flags toggle. Verified
  // on a real Brave install 2026-08-04: navigator.bluetooth is present, so no
  // support banner fires, and this is the user's only signal.
  if (message.includes('globally disabled')) {
    return 'Brave blocks Web Bluetooth by default. Turn it on at brave://flags (search for Bluetooth), or use Chrome, Edge, or Opera.';
  }

  // No usable Bluetooth radio: switched off, disabled, or a driver that never
  // worked. Captured from Edge on Windows, 2026-08-04. The API is present so no
  // banner fires, and the old generic message pointed the user at the scanner
  // when the scanner was fine. Matched on the message rather than the name,
  // because a cancelled chooser is a NotFoundError too.
  if (message.includes('adapter not available')) {
    return "Bluetooth isn't available on this computer. Check that it's switched on in your system settings.";
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

  // Everything unrecognised lands here, and on Windows that includes the one
  // failure we know a specific cure for: `NetworkError: Connection attempt
  // failed.` from a scanner that has never been paired at the OS level. It is
  // also Chromium's generic GATT failure — a reader that is off, out of range,
  // or flat produces exactly the same string — so the hint is appended to a
  // message that still says it does not know, never substituted for it.
  return firstConnectHint ? `${GENERIC}. ${firstConnectHint}` : GENERIC;
}
