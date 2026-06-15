import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { AntennaRow } from './AntennaRow';

const OPTIONS = [
  { value: '', label: '— set location —' },
  { value: '100', label: 'Receiving' },
  { value: '101', label: 'Staging' },
];

function renderRow(over: Partial<React.ComponentProps<typeof AntennaRow>> = {}) {
  const props = {
    antenna: 1,
    enabled: true,
    locationId: 100 as number | null,
    locationOptions: OPTIONS,
    power: 28,
    min: 10,
    max: 31.5,
    step: 0.5,
    onPowerChange: vi.fn(),
    onToggleEnabled: vi.fn(() => Promise.resolve()),
    onSetLocation: vi.fn(() => Promise.resolve()),
    ...over,
  };
  render(<AntennaRow {...props} />);
  return props;
}

describe('AntennaRow', () => {
  afterEach(() => cleanup());

  it('renders the antenna number, enable checkbox, location label, and power readout', () => {
    renderRow();
    expect(screen.getByText('1')).toBeInTheDocument();
    expect(screen.getByLabelText(/enable antenna 1/i)).toBeChecked();
    expect(screen.getByText('Receiving')).toBeInTheDocument();
    expect(screen.getByText('28.0 dBm')).toBeInTheDocument();
  });

  it('shows the placeholder when no location is set', () => {
    renderRow({ locationId: null });
    expect(screen.getByText('— set location —')).toBeInTheDocument();
  });

  it('renders a power slider bounded by min/max/step', () => {
    renderRow();
    const slider = screen.getByLabelText(/antenna 1 transmit power/i);
    expect(slider).toHaveAttribute('min', '10');
    expect(slider).toHaveAttribute('max', '31.5');
    expect(slider).toHaveAttribute('step', '0.5');
  });

  it('calls onPowerChange with the parsed number when the slider moves', () => {
    const { onPowerChange } = renderRow();
    fireEvent.change(screen.getByLabelText(/antenna 1 transmit power/i), {
      target: { value: '15' },
    });
    expect(onPowerChange).toHaveBeenCalledWith(15);
  });

  it('dims the row when the antenna is disabled', () => {
    const { container } = render(
      <AntennaRow
        antenna={1}
        enabled={false}
        locationId={null}
        locationOptions={OPTIONS}
        power={28}
        min={10}
        max={31.5}
        step={0.5}
        onPowerChange={vi.fn()}
        onToggleEnabled={vi.fn(() => Promise.resolve())}
        onSetLocation={vi.fn(() => Promise.resolve())}
      />
    );
    expect(container.firstChild).toHaveClass('opacity-50');
  });
});
