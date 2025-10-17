#!/usr/bin/env node

/**
 * Check what's running on the dev server port
 */

import { execSync } from 'child_process';

const PORT = 5173;

try {
  const result = execSync(`lsof -i :${PORT} | grep LISTEN || true`, { encoding: 'utf8' });
  
  if (!result.trim()) {
    console.log(`✅ Port ${PORT} is available`);
    process.exit(0);
  }
  
  console.log(`📋 Port ${PORT} is in use:`);
  console.log(result);
  
  if (result.includes('dev-mock')) {
    console.log('🔌 Mock server is running (pnpm dev:mock)');
  } else if (result.includes('vite')) {
    console.log('🚀 Regular dev server is running (pnpm dev)');
  } else {
    console.log('❓ Unknown process is using the port');
  }
  
} catch (error) {
  console.error('Error checking port:', error.message);
  process.exit(1);
}