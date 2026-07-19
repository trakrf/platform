import { apiClient } from '../client';

/**
 * Kits API Client (TRA-1033)
 *
 * Internal-only endpoints from TRA-1032 — request/response shapes are FROZEN
 * against backend/internal/models/kit/kit.go. All ids are bigint surrogates
 * serialized as JSON numbers. Both POSTs require entitlement (402) and
 * Operator+ role; errors arrive in the RFC 7807 envelope.
 */

export interface CommissionMemberRequest {
  epc: string;
  role?: string;
  name?: string;
}

export interface CommissionRequest {
  label: string;
  members: CommissionMemberRequest[]; // backend requires >= 2
}

export interface KitMember {
  asset_id: number;
  role: string | null;
  name: string;
  epcs: string[];
}

export interface VerificationSummary {
  result: 'complete' | 'incomplete';
  verified_at: string;
}

export interface Kit {
  id: number;
  label: string;
  status: 'active' | 'closed';
  created_at: string;
  updated_at: string;
  members: KitMember[];
  latest_verification: VerificationSummary | null;
}

export interface KitResponse {
  data: Kit;
}

export interface VerifyRequest {
  epcs: string[];
}

export interface VerifySeenMember {
  asset_id: number;
  role: string | null;
  name: string;
}

export interface VerifyMissingMember {
  asset_id: number;
  role: string | null;
  name: string;
  epcs: string[];
}

export interface VerifyKitResult {
  kit_id: number;
  label: string;
  result: 'complete' | 'incomplete';
  seen: VerifySeenMember[];
  missing: VerifyMissingMember[];
}

export interface VerifyUnexpected {
  asset_id: number;
  epc: string;
  name: string;
  belongs_to_kit_id: number;
  belongs_to_kit_label: string;
}

// Frozen dock-check shape: top-level, no {data} envelope (TRA-1032).
export interface VerifyResponse {
  kits: VerifyKitResult[];
  unexpected: VerifyUnexpected[];
  unknown_epcs: string[];
}

export const kitsApi = {
  /**
   * Commission a kit from scanned members.
   * POST /api/v1/kits — 201 with Location header; 409 conflict when a member
   * already belongs to another active kit (owning kit label in error.detail).
   */
  commission: (request: CommissionRequest) =>
    apiClient.post<KitResponse>('/kits', request),

  /**
   * Dock-check verification of a scan session's EPCs.
   * POST /api/v1/kits/verify — persists an audit row per kit server-side.
   */
  verify: (request: VerifyRequest) =>
    apiClient.post<VerifyResponse>('/kits/verify', request),
};
