/// <reference types="vite/client" />
/// <reference types="vite-plugin-comlink/client" />

// Reader live-feed (TRA-902) — browser MQTT-over-WebSocket config. These are
// public (baked into the bundle), so the broker user must be least-privilege,
// subscribe-only. Empty URL disables the feed.
interface ImportMetaEnv {
  readonly VITE_READER_FEED_MQTT_URL?: string;
  readonly VITE_READER_FEED_MQTT_USERNAME?: string;
  readonly VITE_READER_FEED_MQTT_PASSWORD?: string;
  readonly VITE_READER_FEED_MQTT_TOPIC?: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
