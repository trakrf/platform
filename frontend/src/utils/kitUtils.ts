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

export function buildCommissionRequest(
  label: string,
  tags: TagInfo[],
  roles: Record<string, string>
): CommissionRequest {
  return {
    label: label.trim(),
    members: selectKitMemberTags(tags).map((t) => {
      const role = (roles[t.epc] ?? '').trim();
      return role ? { epc: t.epc, role } : { epc: t.epc };
    }),
  };
}

/**
 * Deep-link into Locate mode pre-armed with an EPC (existing #locate?epc=
 * pattern). return=kits arms the "back to kit results" button in LocateScreen.
 */
export function buildLocateHash(epc: string): string {
  return `#locate?epc=${encodeURIComponent(epc)}&return=kits`;
}
