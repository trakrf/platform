/**
 * Consolidated Console Utilities for E2E Tests
 * Combines functionality from console-monitor and console-tracker
 * Provides error monitoring, filtering, and reporting capabilities
 */

import type { Page } from '@playwright/test';

export interface ConsoleMessage {
  type: 'error' | 'warn' | 'log';
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

// Critical errors that should fail tests
const DEFAULT_CRITICAL_ERRORS = [
  'Start inventory command failed',
  'Failed to start inventory', 
  'Failed to stop inventory',
  'Connection timeout',
  'Connection error',
  'RFID Module: Error',
  'Transport error',
  'Command timeout after',
  'Device error notification'
];

// Known benign errors that should be ignored
const DEFAULT_ALLOWED_ERRORS = [
  'Failed to start battery auto reporting', // Known issue - benign
  'Failed to power OFF RFID module', // Known firmware issue - benign
  'No RFID packets received for', // Auto-stop feature removed - expected
  'Device error notification: code 0x0', // End of fragmented inventory packet - normal operation
  'Command timeout after 2000ms: RFID Firmware Command (0x8002)', // Background RFID cleanup - benign
  'Command timeout after 5000ms: RFID Power Off (0x8001)', // RFID power commands - firmware quirk
  'Command timeout after 5000ms: RFID Power On (0x8000)', // RFID power commands - firmware quirk
  'Device is busy with another session', // Session handling - bridge needs cleanup between runs
];

export class ConsoleMonitor {
  private messages: ConsoleMessage[] = [];
  private errors: ConsoleMessage[] = [];
  private warnings: ConsoleMessage[] = [];
  private options: Required<ConsoleMonitorOptions>;
  private page: Page | null = null;
  
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
    this.page = page;
    
    page.on('console', (msg) => {
      const message: ConsoleMessage = {
        type: msg.type() as 'error' | 'warn' | 'log',
        text: msg.text(),
        timestamp: Date.now(),
        location: msg.location()?.url
      };
      
      // Store the message
      this.messages.push(message);
      
      // Categorize by type
      if (msg.type() === 'error') {
        this.errors.push(message);
      } else if (msg.type() === 'warning' || msg.type() === 'warn') {
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
      return msg.type === 'warn' || 
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