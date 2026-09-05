/**
 * Byte-array to hex-string conversion.
 *
 * Pulled out of three inlined copies — two in the RFID inventory parser
 * (compact and normal mode tag extraction) and one in the barcode scan handler.
 * The barcode copy emitted lowercase while the parser copies emitted uppercase;
 * they are unified on uppercase here, which is the form every tag identifier in
 * this codebase is stored, compared and displayed in.
 */

/**
 * Render bytes as contiguous uppercase hex, two characters per byte.
 *
 * The zero-padding is the part that matters. Without it a byte below 0x10 emits
 * a single character, so `0F 0E` becomes "fe" — shorter than the input, still
 * valid-looking hex, and wrong in a way nothing downstream could detect.
 */
export function toHex(bytes: Uint8Array): string {
  return Array.from(bytes)
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
    .toUpperCase();
}
