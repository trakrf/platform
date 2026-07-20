import type { TagInfo } from '@/stores/tagStore';
import type { CommissionRequest } from '@/lib/api/kits';

/**
 * Kit scan-flow helpers (TRA-1033).
 *
 * Location tags are excluded from kit flows; unknown EPCs stay — the backend
 * auto-creates minimal assets for them on commission (TRA-1032), and verify
 * reports them back as unknown_epcs.
 */
export function selectKitMemberTags(tags: TagInfo[]): TagInfo[] {
  return tags.filter((t) => t.type !== 'location');
}

/**
 * EPCs to send to POST /kits/verify. Raw scanned values — the server
 * normalizes (case, non-hex chars, leading zeros) before matching.
 */
export function collectVerifyEpcs(tags: TagInfo[]): string[] {
  return selectKitMemberTags(tags).map((t) => t.epc);
}

/**
 * The Howmet QA fields (TRA-1033 slide alignment). All optional — Lot #
 * (label) stays the only required entry; the envelope still travels.
 */
export const KIT_QA_FIELDS: { key: string; label: string }[] = [
  { key: 'part', label: 'Part #' },
  { key: 'heat', label: 'Heat #' },
  { key: 'operator', label: 'Operator' },
  { key: 'date', label: 'Date' },
  { key: 'vendor', label: 'Vendor' },
];

export function buildCommissionRequest(
  label: string,
  tags: TagInfo[],
  roles: Record<string, string>,
  metadata?: Record<string, string>
): CommissionRequest {
  const request: CommissionRequest = {
    label: label.trim(),
    members: selectKitMemberTags(tags).map((t) => {
      const role = (roles[t.epc] ?? '').trim();
      return role ? { epc: t.epc, role } : { epc: t.epc };
    }),
  };
  const cleaned = Object.fromEntries(
    Object.entries(metadata ?? {})
      .map(([k, v]) => [k, v.trim()])
      .filter(([, v]) => v !== '')
  );
  if (Object.keys(cleaned).length > 0) {
    request.metadata = cleaned;
  }
  return request;
}

/**
 * Deep-link into Locate mode pre-armed with an EPC (existing #locate?epc=
 * pattern). return=kits arms the "back to kit results" button in LocateScreen.
 */
export function buildLocateHash(epc: string): string {
  return `#locate?epc=${encodeURIComponent(epc)}&return=kits`;
}
