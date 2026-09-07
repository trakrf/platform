/**
 * Consolidated Console Utilities for E2E Tests
 * Combines functionality from console-monitor and console-tracker
 * Provides error monitoring, filtering, and reporting capabilities
 */

import type { Page } from '@playwright/test';

export interface ConsoleMessage {
  /**
   * Playwright's spelling, which is `warning` — NOT `warn` (TRA-1253).
   *
   * This said `'warn'` while `monitor()` assigned `msg.type()` through an
   * unchecked cast, so the value at runtime was always `'warning'` and the type
   * was simply wrong. Being wrong in a tree `tsc` never looked at, it produced
   * three separate dead comparisons rather than one error:
   *
   *   console-utils.ts:150  `msg.type() === 'warn'`  — dead limb, harmless only
   *                          because the `'warning'` limb short-circuits first
   *   console-utils.ts:217  `msg.type === 'warn'`    — ALWAYS FALSE, so
   *                          getWarningErrors() never matched on type at all
   *   assertions.ts:57      `m.type === 'warning'`   — flagged as impossible by
   *                          the compiler, and correct at runtime the whole time
   *
   * The last one is the tell: the type was the lie, not the comparison.
   */
  type: 'error' | 'warning' | 'log';
  text: string;
  timestamp: number;
  location?: string;
}

export interface ConsoleMonitorOptions {
  failOnErrors?: string[]; // Patterns that should fail the test
  warnOnErrors?: string[]; // Patterns that should warn but not fail
  allowedErrors?: string[]; // Patterns that are expected/benign
  logAllMessages?: boolean; // Whether to log all console messages
}

/**
 * Both lists are guarded, per entry, by
 * `tests/config/every-console-allowlist-entry-has-a-producer.test.ts`.
 *
 * Read that guard before adding anything here. The short version: an entry must
 * match a line something under `frontend/src/` can actually print, because that
 * is the only tree the monitored browser page loads. An entry that matches
 * nothing has no symptom — the gate stays green and nothing ever points at the
 * list — which is how 15 of the 16 entries below rotted unobserved.
 *
 * When the guard condemns an entry, DELETE it. Do not reword it to match the
 * current message: that resets the decay clock, adds no failure mode, and leaves
 * the list looking correct until it silently is not again. TRA-1224.
 */

// Critical errors that should fail tests
export const DEFAULT_CRITICAL_ERRORS = [
  'Connection error',
  // REMOVED 2026-09-06: eight entries that matched nothing under src/.
  //
  //   'Start inventory command failed'   'Failed to start inventory'
  //   'Failed to stop inventory'         'Connection timeout'
  //   'RFID Module: Error'               'Transport error'
  //   'Command timeout after'            'Device error notification'
  //
  // This was the worse half of the finding. These are the entries that FAIL a
  // run, so eight dead ones were a gate that could not fire — an e2e pass has
  // been a weaker statement than it looked for as long as they have been dead.
  //
  // They are replatform casualties rather than typos. The CS108 worker rebuild
  // and the ble-mcp-test replatform reworded the emissions out from under them;
  // `'Command timeout after'` is the clearest case, because the line it was
  // written for is now `[CommandManager] Command timeout: RFID_POWER_OFF` —
  // different prefix, different name form, and no duration at all.
  //
  // Not reworded to match, deliberately. Three of them (`Connection timeout`,
  // `Transport error`, and the timeout above) name conditions that DO still
  // occur, so the temptation to retype them is real — but a critical entry is
  // only worth having if a run can prove it still matches, and the guard now
  // demands exactly that of whatever replaces them.
];

