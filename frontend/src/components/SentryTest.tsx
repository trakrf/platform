import * as Sentry from '@sentry/react';

/**
 * Dev-only component to test Sentry integration.
 * Renders a button that captures a test error.
 */
export function SentryTest() {
  if (!import.meta.env.DEV) {
    return null;
  }

  const handleTestError = () => {
    Sentry.captureException(new Error('Sentry test error from React frontend'));
    alert('Test error sent to Sentry! Check dashboard.');
  };

  const handleTestCrash = () => {
    throw new Error('Sentry test crash - this should be caught by ErrorBoundary');
  };

  return (
    <div style={{ padding: '10px', margin: '10px', border: '1px dashed orange' }}>
      <strong>Sentry Test (Dev Only)</strong>
      <div style={{ marginTop: '8px', display: 'flex', gap: '8px' }}>
        <button
          onClick={handleTestError}
          style={{ padding: '4px 8px', cursor: 'pointer' }}
        >
          Send Test Error
        </button>
        <button
          onClick={handleTestCrash}
          style={{ padding: '4px 8px', cursor: 'pointer', backgroundColor: '#ffcccc' }}
        >
          Trigger Crash
        </button>
      </div>
    </div>
  );
}
