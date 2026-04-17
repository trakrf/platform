import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { InventorySettingsPanel } from '../InventorySettingsPanel';

afterEach(cleanup);

function defaultProps(overrides: Partial<React.ComponentProps<typeof InventorySettingsPanel>> = {}) {
  return {
    isOpen: true,
    onToggle: vi.fn(),
    rfPower: 30,
    onRfPowerChange: vi.fn(),
    showLeadingZeros: false,
    onShowLeadingZerosChange: vi.fn(),
    autoClearOnSave: false,
    onAutoClearOnSaveChange: vi.fn(),
    ...overrides,
  };
}

describe('InventorySettingsPanel auto-clear toggle', () => {
  it('renders the Auto-clear after Save checkbox unchecked by default', () => {
    render(<InventorySettingsPanel {...defaultProps()} />);

    const checkbox = screen.getByRole('checkbox', { name: /auto-clear after save/i });
    expect(checkbox).toBeInTheDocument();
    expect(checkbox).not.toBeChecked();
  });

  it('reflects autoClearOnSave=true as checked', () => {
    render(<InventorySettingsPanel {...defaultProps({ autoClearOnSave: true })} />);

    const checkbox = screen.getByRole('checkbox', { name: /auto-clear after save/i });
    expect(checkbox).toBeChecked();
  });

  it('invokes onAutoClearOnSaveChange when toggled', () => {
    const onAutoClearOnSaveChange = vi.fn();
    render(
      <InventorySettingsPanel {...defaultProps({ onAutoClearOnSaveChange })} />
    );

    fireEvent.click(screen.getByRole('checkbox', { name: /auto-clear after save/i }));

    expect(onAutoClearOnSaveChange).toHaveBeenCalledWith(true);
  });

  it('hides the toggle when the panel is closed', () => {
    render(<InventorySettingsPanel {...defaultProps({ isOpen: false })} />);

    expect(
      screen.queryByRole('checkbox', { name: /auto-clear after save/i })
    ).not.toBeInTheDocument();
  });
});
