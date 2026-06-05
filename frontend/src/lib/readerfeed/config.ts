// Browser-side MQTT config for the reader live-feed (TRA-902).
//
// Sourced at RUNTIME from window.__APP_CONFIG__.readerFeed — injected into
// index.html by the backend at serve time (the TRA-853 mechanism), NOT baked
// into the bundle. One immutable build connects to whatever broker the pod's
// env points at; infra flips it on by setting READER_FEED_MQTT_* pod env vars,
// no frontend rebuild.
//
// These values are public (served in pre-auth index.html), so the broker user
// MUST be least-privilege, subscribe-only. An empty URL disables the feed: the
// default everywhere until infra exposes the WSS listener + frontend-readonly
// user.

const DEFAULT_TOPIC = 'trakrf.id/+/reads';

export interface ReaderFeedConfig {
  url: string;
  username: string;
  password: string;
  topic: string;
}

export function getReaderFeedConfig(): ReaderFeedConfig {
  const rf = (typeof window !== 'undefined' ? window.__APP_CONFIG__?.readerFeed : undefined) ?? {};
  const topic = rf.topic?.trim();
  return {
    url: rf.url ?? '',
    username: rf.username ?? '',
    password: rf.password ?? '',
    topic: topic && topic !== '' ? topic : DEFAULT_TOPIC,
  };
}

/** The feed is only usable when a broker WebSocket URL is configured. */
export function isReaderFeedConfigured(cfg: ReaderFeedConfig = getReaderFeedConfig()): boolean {
  return cfg.url.trim() !== '';
}
