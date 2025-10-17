import Tracker from '@openreplay/tracker';
import trackerZustand from '@openreplay/tracker-zustand';

// Initialize tracker immediately when module loads
const isEnabled = import.meta.env.VITE_OPENREPLAY_ENABLED === 'true';
const projectKey = import.meta.env.VITE_OPENREPLAY_PROJECT_KEY;

// console.log('OpenReplay module loading:', {
//   enabled: import.meta.env.VITE_OPENREPLAY_ENABLED,
//   isEnabled,
//   hasProjectKey: !!projectKey,
//   projectKeyLength: projectKey?.length || 0,
//   ingestPoint: import.meta.env.VITE_OPENREPLAY_INGEST_POINT || 'default',
//   isDev: import.meta.env.DEV,
//   insecureMode: import.meta.env.VITE_OPENREPLAY_INSECURE
// });

let tracker: Tracker | null = null;
// Type for the Zustand plugin - the actual type returned by tracker.use()
// We use 'unknown' because the plugin's exact type is complex and not exported by the library
let zustandPlugin: unknown = null;

if (isEnabled && projectKey) {
  const config: Record<string, unknown> = {
    projectKey,
  };
  
  // Add optional configurations
  if (import.meta.env.VITE_OPENREPLAY_INGEST_POINT) {
    config.ingestPoint = import.meta.env.VITE_OPENREPLAY_INGEST_POINT;
  }
  
  // Security and privacy settings
  config.obscureTextEmails = true;
  config.obscureTextNumbers = true;
  config.obscureInputEmails = true;
  config.obscureInputNumbers = true;
  config.respectDoNotTrack = true;
  config.capturePerformance = true;
  
  // Allow insecure mode for local development
  if (import.meta.env.DEV && import.meta.env.VITE_OPENREPLAY_INSECURE === 'true') {
    config.__DISABLE_SECURE_MODE = true;
  }
  
  try {
    tracker = new Tracker(config);
    // console.log('OpenReplay tracker created successfully');
    
    // Create Zustand plugin for state tracking
    zustandPlugin = tracker.use(trackerZustand({
      // Optional: filter sensitive data
      filter: (_mutation, _state) => {
        // Log all mutations by default
        return true;
      },
      // Optional: transform state before logging
      transformer: (state) => {
        // Return state as-is by default
        return state;
      }
    }));
    // console.log('OpenReplay Zustand plugin created');
  } catch (error) {
    // console.error('Failed to create OpenReplay tracker:', error);
  }
} else {
  // console.log('OpenReplay is disabled or project key is missing');
}

export function initOpenReplay(): void {
  if (!tracker) {
    // console.log('OpenReplay tracker not available - skipping initialization');
    return;
  }
  
  try {
    // Start the tracker - following the simple pattern from docs
    tracker.start();
    // console.log('OpenReplay tracker.start() called');
    
    // Set up user identification if available
    const userId = localStorage.getItem('userId');
    if (userId) {
      tracker.setUserID(userId);
    }

    // Add metadata about the device
    tracker.setMetadata('app_version', import.meta.env.VITE_APP_VERSION || '1.0.0');
    tracker.setMetadata('platform', 'handheld');
    
    // Send a test event to verify tracking works
    tracker.event('openreplay_initialized', {
      timestamp: new Date().toISOString(),
      userAgent: navigator.userAgent
    });
    
    // console.log('OpenReplay initialization completed');
  } catch (error) {
    // console.error('Failed to start OpenReplay:', error);
  }
}

// Helper function to track page views
export function trackPageView(pageName: string): void {
  if (tracker) {
    tracker.event('page_view', { page: pageName });
  }
}

// Export the Zustand plugin for store integration
export function getZustandPlugin(): unknown {
  return zustandPlugin;
}

// Helper function to track RFID operations
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

// Debug helper - expose tracker status to window for testing
if (typeof window !== 'undefined' && import.meta.env.DEV) {
  window.__checkOpenReplay = () => {
    const status = {
      initialized: !!tracker,
      trackerObject: tracker,
      sessionId: tracker ? (tracker as unknown as { sessionId?: string }).sessionId : null,
      isActive: tracker ? (tracker as unknown as { active?: boolean }).active : null,
      config: {
        enabled: import.meta.env.VITE_OPENREPLAY_ENABLED,
        hasKey: !!import.meta.env.VITE_OPENREPLAY_PROJECT_KEY,
        insecureMode: import.meta.env.VITE_OPENREPLAY_INSECURE,
      }
    };
    // console.log('OpenReplay Status:', status);
    return status;
  };
  
  window.__testOpenReplay = () => {
    if (tracker) {
      tracker.event('test_event_manual', {
        timestamp: new Date().toISOString(),
        source: 'manual_test'
      });
      // console.log('Test event sent to OpenReplay');
    } else {
      // console.error('OpenReplay tracker not initialized');
    }
  };
}