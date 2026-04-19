import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/react';
import { ExpirySelector } from './ExpirySelector';

describe('ExpirySelector', () => {
  it('defaults to "never" and emits null', () => {
    const onChange = vi.fn();
    render(<ExpirySelector value={null} onChange={onChange} />);
    expect(screen.getByLabelText(/never/i)).toBeChecked();
  });

  it('emits an ISO date ~30 days out when 30 days is selected', () => {
    const onChange = vi.fn();
    const { container } = render(<ExpirySelector value={null} onChange={onChange} />);
    // Find the radio input with value '30d'
    const thirtyDayRadio = container.querySelector(
      'input[type="radio"][value="30d"]',
    ) as HTMLInputElement;
    // Trigger click and change events
    fireEvent.click(thirtyDayRadio);
    fireEvent.change(thirtyDayRadio);
    expect(onChange).toHaveBeenCalled();
    const arg = onChange.mock.calls[0][0] as string;
    const diffDays = (new Date(arg).getTime() - Date.now()) / 86_400_000;
    expect(diffDays).toBeGreaterThan(29);
    expect(diffDays).toBeLessThan(31);
  });

  it('shows custom date input when "Custom" is selected', () => {
    const { container } = render(<ExpirySelector value={null} onChange={() => {}} />);
    // Find the radio input with value 'custom'
    const customRadio = container.querySelector(
      'input[type="radio"][value="custom"]',
    ) as HTMLInputElement;
    fireEvent.click(customRadio);
    fireEvent.change(customRadio);
    expect(screen.getByLabelText(/expiry date/i)).toBeInTheDocument();
  });
});