// Known benign errors that should be ignored
export const DEFAULT_ALLOWED_ERRORS = [
  // REMOVED 2026-09-06: all seven remaining entries matched nothing under src/.
  //
  //   'Failed to start battery auto reporting'
  //   'Failed to power OFF RFID module'
  //   'No RFID packets received for'
  //   'Device error notification: code 0x0'
  //   'Command timeout after 2000ms: RFID Firmware Command (0x8002)'
  //   'Command timeout after 5000ms: RFID Power Off (0x8001)'
  //   'Command timeout after 5000ms: RFID Power On (0x8000)'
  //
  // The last three were inert twice over, and the second reason is worth keeping
  // because it is easy to re-introduce: `isAllowedError` gates the throwing path,
  // but `Command timeout` is a `logger.warn`, and warnings do not fail a run.
  // Wrong string AND nothing to suppress.
  //
  // Do not restore the Power Off entry from memory without reading TRA-1217
  // first. That ticket changed what the condition MEANS: an unanswered
  // RFID_POWER_OFF used to be "firmware quirk, benign" and is now a deliberately
  // tolerated condition that emits its own line — which TRA-1223 wants COUNTED,
  // because a recurrence of the device's silent window is otherwise invisible.
  // "Allowlist it", "count it" and "assert on it" are three different answers.
  //
  // REMOVED 2026-08-31: 'Device is busy with another session'.
  //
  // It matched nothing. Verified on both sides of the seam rather than assumed:
  // ble-mcp-test has zero occurrences in 0.16.0 outside a CHANGELOG entry and a
  // design doc attributing it to `bridge-server.ts` — the TypeScript/Noble
  // bridge, deleted in the Python replatform — and platform emits it nowhere
  // either. Dead since that replatform, not a 0.16.0 regression.
  //
  // Not replaced, and deliberately not replaced with a code match. Two reasons:
  //
  // 1. A console monitor sees RENDERED TEXT, never the thrown object, so
  //    `error.code` is unreachable from here. The code is a real contract —
  //    ble-mcp-test's wire spec says a client MUST discriminate on `code` and
  //    MUST NOT match on `error`, and it is enforced across the language
  //    boundary — but that contract is for whoever catches the error, which is
  //    the transport, not this monitor.
  // 2. There is nothing left to allow. A self-collision is now retried inside
  //    the mock (`DEVICE_BUSY_SELF`, 0.16.0) and never reaches the console as an
  //    error. What remains is a genuinely foreign holder, and that MUST fail the
  //    run rather than be waved through — allowlisting it would hide the one
  //    case the refusal exists to make loud.
  //
  // See TRA-1216.
];

export class ConsoleMonitor {
  private messages: ConsoleMessage[] = [];
  private errors: ConsoleMessage[] = [];
  private warnings: ConsoleMessage[] = [];
  private options: Required<ConsoleMonitorOptions>;
  
  constructor(options: ConsoleMonitorOptions = {}) {
    this.options = {
      failOnErrors: options.failOnErrors || DEFAULT_CRITICAL_ERRORS,
      warnOnErrors: options.warnOnErrors || [],
      allowedErrors: options.allowedErrors || DEFAULT_ALLOWED_ERRORS,
      logAllMessages: options.logAllMessages || false
    };
  }
  
  /**
   * Start monitoring console messages on a page
   */
  public monitor(page: Page): void {
    page.on('console', (msg) => {
      const message: ConsoleMessage = {
        type: msg.type() as 'error' | 'warning' | 'log',
        text: msg.text(),
        timestamp: Date.now(),
        location: msg.location()?.url
      };
      
      // Store the message
      this.messages.push(message);
      
      // Categorize by type
      if (msg.type() === 'error') {
        this.errors.push(message);
      } else if (msg.type() === 'warning') {
        this.warnings.push(message);
      }
      
      // Log if requested
      if (this.options.logAllMessages) {
        console.log(`[Console ${message.type.toUpperCase()}] ${message.text}`);
      }
    });
  }
  
  /**
   * Check for critical errors and throw if found
   */
  public checkForCriticalErrors(): void {
    const criticalErrors = this.getCriticalErrors();
    
    if (criticalErrors.length > 0) {
      const errorSummary = criticalErrors
        .map(err => `[${err.type.toUpperCase()}] ${err.text}`)
        .join('\n');
        
      throw new Error(`Critical console errors detected:\n${errorSummary}`);
    }
  }
  
  /**
   * Assert no console errors (except allowed ones)
   */
  public assertNoErrors(): void {
    const unexpectedErrors = this.errors.filter(error => 
      !this.isAllowedError(error.text)
    );
    
    if (unexpectedErrors.length > 0) {
      throw new Error(`Console errors detected:\n${unexpectedErrors.map(e => e.text).join('\n')}`);
    }
  }
  
