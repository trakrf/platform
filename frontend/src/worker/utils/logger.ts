/**
 * Logger utility for the worker
 * Provides configurable log levels for debugging
 */

export enum LogLevel {
  NONE = 0,
  ERROR = 1,
  WARN = 2,
  INFO = 3,
  DEBUG = 4,
  TRACE = 5
}

class WorkerLogger {
  private level: LogLevel = LogLevel.INFO;
  private prefix = '[Worker]';

  setLevel(level: LogLevel): void {
    this.level = level;
    this.info(`Log level set to ${LogLevel[level]}`);
  }

  getLevel(): LogLevel {
    return this.level;
  }

  error(message: string, ...args: any[]): void {
    if (this.level >= LogLevel.ERROR) {
      console.error(`${this.prefix} ERROR:`, message, ...args);
    }
  }

  warn(message: string, ...args: any[]): void {
    if (this.level >= LogLevel.WARN) {
      console.warn(`${this.prefix} WARN:`, message, ...args);
    }
  }

  info(message: string, ...args: any[]): void {
    if (this.level >= LogLevel.INFO) {
      console.info(`${this.prefix} INFO:`, message, ...args);
    }
  }

  debug(message: string, ...args: any[]): void {
    if (this.level >= LogLevel.DEBUG) {
      console.log(`${this.prefix} DEBUG:`, message, ...args);
    }
  }

  trace(message: string, ...args: any[]): void {
    if (this.level >= LogLevel.TRACE) {
      console.log(`${this.prefix} TRACE:`, message, ...args);
    }
  }

  hexDump(label: string, data: Uint8Array): void {
    if (this.level >= LogLevel.DEBUG) {
      const hex = Array.from(data)
        .map(b => '0x' + b.toString(16).padStart(2, '0'))
        .join(' ');
      console.log(`${this.prefix} HEX [${label}]:`, hex);
    }
  }
}

// Export singleton instance
export const logger = new WorkerLogger();

// Export for use in tests
export { WorkerLogger };