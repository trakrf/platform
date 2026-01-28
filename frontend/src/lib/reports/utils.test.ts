import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { getFreshnessStatus, formatDuration, formatRelativeTime } from './utils';

describe('getFreshnessStatus', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "live" for < 15 minutes ago', () => {
    const tenMinAgo = new Date('2025-01-27T11:50:00Z').toISOString();
    expect(getFreshnessStatus(tenMinAgo)).toBe('live');
  });

  it('returns "today" for 15 min - 24 hours ago', () => {
    const twoHoursAgo = new Date('2025-01-27T10:00:00Z').toISOString();
    expect(getFreshnessStatus(twoHoursAgo)).toBe('today');
  });

  it('returns "recent" for 1-7 days ago', () => {
    const threeDaysAgo = new Date('2025-01-24T12:00:00Z').toISOString();
    expect(getFreshnessStatus(threeDaysAgo)).toBe('recent');
  });

  it('returns "stale" for > 7 days ago', () => {
    const twoWeeksAgo = new Date('2025-01-13T12:00:00Z').toISOString();
    expect(getFreshnessStatus(twoWeeksAgo)).toBe('stale');
  });

  it('returns "live" at exactly 14 minutes', () => {
    const fourteenMinAgo = new Date('2025-01-27T11:46:00Z').toISOString();
    expect(getFreshnessStatus(fourteenMinAgo)).toBe('live');
  });

  it('returns "today" at exactly 15 minutes', () => {
    const fifteenMinAgo = new Date('2025-01-27T11:45:00Z').toISOString();
    expect(getFreshnessStatus(fifteenMinAgo)).toBe('today');
  });
});

describe('formatDuration', () => {
  it('returns "—" for null', () => {
    expect(formatDuration(null)).toBe('—');
  });

  it('formats minutes only', () => {
    expect(formatDuration(120)).toBe('2m');
    expect(formatDuration(45 * 60)).toBe('45m');
  });

  it('formats hours and minutes', () => {
    expect(formatDuration(3700)).toBe('1h 1m');
    expect(formatDuration(2 * 3600 + 30 * 60)).toBe('2h 30m');
  });

  it('formats hours only when no remaining minutes', () => {
    expect(formatDuration(3600)).toBe('1h');
    expect(formatDuration(7200)).toBe('2h');
  });

  it('handles zero', () => {
    expect(formatDuration(0)).toBe('0m');
  });

  it('handles large values', () => {
    expect(formatDuration(24 * 3600)).toBe('24h');
    expect(formatDuration(100 * 3600 + 59 * 60)).toBe('100h 59m');
  });
});

describe('formatRelativeTime', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.setSystemTime(new Date('2025-01-27T12:00:00Z'));
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  it('returns "Just now" for < 1 minute', () => {
    const now = new Date('2025-01-27T11:59:30Z').toISOString();
    expect(formatRelativeTime(now)).toBe('Just now');
  });

  it('returns minutes ago', () => {
    const fiveMinAgo = new Date('2025-01-27T11:55:00Z').toISOString();
    expect(formatRelativeTime(fiveMinAgo)).toBe('5 min ago');
  });

  it('returns "1 hour ago" singular', () => {
    const oneHourAgo = new Date('2025-01-27T11:00:00Z').toISOString();
    expect(formatRelativeTime(oneHourAgo)).toBe('1 hour ago');
  });

  it('returns hours ago plural', () => {
    const threeHoursAgo = new Date('2025-01-27T09:00:00Z').toISOString();
    expect(formatRelativeTime(threeHoursAgo)).toBe('3 hours ago');
  });

  it('returns "Yesterday" for 1 day ago', () => {
    const yesterday = new Date('2025-01-26T12:00:00Z').toISOString();
    expect(formatRelativeTime(yesterday)).toBe('Yesterday');
  });

  it('returns days ago for 2-6 days', () => {
    const threeDaysAgo = new Date('2025-01-24T12:00:00Z').toISOString();
    expect(formatRelativeTime(threeDaysAgo)).toBe('3 days ago');
  });

  it('returns formatted date for > 7 days', () => {
    const twoWeeksAgo = new Date('2025-01-13T12:00:00Z').toISOString();
    expect(formatRelativeTime(twoWeeksAgo)).toBe('Jan 13');
  });
});
