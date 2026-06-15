import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ReaderPointsSection } from './ReaderPointsSection';
import type { ScanDevice } from '@/types/scandevices';

// Stub the heavy, hook-backed children so this test isolates the type-aware
// routing logic (which child renders for which device profile).
vi.mock('./AntennaSettingsPanel', () => ({
  AntennaSettingsPanel: ({ deviceId }: { deviceId: number }) => (
    <div data-testid="antenna-settings-panel">antennas:{deviceId}</div>
  ),
}));
vi.mock('./SinglePointLocationField', () => ({
  SinglePointLocationField: ({ device }: { device: ScanDevice }) => (
    <div data-testid="single-point-field">location:{device.id}</div>
  ),
}));

const device = (over: Partial<ScanDevice>): ScanDevice => ({
  id: 10,
  org_id: 1,
  external_key: 'reader_1',
  name: 'Reader 1',
  type: 'csl_cs463',
  transport: 'mqtt',
  publish_topic: null,
  serial_number: null,
  model: null,
  description: '',
  metadata: {},
  valid_from: '2024-01-01T00:00:00Z',
  valid_to: null,
  is_active: true,
  created_at: '2024-01-01T00:00:00Z',
  updated_at: null,
  deleted_at: null,
  ...over,
});

describe('ReaderPointsSection', () => {
  afterEach(() => cleanup());

  it('renders the consolidated antenna settings panel for a multi-point CS463', () => {
    render(<ReaderPointsSection device={device({ type: 'csl_cs463', transport: 'mqtt' })} />);
    expect(screen.getByTestId('antenna-settings-panel')).toBeInTheDocument();
    expect(screen.queryByTestId('single-point-field')).not.toBeInTheDocument();
  });

  it('renders the single device-level location field for a GL-S10', () => {
    render(<ReaderPointsSection device={device({ type: 'gl_s10', transport: 'mqtt' })} />);
    expect(screen.getByTestId('single-point-field')).toBeInTheDocument();
    expect(screen.queryByTestId('antenna-settings-panel')).not.toBeInTheDocument();
  });

  it('renders no location control for a web_ble handheld', () => {
    render(<ReaderPointsSection device={device({ type: 'csl_cs108', transport: 'web_ble' })} />);
    expect(screen.queryByTestId('antenna-settings-panel')).not.toBeInTheDocument();
    expect(screen.queryByTestId('single-point-field')).not.toBeInTheDocument();
    expect(screen.getByText(/handheld/i)).toBeInTheDocument();
  });
});
