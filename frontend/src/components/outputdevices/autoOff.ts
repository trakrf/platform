/**
 * Auto-off rules for output devices, mirroring the backend's write-path checks.
 *
 * An egress output only ever sends ON; its OFF edge is the device-side auto-off
 * timer. So an egress output with no auto-off — here or from the org default —
 * turns on at its first read and never turns off. The backend refuses that, and
 * so does the form. No on-time may exceed the reader's 60 s ceiling: alarm
 * outputs fail toward off.
 */

export const MAX_AUTO_OFF_SECONDS = 60;

const RANGE = `between 1 and ${MAX_AUTO_OFF_SECONDS} seconds`;

/**
 * Validates the Auto-off field of an egress output. orgAutoOff is the org
 * default (null/undefined/0 when it supplies none, which makes the field
 * required).
 */
export function validateAutoOff(raw: string, orgAutoOff: number | null | undefined): string | null {
  const t = raw.trim();
  if (t === '') {
    return orgAutoOff && orgAutoOff > 0
      ? null
      : `Required for egress (${RANGE}). Without it the output turns on and never turns off.`;
  }
  if (!/^\d+$/.test(t)) return 'Must be a whole number of seconds';
  const n = Number(t);
  if (n < 1 || n > MAX_AUTO_OFF_SECONDS) return `Must be ${RANGE}`;
  return null;
}

/** Validates the org-default Auto-off. Blank (null) is allowed. */
export function validateOrgAutoOff(n: number | null): string | null {
  if (n === null) return null;
  if (n < 0 || n > MAX_AUTO_OFF_SECONDS) return `Auto-off must be between 0 and ${MAX_AUTO_OFF_SECONDS} seconds.`;
  return null;
}
