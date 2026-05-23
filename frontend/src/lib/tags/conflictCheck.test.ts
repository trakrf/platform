import { describe, it, expect, vi, beforeEach } from 'vitest';
import { checkTagConflict } from './conflictCheck';
import { lookupApi } from '@/lib/api/lookup';

vi.mock('@/lib/api/lookup');

describe('checkTagConflict', () => {
  beforeEach(() => vi.clearAllMocks());

  it('returns null for an empty value', async () => {
    expect(await checkTagConflict('   ')).toBeNull();
  });

  it('returns null when the tag is not attached anywhere (404)', async () => {
    vi.mocked(lookupApi.byTag).mockRejectedValue({ response: { status: 404 } });
    expect(await checkTagConflict('E2-FREE')).toBeNull();
  });

  it('returns a message naming a conflicting location', async () => {
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'location', entity_id: 9, location: { id: 9, name: 'Dock 3' } } },
    } as never);
    const msg = await checkTagConflict('E2-TAKEN');
    expect(msg).toContain('location');
    expect(msg).toContain('Dock 3');
  });

  it('returns null when the hit is the entity being edited', async () => {
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'asset', entity_id: 42, asset: { id: 42, name: 'Forklift' } } },
    } as never);
    expect(await checkTagConflict('E2-OWN', { entityType: 'asset', entityId: 42 })).toBeNull();
  });

  it('returns a no-access message when the entity name is missing (TRA-816 orphan)', async () => {
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'asset', entity_id: 558328969 } },
    } as never);
    const msg = await checkTagConflict('E2-ORPHAN');
    expect(msg).toContain('no longer have access');
    expect(msg).not.toContain('#558328969');
    expect(msg).not.toContain('asset "asset');
  });

  it('returns null on an unexpected error (best-effort)', async () => {
    vi.mocked(lookupApi.byTag).mockRejectedValue({ response: { status: 500 } });
    expect(await checkTagConflict('E2-ERR')).toBeNull();
  });
});
