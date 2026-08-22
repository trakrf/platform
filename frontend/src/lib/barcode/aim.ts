/**
 * AIM symbology identifiers, as prefixed by barcode scanners.
 *
 * Lives apart from useScanToInput so consumers that read barcodeStore directly
 * — the Locate screen takes trigger-fired reads that never open a hook session
 * (TRA-1121) — can clean a read without pulling in the hook.
 */

/**
 * Strip AIM symbology identifiers from barcode data.
 * AIM IDs follow the pattern: ]<symbology><modifier> (e.g., ]C1 for Code 128, ]Q1 for QR)
 * Some scanners prepend symbology char before AIM ID (e.g., Q]Q1...)
 *
 * Examples:
 *   "Q]Q1000000000000000000000130" -> "000000000000000000000130"
 *   "]C1E200123456789" -> "E200123456789"
 */
export function stripAimIdentifier(data: string): string {
  // Match: optional char + ] + letter + digit at start
  const match = data.match(/^(.?\][A-Za-z]\d)(.*)$/);
  if (match) {
    const [, prefix, rest] = match;
    console.debug('[stripAimIdentifier]', { prefix, rest, restLength: rest.length });
    return rest;
  }
  // No AIM prefix found, return as-is
  return data;
}
