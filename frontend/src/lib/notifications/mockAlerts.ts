/**
 * Generated alert history for non-production environments. No endpoint serves
 * alerts yet; this builds rows in the exact NotificationAlert shape from the
 * org's real assets and contacts, so per-asset history reads meaningfully.
 *
 * Deterministic: every value is derived from the asset id and `now`, never
 * Math.random, so the same input always yields the same rows and ids.
 */
import type { Asset } from '@/types/assets';
import type {
  NotificationAlert,
  NotificationChannel,
  NotificationRecipient,
} from '@/types/notifications';
import { formatTime } from '@/lib/reports/utils';

type MockAsset = Pick<Asset, 'id' | 'name' | 'external_key'>;
type MockRecipient = Pick<NotificationRecipient, 'id' | 'name' | 'email' | 'phone'>;

interface SourceAsset extends MockAsset {
  /** How the message refers to the asset, e.g. "RV 'Blue Wind'". */
  label: string;
}

// Used when the org has no assets or no contacts, so the page is never empty in dev.
const FALLBACK_ASSETS: SourceAsset[] = [
  { id: 900001, name: 'Blue Wind RV', external_key: 'RV-0042', label: "RV 'Blue Wind'" },
  { id: 900002, name: 'Sea Breeze 24ft', external_key: 'BOAT-0117', label: "boat 'Sea Breeze'" },
  { id: 900003, name: 'Trailer #12', external_key: 'TRL-0012', label: "trailer 'Trailer #12'" },
];

const FALLBACK_RECIPIENTS: MockRecipient[] = [
  { id: 900101, name: 'Maria Nguyen', email: 'maria@example.com', phone: '+15551230001' },
  { id: 900102, name: 'Sam Okafor', email: 'sam@example.com', phone: '+15551230002' },
];

const GATES = ['Gate A', 'Gate B', 'North Gate', 'Service Entrance'];
const PLACES = ['Row A', 'Row B', 'Row C', 'Wash Bay', 'Service Bay', 'Storage Lot 2'];
const DIRECTIONS = ['unknown', 'out', 'out'];
// Weighted 0–4 events per asset (mean 2.6); each event reaches 2–3 subscribers
// when the org has that many, so ten assets yield 40+ rows.
const EVENT_COUNTS = [0, 2, 3, 4, 4];

const DAY_MS = 24 * 60 * 60 * 1000;
const WINDOW_MS = 30 * DAY_MS;

/**
 * FNV-1a over the id's digits (asset ids are 52-bit, too wide for bit ops),
 * then a murmur3 finaliser so neighbouring ids get unrelated seeds.
 */
function seedFor(id: number): number {
  let hash = 0x811c9dc5;
  for (const ch of String(id)) {
    hash ^= ch.charCodeAt(0);
    hash = Math.imul(hash, 0x01000193);
  }
  hash ^= hash >>> 16;
  hash = Math.imul(hash, 0x85ebca6b);
  hash ^= hash >>> 13;
  hash = Math.imul(hash, 0xc2b2ae35);
  hash ^= hash >>> 16;
  return hash >>> 0;
}

/** mulberry32: a tiny seeded PRNG returning floats in [0, 1). */
function prng(seed: number): () => number {
  let a = seed;
  return () => {
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

const pick = <T>(rand: () => number, items: readonly T[]): T =>
  items[Math.floor(rand() * items.length)];

function channelFor(rand: () => number, recipient: MockRecipient): NotificationChannel {
  if (recipient.email && !recipient.phone) return 'email';
  if (recipient.phone && !recipient.email) return 'sms';
  return rand() < 0.6 ? 'email' : 'sms';
}

interface MockEvent {
  type: NotificationAlert['type'];
  subject: string;
  body: string;
  details: Record<string, string>;
}

function alarmEvent(rand: () => number, asset: SourceAsset, at: Date): MockEvent {
  const location = pick(rand, GATES);
  const direction = pick(rand, DIRECTIONS);
  const suffix = direction === 'unknown' ? ' Direction could not be confirmed.' : '';
  return {
    type: 'alarm',
    subject: `Alert: ${asset.name} left the yard`,
    body: `Your ${asset.label} left the yard through ${location} at ${formatTime(at)}.${suffix}`,
    details: { location, direction },
  };
}

function locationChangeEvent(rand: () => number, asset: SourceAsset, at: Date): MockEvent {
  const from = pick(rand, PLACES);
  const to = pick(rand, PLACES.filter((p) => p !== from));
  return {
    type: 'location_change',
    subject: `${asset.name} changed location`,
    body: `Your ${asset.label} moved from ${from} to ${to} at ${formatTime(at)}.`,
    details: { from, to },
  };
}

export function buildMockAlerts(
  assets: MockAsset[],
  recipients: MockRecipient[],
  now: Date = new Date()
): NotificationAlert[] {
  const useFallback = assets.length === 0 || recipients.length === 0;
  const sourceAssets: SourceAsset[] = useFallback
    ? FALLBACK_ASSETS
    : assets.map((a) => ({ ...a, label: `asset '${a.name}'` }));
  const sourceRecipients = useFallback ? FALLBACK_RECIPIENTS : recipients;

  // Fixed order so sequential ids are stable whatever order the API returns.
  const ordered = [...sourceAssets].sort((a, b) => a.id - b.id);
  const alerts: NotificationAlert[] = [];
  let nextId = 1;

  for (const asset of ordered) {
    const rand = prng(seedFor(asset.id));
    // The fallback set is small, so give each of its assets the maximum.
    const eventCount = useFallback ? 4 : pick(rand, EVENT_COUNTS);
    const startsWithAlarm = rand() < 0.5;

    for (let e = 0; e < eventCount; e++) {
      const occurred = new Date(now.getTime() - Math.floor(rand() * WINDOW_MS));
      const isAlarm = (e % 2 === 0) === startsWithAlarm;
      const event = isAlarm
        ? alarmEvent(rand, asset, occurred)
        : locationChangeEvent(rand, asset, occurred);

      // Fan out to 2–3 distinct subscribers, starting at a seeded offset.
      const fanOut = Math.min(sourceRecipients.length, 2 + Math.floor(rand() * 2));
      const offset = Math.floor(rand() * sourceRecipients.length);
      for (let r = 0; r < fanOut; r++) {
        const recipient = sourceRecipients[(offset + r) % sourceRecipients.length];
        const channel = channelFor(rand, recipient);
        // Delivery lags the event by 5–90 seconds.
        const sent = new Date(occurred.getTime() + (5 + Math.floor(rand() * 86)) * 1000);
        alerts.push({
          id: nextId++,
          asset_id: asset.id,
          asset_name: asset.name,
          asset_external_key: asset.external_key,
          type: event.type,
          occurred_at: occurred.toISOString(),
          sent_at: sent.toISOString(),
          recipient_id: recipient.id,
          recipient_name: recipient.name,
          channel,
          subject: channel === 'email' ? event.subject : null,
          body: event.body,
          details: event.details,
        });
      }
    }
  }

  return alerts;
}
