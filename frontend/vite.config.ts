import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';
import { comlink } from 'vite-plugin-comlink';
import path from 'path';
import fs from 'fs';
import { fileURLToPath } from 'url';
import { getViteMockConfig } from './tests/config/vite-mock.config';

// Define __dirname for ES modules
const __dirname = path.dirname(fileURLToPath(import.meta.url));

// Check if local certificates exist
const certExists = fs.existsSync('./.cert/localhost.pem') && fs.existsSync('./.cert/localhost-key.pem');

// BLE Mock Plugin
function injectBleMockPlugin(env: Record<string, string>) {
  const mockEnabled = env.VITE_BLE_MOCK_ENABLED === 'true';
  
  return {
    name: 'inject-ble-mock',
    transformIndexHtml(html: string) {
      // Only inject the mock when explicitly enabled
      if (!mockEnabled) {
        return html;
      }

      // Load the ble-mcp-test bundle from public folder
      const bundlePath = path.join(process.cwd(), 'public/web-ble-mock.bundle.js');
      let bundleCode = '';
      
      try {
        bundleCode = fs.readFileSync(bundlePath, 'utf-8');
      } catch (err) {
        console.error('[BLE Mock Plugin] Failed to read bundle:', err);
        return html;
      }

      // Get shared configuration from ble-test-config
      const mockConfig = getViteMockConfig();

      // Inject the bundle and initialize the mock
      const injection = `
    <!-- BLE Mock Injection -->
    <script>
    // Load the bundle (it sets window.WebBleMock)
    ${bundleCode}
    
    // Initialize the Web Bluetooth mock (0.7.2 API)
    if (typeof WebBleMock !== 'undefined' && WebBleMock.injectWebBluetoothMock) {
      // Configuration object from shared config
      const mockConfig = ${JSON.stringify(mockConfig)};
      // Mock config loaded
      WebBleMock.injectWebBluetoothMock(mockConfig);
      // Injected ble-mcp-test mock
      
      // Verify injection and mark as mocked
      if ('bluetooth' in navigator) {
        // navigator.bluetooth is now available
        // Add a marker to indicate this is a mock
        window.__webBluetoothMocked = true;
      } else {
        console.error('[WebBLE Adapter] Failed to inject navigator.bluetooth');
      }
    } else {
      console.error('[WebBLE Adapter] Bundle loaded but WebBleMock not found');
    }
    </script>`;

      // Insert before the closing head tag
      return html.replace('</head>', injection + '\n  </head>');
    }
  };
}

export default defineConfig(({ mode }) => {
  // Load env file based on mode from project root (monorepo)
  const env = loadEnv(mode, path.resolve(__dirname, '../'), '');

  return {
    envDir: '../', // Read .env files from project root (monorepo)
    plugins: [
      react(),
      comlink(), // Add vite-plugin-comlink for worker proxying
      // Inject BLE mock when running in mock mode
      injectBleMockPlugin(env)
    ],
    worker: {
      format: 'es', // Ensure ES modules for workers
      plugins: () => [comlink()] // Also add comlink plugin for workers
    },
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
      '@test-utils': path.resolve(__dirname, './test-utils')
    }
  },
  server: {
    https: env.VITE_HTTPS === 'true' ? (certExists ? {
      cert: fs.readFileSync('./.cert/localhost.pem'),
      key: fs.readFileSync('./.cert/localhost-key.pem')
    } : true as unknown as { cert: string; key: string }) : undefined,
    host: true,
    cors: true,
    headers: {
      'Access-Control-Allow-Origin': '*',
      'Access-Control-Allow-Methods': 'GET, POST, PUT, DELETE, OPTIONS',
      'Access-Control-Allow-Headers': 'Origin, X-Requested-With, Content-Type, Accept'
    },
    hmr: {
      protocol: 'wss',
      clientPort: 443,
      host: 'localhost'
    },
    // Allow all hosts - use wildcard patterns for any ngrok subdomain
    allowedHosts: ['.ngrok-free.app', '.ngrok.io', '.localhost', 'localhost']
  },
  build: {
    rollupOptions: {
      output: {
        manualChunks: {
          // Separate vendor chunks
          'react-vendor': ['react', 'react-dom'],
          'ui-vendor': ['@headlessui/react', 'react-hot-toast', 'clsx'],
          'icons': ['react-icons'],
          'gauge': ['react-gauge-component']
        }
      }
    },
    // Set chunk size warning limit
    chunkSizeWarningLimit: 500
  }
  };
});