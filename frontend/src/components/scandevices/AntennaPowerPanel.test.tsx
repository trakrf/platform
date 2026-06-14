import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, act, cleanup } from '@testing-library/react';
import type { AntennaPower, ScanPoint } from '@/types/scandevices';

const setPower = vi.fn(() => Promise.resolve({ status: 'accepted', request_id: 'x' }));
let antennas: AntennaPower[] = [];
let scanPoints: ScanPoint[] = [];

vi.mock('@/hooks/scandevices', () => ({
  useScanPoints: () => ({ scanPoints, isLoading: false }),
}));
vi.mock('@/hooks/scandevices/useAntennaPower', () => ({
  useAntennaPower: () => ({ antennas }),
  useSetAntennaPower: () => ({ setPower, isSetting: false }),
}));
vi.mock('react-hot-toast', () => ({
  default: { success: vi.fn(), error: vi.fn() },
}));

import { AntennaPowerPanel } from './AntennaPowerPanel';

const point = (port: number): ScanPoint =>
  ({ id: port, antenna_port: port, scan_device_id: 1, org_id: 1, name: `A${port}` } as ScanPoint);

describe('AntennaPowerPanel', () => {
  beforeEach(() => {
    setPower.mockClear();
    antennas = [];
    scanPoints = [];
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
    cleanup();
  });

  it('renders one slider per provisioned antenna port', () => {
    scanPoints = [point(1), point(2)];
    antennas = [
      { antenna_port: 1, power_dbm: 30 },
      { antenna_port: 2, power_dbm: 22 },
    ];
    render(<AntennaPowerPanel deviceId={1} />);
    expect(screen.getByLabelText('Antenna 1 transmit power')).toBeInTheDocument();
    expect(screen.getByLabelText('Antenna 2 transmit power')).toBeInTheDocument();
  });

  it('debounces a slider change into a setPower call', () => {
    scanPoints = [point(1)];
    antennas = [{ antenna_port: 1, power_dbm: 30 }];
    render(<AntennaPowerPanel deviceId={1} />);

    fireEvent.change(screen.getByLabelText('Antenna 1 transmit power'), { target: { value: '20' } });
    expect(setPower).not.toHaveBeenCalled(); // debounced
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    expect(setPower).toHaveBeenCalledWith({ powers: { '1': 20 }, force: false });
  });

  it('shows a force button when the reader is busy', () => {
    scanPoints = [point(1)];
    antennas = [{ antenna_port: 1, power_dbm: 30, status: 'busy' }];
    render(<AntennaPowerPanel deviceId={1} />);
    expect(screen.getByRole('button', { name: /force logout/i })).toBeInTheDocument();
  });

  it('prompts to add antennas when none are provisioned', () => {
    render(<AntennaPowerPanel deviceId={1} />);
    expect(screen.getByText(/no antennas provisioned/i)).toBeInTheDocument();
  });
});
