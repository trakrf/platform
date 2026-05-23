import { lookupApi } from '@/lib/api/lookup';

export interface ConflictSelf {
  entityType: 'asset' | 'location';
  entityId: number;
}

/**
 * Checks whether an RFID tag value is already attached to a different entity.
 * Returns a human-readable conflict message, or null when the value is free,
 * not found, or already belongs to `self` (the entity currently being edited).
 * Best-effort: any unexpected error resolves to null — the save-time 409 is
 * the correctness backstop.
 */
export async function checkTagConflict(
  value: string,
  self?: ConflictSelf,
): Promise<string | null> {
  const trimmed = value.trim();
  if (!trimmed) return null;
  try {
    const response = await lookupApi.byTag('rfid', trimmed);
    const result = response.data.data;
    if (self && result.entity_type === self.entityType && result.entity_id === self.entityId) {
      return null;
    }
    // TRA-816: the lookup may surface a tag whose parent has been soft-deleted
    // (orphan). The asset/location envelope is then missing — never show the
    // surrogate `${type} #${id}` string, the user has no UI path to that row.
    const name = result.asset?.name ?? result.location?.name;
    if (!name) {
      return `Tag already attached to a ${result.entity_type} you no longer have access to — contact support to release it.`;
    }
    return `Tag already attached to ${result.entity_type} "${name}" — remove it there before attaching here.`;
  } catch (err: unknown) {
    // 404 = not attached anywhere; any other error = best-effort skip.
    return null;
  }
}
