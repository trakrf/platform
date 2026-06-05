// useReaderFeed — owns the browser MQTT client lifecycle for the live read
// feed. Connects on mount from env config, subscribes the reads topic, feeds
// each publish through the pure parse/merge/expire reducer, and tears the
// client down on unmount. Connection is lazy (no module-load side effect,
// unlike Power Mixer's import-time singleton).

import { useEffect, useMemo, useRef, useState } from 'react';
import mqtt from 'mqtt';
import type { MqttClient } from 'mqtt';
import { parseReaderPayload } from '@/lib/readerfeed/parse';
import { mergeReads, expireReads, READ_TTL_SECONDS } from '@/lib/readerfeed/store';
import { getReaderFeedConfig, isReaderFeedConfigured } from '@/lib/readerfeed/config';
import type { LiveRead, ReaderFeedStatus } from '@/types/readerfeed';

const EXPIRY_TICK_MS = 1000;

export interface ReaderFeedState {
  reads: LiveRead[];
  status: ReaderFeedStatus;
  error: string | null;
  /** Distinct reader keys currently in view. */
  readerCount: number;
  topic: string;
  configured: boolean;
}

export function useReaderFeed(): ReaderFeedState {
  const cfg = useMemo(() => getReaderFeedConfig(), []);
  const configured = isReaderFeedConfigured(cfg);

  const [tags, setTags] = useState<Map<string, LiveRead>>(new Map());
  const [status, setStatus] = useState<ReaderFeedStatus>(configured ? 'connecting' : 'disabled');
  const [error, setError] = useState<string | null>(null);
  const clientRef = useRef<MqttClient | null>(null);

  // MQTT connection lifecycle.
  useEffect(() => {
    if (!configured) return;

    const client = mqtt.connect(cfg.url, {
      clientId: `trakrf-reader-feed-${Math.random().toString(16).slice(2, 8)}`,
      username: cfg.username || undefined,
      password: cfg.password || undefined,
      clean: true,
      reconnectPeriod: 2000,
      connectTimeout: 30_000,
    });
    clientRef.current = client;

    client.on('connect', () => {
      setStatus('connected');
      setError(null);
      client.subscribe(cfg.topic, { qos: 0 }, (err) => {
        if (err) setError(`subscribe failed: ${err.message}`);
      });
    });

    client.on('reconnect', () => setStatus('connecting'));
    client.on('close', () => setStatus((s) => (s === 'error' ? s : 'closed')));
    client.on('error', (err) => {
      setStatus('error');
      setError(err.message);
    });

    client.on('message', (topic, payload) => {
      const reads = parseReaderPayload(topic, payload);
      if (reads.length === 0) return;
      setTags((prev) => mergeReads(prev, reads, Date.now()));
    });

    return () => {
      clientRef.current = null;
      client.end(true);
    };
  }, [cfg, configured]);

  // Age-based expiry tick (drops reads past the TTL window).
  useEffect(() => {
    if (!configured) return;
    const id = setInterval(() => {
      setTags((prev) => expireReads(prev, Date.now(), READ_TTL_SECONDS));
    }, EXPIRY_TICK_MS);
    return () => clearInterval(id);
  }, [configured]);

  const reads = useMemo(() => [...tags.values()], [tags]);
  const readerCount = useMemo(() => new Set(reads.map((r) => r.readerKey)).size, [reads]);

  return { reads, status, error, readerCount, topic: cfg.topic, configured };
}
