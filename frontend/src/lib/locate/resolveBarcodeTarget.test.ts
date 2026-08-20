import { describe, it, expect, vi, beforeEach } from 'vitest';
import { resolveBarcodeTarget } from './resolveBarcodeTarget';
import { assetsApi } from '@/lib/api/assets';
import { lookupApi } from '@/lib/api/lookup';

vi.mock('@/lib/api/assets', () => ({
  assetsApi: { list: vi.fn(), get: vi.fn() },
}));
vi.mock('@/lib/api/lookup', () => ({
  lookupApi: { byTag: vi.fn() },
}));

const asset = (over: Record<string, unknown> = {}) => ({
  id: 7,
  external_key: '10023',
  name: 'Reel 10023',
  description: null,
  valid_from: '2026-01-01T00:00:00Z',
  valid_to: null,
  metadata: {},
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  tags: [{ id: 1, tag_type: 'rfid', value: '000000000000000000010023' }],
  ...over,
});

const listOk = (assets: unknown[]) => ({
  data: { data: assets, limit: 2, offset: 0, total_count: assets.length },
});

beforeEach(() => {
  vi.clearAllMocks();
  // Default: no barcode-type tag registered, so the fallback path misses.
  vi.mocked(lookupApi.byTag).mockRejectedValue({ response: { status: 404 } });
});

describe('resolveBarcodeTarget', () => {
  it('resolves a barcode matching one asset with one RFID tag', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([asset()]) as never);

    const result = await resolveBarcodeTarget('10023');

    expect(assetsApi.list).toHaveBeenCalledWith({ external_key: '10023', limit: 2 });
    expect(result).toEqual({
      status: 'resolved',
      epc: '000000000000000000010023',
      asset: expect.objectContaining({ id: 7 }),
    });
  });

  it('reports ambiguity when the asset carries several RFID tags', async () => {
    const tags = [
      { id: 1, tag_type: 'rfid', value: '000000000000000000010023' },
      { id: 2, tag_type: 'rfid', value: '000000000000000000010024' },
    ];
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([asset({ tags })]) as never);

    const result = await resolveBarcodeTarget('10023');

    expect(result).toMatchObject({ status: 'ambiguous', tags });
  });

  it('ignores non-RFID tags when choosing the target', async () => {
    const tags = [
      { id: 3, tag_type: 'barcode', value: '10023' },
      { id: 1, tag_type: 'rfid', value: '000000000000000000010023' },
    ];
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([asset({ tags })]) as never);

    await expect(resolveBarcodeTarget('10023')).resolves.toMatchObject({
      status: 'resolved',
      epc: '000000000000000000010023',
    });
  });

  it('reports no-rfid-tag when the matched asset has no RFID tag', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([asset({ tags: [] })]) as never);

    await expect(resolveBarcodeTarget('10023')).resolves.toMatchObject({
      status: 'no-rfid-tag',
    });
  });

  it('falls back to a registered barcode tag when no external_key matches', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([]) as never);
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'asset', entity_id: 7 } },
    } as never);
    vi.mocked(assetsApi.get).mockResolvedValue({ data: { data: asset() } } as never);

    const result = await resolveBarcodeTarget('S04163');

    expect(lookupApi.byTag).toHaveBeenCalledWith('barcode', 'S04163');
    expect(assetsApi.get).toHaveBeenCalledWith(7);
    expect(result).toMatchObject({ status: 'resolved' });
  });

  it('skips the external_key call for a barcode the filter would reject', async () => {
    await resolveBarcodeTarget('LOT/4163');

    expect(assetsApi.list).not.toHaveBeenCalled();
    expect(lookupApi.byTag).toHaveBeenCalledWith('barcode', 'LOT/4163');
  });

  // The rfidCollect convention prints the tag's own value on the label, and
  // commissioning stores it leading-zero-stripped, so the bench asset carries
  // an RFID tag valued exactly "10021" and no barcode tag at all. Confirming
  // the value against the registry is not the literal-EPC fallback this
  // feature rejects — a value the registry does not know is still refused.
  it('falls back to an RFID tag whose registered value is the barcode', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([]) as never);
    vi.mocked(lookupApi.byTag).mockImplementation(((type: string) =>
      type === 'rfid'
        ? Promise.resolve({ data: { data: { entity_type: 'asset', entity_id: 7 } } })
        : Promise.reject({ response: { status: 404 } })) as never);
    vi.mocked(assetsApi.get).mockResolvedValue({ data: { data: asset() } } as never);

    const result = await resolveBarcodeTarget('10021');

    expect(lookupApi.byTag).toHaveBeenCalledWith('rfid', '10021');
    expect(result).toMatchObject({ status: 'resolved' });
  });

  it('prefers a barcode tag over an RFID tag of the same value', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([]) as never);
    vi.mocked(lookupApi.byTag).mockResolvedValue({
      data: { data: { entity_type: 'asset', entity_id: 7 } },
    } as never);
    vi.mocked(assetsApi.get).mockResolvedValue({ data: { data: asset() } } as never);

    await resolveBarcodeTarget('10021');

    expect(lookupApi.byTag).toHaveBeenCalledWith('barcode', '10021');
    expect(lookupApi.byTag).not.toHaveBeenCalledWith('rfid', '10021');
  });

  it('reports no-asset when no path matches', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(listOk([]) as never);

    await expect(resolveBarcodeTarget('10023')).resolves.toEqual({ status: 'no-asset' });
  });

  it('does not resolve when the external_key filter matches several assets', async () => {
    vi.mocked(assetsApi.list).mockResolvedValue(
      listOk([asset(), asset({ id: 8 })]) as never
    );

    await expect(resolveBarcodeTarget('10023')).resolves.toEqual({ status: 'no-asset' });
  });

  it('surfaces a transport failure as an error, not a miss', async () => {
    vi.mocked(assetsApi.list).mockRejectedValue(new Error('network down'));

    await expect(resolveBarcodeTarget('10023')).resolves.toMatchObject({ status: 'error' });
  });

  it('rejects an empty barcode without calling the API', async () => {
    await expect(resolveBarcodeTarget('   ')).resolves.toEqual({ status: 'no-asset' });
    expect(assetsApi.list).not.toHaveBeenCalled();
    expect(lookupApi.byTag).not.toHaveBeenCalled();
  });
});
