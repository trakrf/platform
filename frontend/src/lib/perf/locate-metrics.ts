/**
 * Locate Performance Metrics
 *
 * Lightweight instrumentation to identify bottlenecks in the locate data flow.
 * Enable via: window.__LOCATE_METRICS_ENABLED = true
 * View via:   window.__LOCATE_METRICS.getSummary()
 */

interface MetricsSample {
  timestamp: number;
  value: number;
}

interface MetricsChannel {
  name: string;
  samples: MetricsSample[];
  maxSamples: number;
}

interface LocateMetrics {
  // Counters
  workerEventsPosted: number;
  workerEventsThrottled: number;
  storeUpdatesReceived: number;
  componentRenders: number;

  // Timing channels (ring buffers)
  channels: {
    workerProcessingMs: MetricsChannel;
    messageLatencyMs: MetricsChannel;
    storeProcessingMs: MetricsChannel;
    renderIntervalMs: MetricsChannel;
  };

  // Timestamps for rate calculation
  startTime: number;
  lastWorkerPost: number;
  lastStoreUpdate: number;
  lastRender: number;
}

// Raw counter for debugging (always increments, ignores enabled flag)
let rawStoreUpdateCount = 0;

// Global metrics instance
const metrics: LocateMetrics = {
  workerEventsPosted: 0,
  workerEventsThrottled: 0,
  storeUpdatesReceived: 0,
  componentRenders: 0,

  channels: {
    workerProcessingMs: { name: 'Worker Processing', samples: [], maxSamples: 100 },
    messageLatencyMs: { name: 'Message Latency', samples: [], maxSamples: 100 },
    storeProcessingMs: { name: 'Store Processing', samples: [], maxSamples: 100 },
    renderIntervalMs: { name: 'Render Interval', samples: [], maxSamples: 100 },
  },

  startTime: 0,
  lastWorkerPost: 0,
  lastStoreUpdate: 0,
  lastRender: 0,
};

/**
 * Check if metrics collection is enabled
 * Checks both window and globalThis for compatibility across contexts
 */
export function isMetricsEnabled(): boolean {
  // Check window first (browser main thread)
  if (typeof window !== 'undefined' && (window as unknown as Record<string, unknown>).__LOCATE_METRICS_ENABLED) {
    return true;
  }
  // Fallback to globalThis (workers, other contexts)
  if (typeof globalThis !== 'undefined' && (globalThis as unknown as Record<string, unknown>).__LOCATE_METRICS_ENABLED) {
    return true;
  }
  return false;
}

/**
 * Add a sample to a metrics channel
 */
function addSample(channel: MetricsChannel, value: number): void {
  const sample: MetricsSample = { timestamp: performance.now(), value };
  channel.samples.push(sample);
  if (channel.samples.length > channel.maxSamples) {
    channel.samples.shift();
  }
}

/**
 * Get statistics for a channel
 */
function getChannelStats(channel: MetricsChannel): { avg: number; min: number; max: number; p95: number; count: number } {
  if (channel.samples.length === 0) {
    return { avg: 0, min: 0, max: 0, p95: 0, count: 0 };
  }

  const values = channel.samples.map(s => s.value).sort((a, b) => a - b);
  const sum = values.reduce((a, b) => a + b, 0);
  const p95Index = Math.floor(values.length * 0.95);

  return {
    avg: Math.round(sum / values.length * 100) / 100,
    min: Math.round(values[0] * 100) / 100,
    max: Math.round(values[values.length - 1] * 100) / 100,
    p95: Math.round(values[p95Index] * 100) / 100,
    count: values.length,
  };
}

// ============================================================================
// Worker-side instrumentation (call from worker)
// ============================================================================

/**
 * Record worker event posted (call when posting LOCATE_UPDATE)
 * Returns timestamp to include in payload for latency measurement
 * Uses Date.now() for cross-context timing (worker <-> main thread)
 */
export function recordWorkerPost(): number {
  // Always return Date.now() for cross-context latency measurement
  const crossContextTimestamp = Date.now();

  if (!isMetricsEnabled()) return crossContextTimestamp;

  const now = performance.now();
  metrics.workerEventsPosted++;

  if (metrics.startTime === 0) {
    metrics.startTime = now;
  }

  if (metrics.lastWorkerPost > 0) {
    const interval = now - metrics.lastWorkerPost;
    addSample(metrics.channels.workerProcessingMs, interval);
  }
  metrics.lastWorkerPost = now;

  return crossContextTimestamp;
}

/**
 * Record worker event throttled (call when skipping due to throttle)
 */
