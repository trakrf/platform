/**
 * Audio Feedback Utilities
 */

export function calculateReadFrequency(timestamps: number[]): number {
  if (timestamps.length < 2) return 0;
  const recentTimestamps = timestamps.slice(-10); // Last 10 reads
  const timeDiff = recentTimestamps[recentTimestamps.length - 1] - recentTimestamps[0];
  return (recentTimestamps.length - 1) / (timeDiff / 1000); // Reads per second
}

export function frequencyToBeepInterval(readsPerSecond: number): number {
  if (readsPerSecond === 0) return 0;

  // For very slow rates (< 1 tag/sec), use proportional interval
  // Scaled down by 5x - was 1000/readsPerSecond, now 5000/readsPerSecond
  if (readsPerSecond < 1 && readsPerSecond > 0) {
    return Math.min(10000, Math.round(5000 / readsPerSecond));
  }

  // Exponential decay for rates >= 1 tag/sec
  // Scaled down by 5x - base interval was 1000ms, now 5000ms
  const baseInterval = 5000;
  const baseFrequency = 1;
  const scalingFactor = 2.5;

  const interval = baseInterval * Math.pow(2, -(readsPerSecond - baseFrequency) / scalingFactor);
  // Min interval was 100ms (10 beeps/sec), now 500ms (2 beeps/sec)
  // Max interval was 2000ms, now 10000ms
  return Math.min(10000, Math.max(500, Math.round(interval)));
}

export function playSuccessSound(): void {
  // TODO: Implement actual sound playback
  console.log('Success sound');
}

export function playErrorSound(): void {
  // TODO: Implement actual sound playback
  console.log('Error sound');
}

export function playTickSound(): void {
  // TODO: Implement actual sound playback
  console.log('Tick sound');
}