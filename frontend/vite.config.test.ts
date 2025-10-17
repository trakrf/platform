import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

// Test configuration without HTTPS
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './')
    }
  },
  server: {
    // HTTPS is disabled by default
    host: true,
    port: 5173
  }
});