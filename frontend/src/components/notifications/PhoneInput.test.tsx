import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { PhoneInput } from './PhoneInput';

const HINT = 'Country code, then the number without the leading 0';

describe('PhoneInput', () => {
  afterEach(cleanup);

  it('shows the hint when there is no error', () => {
    render(<PhoneInput id="p" value={{ code: '', number: '' }} onChange={vi.fn()} />);
    expect(screen.getByText(HINT)).toBeInTheDocument();
  });

  it('replaces the hint with the errors', () => {
    render(
      <PhoneInput
        id="p"
        value={{ code: '', number: '0' }}
        onChange={vi.fn()}
        errors={{ phoneCode: 'Enter the country code, e.g. +1', phoneNumber: 'Leave out the leading 0' }}
      />
    );
    expect(screen.queryByText(HINT)).not.toBeInTheDocument();
    expect(screen.getByText('Enter the country code, e.g. +1')).toBeInTheDocument();
    expect(screen.getByText('Leave out the leading 0')).toBeInTheDocument();
    expect(screen.getByTestId('p-code')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.getByTestId('p-number')).toHaveAttribute('aria-invalid', 'true');
  });

  it('adds the plus sign to the country code', () => {
    const onChange = vi.fn();
    render(<PhoneInput id="p" value={{ code: '', number: '7911' }} onChange={onChange} />);
    fireEvent.change(screen.getByTestId('p-code'), { target: { value: '44' } });
    expect(onChange).toHaveBeenCalledWith({ code: '+44', number: '7911' });
  });
});
