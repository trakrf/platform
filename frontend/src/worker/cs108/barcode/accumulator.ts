/**
 * Byte-stream accumulator for CS108 barcode notifications.
 *
 * The CS108 firmware delivers a barcode read as one or more 0x9100 BARCODE_DATA
 * notifications. Each notification's payload contributes bytes to a stream
 * terminated by 0x0D (CR). A single payload may contain multiple complete
 * records, or a record may span multiple payloads.
 *
 * See `docs/superpowers/specs/2026-05-23-tra-821-barcode-fragment-reassembly-design.md`
 * for the empirical firmware behaviors this class handles.
 */

const RECORD_TERMINATOR = 0x0D;
const STATUS_PING_BYTE = 0x06;
const IDLE_TIMEOUT_MS = 500;

export class BarcodeAccumulator {
  private buffer: Uint8Array = new Uint8Array(0);
  private idleTimeoutHandle: ReturnType<typeof setTimeout> | null = null;

  /**
   * Append a 0x9100 payload to the buffer and return any complete records
   * (each terminated by 0x0D, terminator included in the returned bytes).
   * Schedules an idle-timeout flush if bytes remain in the buffer.
   */
  appendAndExtract(payload: Uint8Array): Uint8Array[] {
    const combined = new Uint8Array(this.buffer.length + payload.length);
    combined.set(this.buffer);
    combined.set(payload, this.buffer.length);
    this.buffer = combined;

    const records: Uint8Array[] = [];
    let crIndex = this.buffer.indexOf(RECORD_TERMINATOR);
    while (crIndex >= 0) {
      records.push(this.buffer.slice(0, crIndex + 1));
      this.buffer = this.buffer.slice(crIndex + 1);
      crIndex = this.buffer.indexOf(RECORD_TERMINATOR);
    }
    return records;
  }

  /**
   * Reset internal state. Use on mode change, disconnect, or handler cleanup.
   */
  reset(): void {
    this.buffer = new Uint8Array(0);
    if (this.idleTimeoutHandle !== null) {
      clearTimeout(this.idleTimeoutHandle);
      this.idleTimeoutHandle = null;
    }
  }
}
