/**
 * Mustering API Client (TRA-978 POC).
 *
 * Type-safe wrapper around the internal, session-authenticated mustering
 * endpoints. Uses the shared apiClient with automatic JWT injection + org_id
 * context. Errors propagate unchanged — callers extract RFC 7807. All routes
 * are internal-only (NOT in the public OpenAPI surface).
 */

import { apiClient } from '../client';
import type {
  MusterEvent,
  MusterEntry,
  MusterCounts,
  MusterSighting,
  MusterSnapshotPayload,
} from '@/types/mustering';

/** REST responses wrap their payload in `{ data: ... }` (same envelope as scandevices). */
interface Envelope<T> {
  data: T;
}

export type EntryAction = 'verify' | 'mark_safe';

export const musteringApi = {
  /**
   * Presence + active event — same shape as the SSE `snapshot` frame.
   * GET /api/v1/mustering/status
   */
  status: () => apiClient.get<MusterSnapshotPayload>('/mustering/status'),

  /**
   * Activate a muster drill. 201 Event, or 409 if one is already active.
   * POST /api/v1/mustering/events
   */
  activate: (windowMinutes?: number) =>
    apiClient.post<Envelope<MusterEvent>>('/mustering/events', {
      ...(windowMinutes !== undefined ? { window_minutes: windowMinutes } : {}),
    }),

  /**
   * List past + active events (headers + counts, no entries).
   * GET /api/v1/mustering/events
   */
  listEvents: () => apiClient.get<Envelope<MusterEvent[]>>('/mustering/events'),

  /**
   * One event with full entries + report.
   * GET /api/v1/mustering/events/:id
   */
  getEvent: (id: number) =>
    apiClient.get<Envelope<MusterEvent>>(`/mustering/events/${id}`),

  /**
   * All-clear an event — returns the completed event incl. report.
   * POST /api/v1/mustering/events/:id/all-clear
   */
  allClear: (id: number) =>
    apiClient.post<Envelope<MusterEvent>>(`/mustering/events/${id}/all-clear`, {}),

  /**
   * Cancel a drill.
   * POST /api/v1/mustering/events/:id/cancel
   */
  cancel: (id: number) =>
    apiClient.post<Envelope<MusterEvent>>(`/mustering/events/${id}/cancel`, {}),

  /**
   * Log a break-glass reveal (appends to metadata.unlocks). Returns bare
   * {unlocked: true} — NOT a data-envelope; the body is discarded by callers
   * (the store re-snapshots instead). The type here reflects the real wire
   * shape so nothing falsely tries to unwrap .data.
   * POST /api/v1/mustering/events/:id/unlock
   */
  unlock: (id: number) =>
    apiClient.post<{ unlocked: boolean }>(`/mustering/events/${id}/unlock`, {}),

  /**
   * Verify / mark-safe a single entry.
   * PATCH /api/v1/mustering/events/:id/entries/:entryId
   */
  updateEntry: (
    eventId: number,
    entryId: number,
    action: EntryAction,
    note?: string,
  ) =>
    apiClient.patch<Envelope<{ entry: MusterEntry; counts: MusterCounts }>>(
      `/mustering/events/${eventId}/entries/${entryId}`,
      { action, ...(note ? { note } : {}) },
    ),

  /**
   * Synthesize sightings through the real ingest pipeline (demo control).
   * POST /api/v1/mustering/simulate
   */
  simulate: (sightings: MusterSighting[]) =>
    apiClient.post('/mustering/simulate', { sightings }),

  /**
   * Idempotent demo seed (admin+).
   * POST /api/v1/mustering/seed
   */
  seed: () => apiClient.post('/mustering/seed', {}),
};
