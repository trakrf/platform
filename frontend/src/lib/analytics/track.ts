import * as Sentry from '@sentry/react';
import { orEvent } from '@/lib/openreplay';

/**
 * Thin funnel-event passthrough (TRA-948). Forwards to the already-wired
 * OpenReplay tracker (no-op when disabled) and drops a Sentry breadcrumb so the
 * event is visible in session/error context. Never throws.
 */
export function trackEvent(name: string, props: Record<string, unknown> = {}): void {
  orEvent(name, props);
  try {
    Sentry.addBreadcrumb({ category: 'funnel', message: name, level: 'info', data: props });
  } catch {
    /* breadcrumb best-effort */
  }
}
