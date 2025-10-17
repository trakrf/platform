# CS108 Worker Implementation - ISOLATED THREAD CODE

## ⚠️ CRITICAL: Thread Isolation

This directory contains code that runs in a **Web Worker thread**, completely isolated from the main thread.

## Architecture

```
Main Thread                    |  Worker Thread
-------------------------------|--------------------------------
UI Components                  |
     ↓                         |
DeviceManager                  |
     ↓                         |
DeviceFactory.createWorker() ──┼──→ new Worker('cs108-worker.js')
     ↓                         |           ↓
postMessage() ─────────────────┼──→ onmessage handler
     ↑                         |           ↓
onmessage ←────────────────────┼─── postMessage()
                               |           ↓
                               |     CS108Reader (this code)
```

## Import Rules

### ❌ NEVER Import CS108 Code Into:
- UI components (`src/components/`)
- Stores (`src/stores/`)
- DeviceManager or DeviceFactory
- Other workers
- Any main thread code

### ✅ ONLY Import Shared Types:
- `import { ReaderState } from '@/worker/types/reader'` ✅
- `import { CS108Reader } from '@/worker/cs108/reader'` ❌

## Why This Matters

1. **Thread Boundary**: Workers run in a separate JavaScript context with:
   - No DOM access
   - No shared memory (except SharedArrayBuffer)
   - Only structured cloning for data transfer

2. **Build Separation**: The worker is built as a separate bundle (`cs108-worker.js`)

3. **Runtime Isolation**: Direct imports would either:
   - Fail at runtime (missing DOM APIs)
   - Accidentally include worker code in main bundle
   - Break the thread isolation model

## Communication Pattern

```typescript
// Main Thread (DeviceManager)
worker.postMessage({ type: 'connect', config });
worker.onmessage = (e) => handleWorkerMessage(e.data);

// Worker Thread (this code)
self.onmessage = (e) => handleMainMessage(e.data);
self.postMessage({ type: 'connected', state });
```

## Testing

- Unit tests can import directly (they run in Node, not browser)
- Integration tests use the worker via `DeviceFactory`
- E2E tests go through the full stack

Remember: **This is an island**. Code here cannot and should not be directly accessible from the main thread.