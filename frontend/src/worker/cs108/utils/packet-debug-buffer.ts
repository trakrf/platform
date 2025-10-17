/**
 * Ring buffer for debugging BLE packet sequences
 * Captures recent packets to help diagnose reassembly issues
 */
export class PacketDebugBuffer {
  private buffer: { timestamp: number; hex: string; size: number }[] = [];
  private readonly maxSize = 8;

  /**
   * Add a packet to the ring buffer
   */
  add(data: Uint8Array): void {
    const entry = {
      timestamp: Date.now(),
      hex: Array.from(data)
        .map(b => `0x${b.toString(16).padStart(2, '0').toUpperCase()}`)
        .join(' '),
      size: data.length
    };

    this.buffer.push(entry);
    if (this.buffer.length > this.maxSize) {
      this.buffer.shift(); // Remove oldest entry
    }
  }

  /**
   * Get recent packets formatted for debugging
   */
  getRecentPackets(): string[] {
    return this.buffer.map((entry, index) => {
      const age = Date.now() - entry.timestamp;
      return `[${index}] ${age}ms ago (${entry.size}B): ${entry.hex}`;
    });
  }

  /**
   * Clear the buffer
   */
  clear(): void {
    this.buffer = [];
  }

  /**
   * Get formatted report for error logging
   */
  getErrorReport(errorContext: string): string {
    const lines = [
      `\n🔴 PACKET REASSEMBLY ERROR: ${errorContext}`,
      `📦 Recent BLE packets (${this.buffer.length} captured):`
    ];

    this.buffer.forEach((entry, index) => {
      const age = Date.now() - entry.timestamp;
      const isLast = index === this.buffer.length - 1;
      const marker = isLast ? '>>> ' : '    ';
      lines.push(`${marker}[${index}] ${age}ms ago (${entry.size}B): ${entry.hex}`);
    });

    lines.push('');
    return lines.join('\n');
  }
}