export function recordWorkerThrottled(): void {
  if (!isMetricsEnabled()) return;
  metrics.workerEventsThrottled++;
}

// ============================================================================
// Main thread instrumentation (call from store/component)
// ============================================================================

/**
 * Record store update received
 * @param workerTimestamp - Timestamp from worker payload for latency calculation (Date.now())
 */
export function recordStoreUpdateStart(workerTimestamp?: number): () => void {
  // Always increment raw counter for debugging
  rawStoreUpdateCount++;

  if (!isMetricsEnabled()) return () => {};

  const startTime = performance.now();
  metrics.storeUpdatesReceived++;

  // Measure message latency if worker timestamp provided
  // Uses Date.now() for cross-context timing (worker timestamp is Date.now())
  if (workerTimestamp) {
    const latency = Date.now() - workerTimestamp;
    addSample(metrics.channels.messageLatencyMs, latency);
  }

  // Return function to call when store update completes
  return () => {
    const processingTime = performance.now() - startTime;
    addSample(metrics.channels.storeProcessingMs, processingTime);
    metrics.lastStoreUpdate = performance.now();
  };
}

/**
 * Record component render
 */
export function recordComponentRender(): void {
  if (!isMetricsEnabled()) return;

  const now = performance.now();
  metrics.componentRenders++;

  if (metrics.lastRender > 0) {
    const interval = now - metrics.lastRender;
    addSample(metrics.channels.renderIntervalMs, interval);
  }
  metrics.lastRender = now;
}

// ============================================================================
// Metrics retrieval
// ============================================================================

/**
 * Get metrics summary
 */
export function getMetricsSummary(): Record<string, unknown> {
  const elapsed = (performance.now() - metrics.startTime) / 1000;

  return {
    elapsed: `${elapsed.toFixed(1)}s`,
    rates: {
      workerPostsPerSec: (metrics.workerEventsPosted / elapsed).toFixed(1),
      storeUpdatesPerSec: (metrics.storeUpdatesReceived / elapsed).toFixed(1),
      rendersPerSec: (metrics.componentRenders / elapsed).toFixed(1),
      throttleRatio: metrics.workerEventsPosted > 0
        ? ((metrics.workerEventsThrottled / (metrics.workerEventsPosted + metrics.workerEventsThrottled)) * 100).toFixed(1) + '%'
        : '0%',
    },
    counters: {
      workerEventsPosted: metrics.workerEventsPosted,
      workerEventsThrottled: metrics.workerEventsThrottled,
      storeUpdatesReceived: metrics.storeUpdatesReceived,
      componentRenders: metrics.componentRenders,
    },
    timing: {
      workerInterval: getChannelStats(metrics.channels.workerProcessingMs),
      messageLatency: getChannelStats(metrics.channels.messageLatencyMs),
      storeProcessing: getChannelStats(metrics.channels.storeProcessingMs),
      renderInterval: getChannelStats(metrics.channels.renderIntervalMs),
    },
  };
}

/**
 * Reset all metrics
 */
export function resetMetrics(): void {
  rawStoreUpdateCount = 0;
  metrics.workerEventsPosted = 0;
  metrics.workerEventsThrottled = 0;
  metrics.storeUpdatesReceived = 0;
  metrics.componentRenders = 0;
  metrics.startTime = 0;
  metrics.lastWorkerPost = 0;
  metrics.lastStoreUpdate = 0;
  metrics.lastRender = 0;

  Object.values(metrics.channels).forEach(channel => {
    channel.samples = [];
  });
}

/**
 * Log metrics summary to console
 */
export function logMetricsSummary(): void {
  const enabled = isMetricsEnabled();
  const summary = getMetricsSummary();

  console.log('[Locate Metrics] Summary:');
  console.log(`  Enabled: ${enabled}, Raw calls: ${rawStoreUpdateCount}, Recorded: ${metrics.storeUpdatesReceived}`);
  console.log(`  Capture rate: ${metrics.storeUpdatesReceived > 0 ? ((metrics.storeUpdatesReceived / rawStoreUpdateCount) * 100).toFixed(1) : 0}%`);
  console.table(summary.rates);
  console.log('Timing (ms):');
  console.table(summary.timing);
}

// Expose to window for debugging
if (typeof window !== 'undefined') {
  (window as unknown as Record<string, unknown>).__LOCATE_METRICS = {
    getSummary: getMetricsSummary,
    reset: resetMetrics,
    log: logMetricsSummary,
    // Debug: raw counter that always increments (ignores enabled flag)
    getRawCount: () => rawStoreUpdateCount,
    resetRawCount: () => { rawStoreUpdateCount = 0; },
  };
}
