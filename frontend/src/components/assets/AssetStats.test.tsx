import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { AssetStats } from './AssetStats';
import { useAssetStore } from '@/stores';
import type { Asset } from '@/types/assets';

vi.mock('@/stores');

describe('AssetStats', () => {
  afterEach(() => {
    cleanup();
  });

  const mockAssets: Asset[] = [
    {
      id: 1,
      org_id: 1,
      identifier: 'DEV-001',
      name: 'Device 1',
      type: 'device',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 2,
      org_id: 1,
      identifier: 'DEV-002',
      name: 'Device 2',
      type: 'device',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: false,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
    {
      id: 3,
      org_id: 1,
      identifier: 'PER-001',
      name: 'Person 1',
      type: 'person',
      description: '',
      valid_from: '2024-01-01T00:00:00Z',
      valid_to: null,
      metadata: {},
      is_active: true,
      created_at: '2024-01-01T00:00:00Z',
      updated_at: '2024-01-01T00:00:00Z',
      deleted_at: null,
    },
  ];

  beforeEach(() => {
    const mockByIdMap = new Map(mockAssets.map((asset) => [asset.id, asset]));
    const mockStore = {
      cache: { byId: mockByIdMap },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));
  });

  it('renders total asset count', () => {
    render(<AssetStats />);

    expect(screen.getByText('Total Assets')).toBeInTheDocument();
    expect(screen.getByText('3')).toBeInTheDocument();
  });

  it('renders active and inactive counts', () => {
    render(<AssetStats />);

    expect(screen.getByText('Active')).toBeInTheDocument();
    expect(screen.getByText('Inactive')).toBeInTheDocument();

    // Check that stats are rendered (may appear multiple times in grid)
    const activeCount = screen.getAllByText('2');
    expect(activeCount.length).toBeGreaterThan(0);

    const inactiveCount = screen.getAllByText('1');
    expect(inactiveCount.length).toBeGreaterThan(0);
  });

  it('handles empty assets array', () => {
    const mockStore = {
      cache: { byId: new Map() },
    };
    (useAssetStore as any).mockImplementation((selector: any) => selector(mockStore));

    render(<AssetStats />);

    // Check that zeros are rendered (may appear multiple times)
    const zeros = screen.getAllByText('0');
    expect(zeros.length).toBeGreaterThan(0);
  });

  it('applies custom className', () => {
    const { container } = render(<AssetStats className="custom-stats-class" />);
    const statsDiv = container.firstChild as HTMLElement;

    expect(statsDiv.className).toContain('custom-stats-class');
  });
});
