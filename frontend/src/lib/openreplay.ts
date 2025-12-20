/**
 * OpenReplay integration module
 * Only loads the OpenReplay tracker when explicitly enabled via environment variables.
 */

const isEnabled = import.meta.env.VITE_OPENREPLAY_ENABLED === 'true';
const projectKey = import.meta.env.VITE_OPENREPLAY_PROJECT_KEY;

// Tracker instance - only created when enabled
// eslint-disable-next-line @typescript-eslint/no-explicit-any
let tracker: any = null;
let zustandPlugin: unknown = null;

// Initialize OpenReplay only when enabled
async function loadOpenReplay() {
  if (!isEnabled || !projectKey) {
    return;
  }

  try {
    const [{ default: Tracker }, { default: trackerZustand }] = await Promise.all([
      import('@openreplay/tracker'),
      import('@openreplay/tracker-zustand'),
    ]);

    const config: Record<string, unknown> = {
      projectKey,
      obscureTextEmails: true,
      obscureTextNumbers: true,
      obscureInputEmails: true,
      obscureInputNumbers: true,
      respectDoNotTrack: true,
      capturePerformance: true,
    };

    if (import.meta.env.VITE_OPENREPLAY_INGEST_POINT) {
      config.ingestPoint = import.meta.env.VITE_OPENREPLAY_INGEST_POINT;
    }

    if (import.meta.env.DEV && import.meta.env.VITE_OPENREPLAY_INSECURE === 'true') {
      config.__DISABLE_SECURE_MODE = true;
    }

    tracker = new Tracker(config);
    zustandPlugin = tracker.use(trackerZustand({
      filter: () => true,
      transformer: (state: unknown) => state,
    }));
  } catch (error) {
    console.error('Failed to load OpenReplay:', error);
  }
}

// Load OpenReplay asynchronously if enabled
if (isEnabled && projectKey) {
  loadOpenReplay();
}

export function initOpenReplay(): void {
  if (!tracker) return;

  try {
    tracker.start();
    const userId = localStorage.getItem('userId');
    if (userId) {
      tracker.setUserID(userId);
    }
    tracker.setMetadata('app_version', import.meta.env.VITE_APP_VERSION || '1.0.0');
    tracker.setMetadata('platform', 'handheld');
    tracker.event('openreplay_initialized', {
      timestamp: new Date().toISOString(),
      userAgent: navigator.userAgent,
    });
  } catch (error) {
    console.error('Failed to start OpenReplay:', error);
  }
}

export function trackPageView(pageName: string): void {
  if (tracker) {
    tracker.event('page_view', { page: pageName });
  }
}

export function getZustandPlugin(): unknown {
  return zustandPlugin;
}

export function trackRFIDOperation(
  operation: 'connect' | 'disconnect' | 'inventory_start' | 'inventory_stop' | 'tag_read' | 'error',
  details?: Record<string, unknown>
): void {
  if (tracker) {
    tracker.event(`rfid_${operation}`, {
      timestamp: Date.now(),
      ...details,
    });
  }
}

// Debug helpers for development
if (typeof window !== 'undefined' && import.meta.env.DEV) {
  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any).__checkOpenReplay = () => ({
    initialized: !!tracker,
    trackerObject: tracker,
    config: {
      enabled: import.meta.env.VITE_OPENREPLAY_ENABLED,
      hasKey: !!import.meta.env.VITE_OPENREPLAY_PROJECT_KEY,
    },
  });

  // eslint-disable-next-line @typescript-eslint/no-explicit-any
  (window as any).__testOpenReplay = () => {
    if (tracker) {
      tracker.event('test_event_manual', {
        timestamp: new Date().toISOString(),
        source: 'manual_test',
      });
    }
  };
}
