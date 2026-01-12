import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import { TagIdentifierInputRow } from './TagIdentifierInputRow';

describe('TagIdentifierInputRow', () => {
  const defaultProps = {
    type: 'rfid' as const,
    value: '',
    onTypeChange: vi.fn(),
    onValueChange: vi.fn(),
  };

  afterEach(() => {
    cleanup();
  });

  it('renders with RFID label', () => {
    render(<TagIdentifierInputRow {...defaultProps} />);

    expect(screen.getByText('RFID')).toBeInTheDocument();
  });

  it('renders input field with placeholder', () => {
    render(<TagIdentifierInputRow {...defaultProps} />);

    expect(screen.getByPlaceholderText('Enter tag number...')).toBeInTheDocument();
  });

  it('displays the provided value', () => {
    render(<TagIdentifierInputRow {...defaultProps} value="TAG-12345" />);

    const input = screen.getByDisplayValue('TAG-12345');
    expect(input).toBeInTheDocument();
  });

  it('calls onValueChange when input value changes', () => {
    const onValueChange = vi.fn();
    render(<TagIdentifierInputRow {...defaultProps} onValueChange={onValueChange} />);

    const input = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(input, { target: { value: 'NEW-TAG' } });

    expect(onValueChange).toHaveBeenCalledWith('NEW-TAG');
  });

  it('does not render remove button when onRemove is not provided', () => {
    render(<TagIdentifierInputRow {...defaultProps} />);

    expect(screen.queryByLabelText('Remove tag')).not.toBeInTheDocument();
  });

  it('renders remove button when onRemove is provided', () => {
    const onRemove = vi.fn();
    render(<TagIdentifierInputRow {...defaultProps} onRemove={onRemove} />);

    expect(screen.getByLabelText('Remove tag')).toBeInTheDocument();
  });

  it('calls onRemove when remove button is clicked', () => {
    const onRemove = vi.fn();
    render(<TagIdentifierInputRow {...defaultProps} onRemove={onRemove} />);

    fireEvent.click(screen.getByLabelText('Remove tag'));

    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it('disables input when disabled prop is true', () => {
    render(<TagIdentifierInputRow {...defaultProps} disabled={true} />);

    const input = screen.getByPlaceholderText('Enter tag number...');
    expect(input).toBeDisabled();
  });

  it('disables remove button when disabled prop is true', () => {
    const onRemove = vi.fn();
    render(<TagIdentifierInputRow {...defaultProps} onRemove={onRemove} disabled={true} />);

    const removeButton = screen.getByLabelText('Remove tag');
    expect(removeButton).toBeDisabled();
  });

  it('displays error message when error prop is provided', () => {
    render(<TagIdentifierInputRow {...defaultProps} error="Tag value is required" />);

    expect(screen.getByText('Tag value is required')).toBeInTheDocument();
  });

  it('applies error styling to input when error prop is provided', () => {
    const { container } = render(<TagIdentifierInputRow {...defaultProps} error="Invalid tag" />);

    const input = container.querySelector('input');
    expect(input?.className).toContain('border-red-500');
  });

  it('applies normal styling to input when no error', () => {
    const { container } = render(<TagIdentifierInputRow {...defaultProps} />);

    const input = container.querySelector('input');
    expect(input?.className).toContain('border-gray-300');
    expect(input?.className).not.toContain('border-red-500');
  });
});