  /**
   * Get all critical errors (should fail test)
   */
  public getCriticalErrors(): ConsoleMessage[] {
    return this.errors.filter(error => {
      // Skip if explicitly allowed
      if (this.isAllowedError(error.text)) {
        return false;
      }
      
      // Check if it matches critical error patterns
      return this.options.failOnErrors.some(pattern => 
        error.text.includes(pattern)
      );
    });
  }
  
  /**
   * Get warning errors (should warn but not fail)
   */
  public getWarningErrors(): ConsoleMessage[] {
    return this.messages.filter(msg => {
      // Skip if explicitly allowed or critical
      if (this.isAllowedError(msg.text) || this.isCriticalError(msg.text)) {
        return false;
      }
      
      // Check if it matches warning patterns or is a warning type
      return msg.type === 'warning' || 
        this.options.warnOnErrors.some(pattern => msg.text.includes(pattern));
    });
  }
  
  /**
   * Get all collected messages
   */
  public getMessages(): ConsoleMessage[] {
    return [...this.messages];
  }
  
  /**
   * Get all errors
   */
  public getErrors(): ConsoleMessage[] {
    return [...this.errors];
  }
  
  /**
   * Get all warnings
   */
  public getWarnings(): ConsoleMessage[] {
    return [...this.warnings];
  }
  
  /**
   * Clear collected messages
   */
  public clear(): void {
    this.messages = [];
    this.errors = [];
    this.warnings = [];
  }
  
  /**
   * Check if there are any errors
   */
  public hasErrors(): boolean {
    return this.errors.length > 0;
  }
  
  /**
   * Check if there are any warnings
   */
  public hasWarnings(): boolean {
    return this.warnings.length > 0;
  }
  
  /**
   * Get summary of errors for reporting
   */
  public getSummary(): { critical: number; warnings: number; total: number } {
    return {
      critical: this.getCriticalErrors().length,
      warnings: this.getWarningErrors().length,
      total: this.messages.length
    };
  }
  
  /**
   * Generate a console report
   */
  public generateReport(): string {
    const criticalErrors = this.getCriticalErrors();
    const warnings = this.getWarningErrors();
    
    let report = '';
    
    if (criticalErrors.length > 0) {
      report += `\nCritical Errors (${criticalErrors.length}):\n`;
      criticalErrors.forEach(err => {
        report += `  - ${err.text}\n`;
      });
    }
    
    if (warnings.length > 0) {
      report += `\nWarnings (${warnings.length}):\n`;
      warnings.forEach(warn => {
        report += `  - ${warn.text}\n`;
      });
    }
    
    return report || 'No console errors or warnings detected';
  }
  
  private isAllowedError(message: string): boolean {
    return this.options.allowedErrors.some(pattern => 
      message.includes(pattern)
    );
  }
  
  private isCriticalError(message: string): boolean {
    return this.options.failOnErrors.some(pattern => 
      message.includes(pattern)
    );
  }
}

/**
 * Helper function to create and setup console monitoring for a test
 */
export function setupConsoleMonitoring(
  page: Page, 
  options?: ConsoleMonitorOptions
): ConsoleMonitor {
  const monitor = new ConsoleMonitor(options);
  monitor.monitor(page);
  return monitor;
}

/**
 * Legacy helper for compatibility - use setupConsoleMonitoring instead
 * @deprecated Use setupConsoleMonitoring instead
 */
export function setupConsoleTracking(page: Page): ConsoleMonitor {
  return setupConsoleMonitoring(page);
}

/**
 * Legacy helper for report generation
 * @deprecated Use monitor.generateReport() instead
 */
export function generateConsoleReport(monitor: ConsoleMonitor): string {
  return monitor.generateReport();
}

/**
 * Legacy assertion helper
 * @deprecated Use monitor.assertNoErrors() instead
 */
export function assertNoErrors(monitor: ConsoleMonitor): void {
  monitor.assertNoErrors();
}

// Re-export types for backward compatibility
export type { ConsoleMessage as ConsoleError } from './console-utils';
export type ConsoleTracker = ConsoleMonitor;