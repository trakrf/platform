// Wire types for the TRA-978 mustering POC. These mirror the Go models in
// backend/internal/models/muster/muster.go EXACTLY (snake_case JSON, all ids
// are numbers). Keep them in sync — they are the contract between the muster
// SSE/REST surface and the frontend store/components.

export type MusterEventStatus = 'active' | 'completed' | 'cancelled';

export type MusterEntryStatus = 'missing' | 'at_muster' | 'verified' | 'safe_manual';

/** Aggregate entry-status counts for an event. */
export interface MusterCounts {
  expected: number;
  missing: number;
  at_muster: number;
  verified: number;
  safe_manual: number;
}

/** A person-level entry within a muster event. */
export interface MusterEntry {
  id: number;
  org_id: number;
  muster_event_id: number;
  asset_id: number;
  label: string;
  expected_location_id?: number;
  status: MusterEntryStatus;
  muster_location_id?: number;
  first_muster_seen_at?: string;
  verified_by?: number;
  verified_at?: string;
  marked_safe_by?: number;
  marked_safe_at?: string;
  marked_safe_note?: string;
  // Populated only while the event is active (break-glass).
  last_seen_location_id?: number;
  last_seen_at?: string;
  created_at: string;
  updated_at: string;
}

/** A mustering drill event. One active event per org at a time. */
export interface MusterEvent {
  id: number;
  org_id: number;
  status: MusterEventStatus;
  started_at: string;
  ended_at?: string;
  window_minutes: number;
  started_by?: number;
  // Filled at all-clear. Shape is the report jsonb from the engine.
  report?: MusterReport;
  metadata?: Record<string, unknown>;
  entries?: MusterEntry[];
  counts: MusterCounts;
  created_at: string;
  updated_at: string;
}

/** Presence-only headcount for one location (zone or muster point). */
export interface ZonePresence {
  location_id: number;
  name: string;
  muster_point: boolean;
  count: number;
}

/** Most-recent sighting of a person-asset (only sent while an event is active). */
export interface PersonPresence {
  asset_id: number;
  label: string;
  location_id?: number;
  last_seen_at: string;
}

/** Per-zone breakdown inside an all-clear report. */
export interface MusterReportZone {
  location_id: number;
  name: string;
  expected: number;
  accounted: number;
  cleared_at?: string | null;
}

/** Per-muster-point arrival count inside an all-clear report. */
export interface MusterReportMusterPoint {
  location_id: number;
  name: string;
  arrivals: number;
}

/** The report jsonb computed at all-clear. */
export interface MusterReport {
  total_seconds: number;
  counts: MusterCounts;
  zones: MusterReportZone[];
  muster_points: MusterReportMusterPoint[];
}

/** A synthetic sighting fed to POST /mustering/simulate. */
export interface MusterSighting {
  asset_id: number;
  location_id: number;
}

// --- SSE frame payloads (GET /api/v1/mustering/stream) ---

export interface MusterSnapshotPayload {
  zones: ZonePresence[];
  persons_on_site: number;
  event: MusterEvent | null;
}

export interface MusterPresencePayload {
  zones: ZonePresence[];
  persons: PersonPresence[];
}

export interface MusterEntryPayload {
  entry: MusterEntry;
  counts: MusterCounts;
}

export interface MusterEventPayload {
  event: MusterEvent;
}
