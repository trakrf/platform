// Browser-side MQTT config for the reader live-feed (TRA-902).
//
// These are VITE_ vars => public (baked into the bundle), so the broker user
// behind them MUST be a least-privilege, subscribe-only account scoped to
// trakrf.id/+/reads (infra prereq — see the design doc / TRA-857). An empty
// URL disables the feed entirely, which is the default: builds, tests, and
// preview stay inert until infra exposes the WSS listener + frontend-readonly
// user.

export interface ReaderFeedConfig {
  url: string;
  username: string;
  password: string;
  topic: string;
}

export function getReaderFeedConfig(): ReaderFeedConfig {
  const env = import.meta.env;
  return {
    url: env.VITE_READER_FEED_MQTT_URL ?? '',
    username: env.VITE_READER_FEED_MQTT_USERNAME ?? '',
    password: env.VITE_READER_FEED_MQTT_PASSWORD ?? '',
    topic: env.VITE_READER_FEED_MQTT_TOPIC ?? 'trakrf.id/+/reads',
  };
}

/** The feed is only usable when a broker WebSocket URL is configured. */
export function isReaderFeedConfigured(cfg: ReaderFeedConfig = getReaderFeedConfig()): boolean {
  return cfg.url.trim() !== '';
}
