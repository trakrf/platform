import * as Sentry from '@sentry/react';

// Initialize Sentry for error tracking (disabled if DSN not set)
if (import.meta.env.VITE_SENTRY_DSN) {
  Sentry.init({
    dsn: import.meta.env.VITE_SENTRY_DSN,
    environment: import.meta.env.MODE,
    enabled: true,
  });
}

import React from 'react';
import ReactDOM from 'react-dom/client';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import App from './App';
import './styles/globals.css';

// Create a QueryClient instance for TanStack Query
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000, // 5 minutes
      gcTime: 10 * 60 * 1000, // 10 minutes (formerly cacheTime)
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

// Function to initialize the app - only called in non-test environments
function initializeApp() {
  const rootElement = document.getElementById('root');
  if (rootElement) {
    ReactDOM.createRoot(rootElement).render(
      <React.StrictMode>
        <QueryClientProvider client={queryClient}>
          <App />
        </QueryClientProvider>
      </React.StrictMode>,
    );
  }
}

// Expose stores for testing in development
if (import.meta.env.DEV) {
  import('./stores').then((stores) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (window as unknown as { __ZUSTAND_STORES__: any }).__ZUSTAND_STORES__ = {
      deviceStore: stores.useDeviceStore,
      tagStore: stores.useTagStore,
      uiStore: stores.useUIStore,
      settingsStore: stores.useSettingsStore,
      packetStore: stores.usePacketStore,
      barcodeStore: stores.useBarcodeStore
    };
  });

  // Expose DeviceManager for testing
  import('./lib/device/device-manager').then(({ DeviceManager }) => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    (window as unknown as { DeviceManager: any }).DeviceManager = DeviceManager;
  });

  // Create a global worker reference for testing
  (window as unknown as { __WORKER_DEVICE__: null }).__WORKER_DEVICE__ = null;
}

// Initialize the app - moved back to direct call
initializeApp();
