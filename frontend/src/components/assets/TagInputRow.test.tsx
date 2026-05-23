import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import { TagInputRow } from './TagInputRow';

describe('TagInputRow', () => {
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
    render(<TagInputRow {...defaultProps} />);

    expect(screen.getByText('RFID')).toBeInTheDocument();
  });

  it('renders input field with placeholder', () => {
    render(<TagInputRow {...defaultProps} />);

    expect(screen.getByPlaceholderText('Enter tag number...')).toBeInTheDocument();
  });

  it('displays the provided value', () => {
    render(<TagInputRow {...defaultProps} value="TAG-12345" />);

    const input = screen.getByDisplayValue('TAG-12345');
    expect(input).toBeInTheDocument();
  });

  it('calls onValueChange when input value changes', () => {
    const onValueChange = vi.fn();
    render(<TagInputRow {...defaultProps} onValueChange={onValueChange} />);

    const input = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(input, { target: { value: 'NEW-TAG' } });

    expect(onValueChange).toHaveBeenCalledWith('NEW-TAG');
  });

  it('does not render remove button when onRemove is not provided', () => {
    render(<TagInputRow {...defaultProps} />);

    expect(screen.queryByLabelText('Remove tag')).not.toBeInTheDocument();
  });

  it('renders remove button when onRemove is provided', () => {
    const onRemove = vi.fn();
    render(<TagInputRow {...defaultProps} onRemove={onRemove} />);

    expect(screen.getByLabelText('Remove tag')).toBeInTheDocument();
  });

  it('calls onRemove when remove button is clicked', () => {
    const onRemove = vi.fn();
    render(<TagInputRow {...defaultProps} onRemove={onRemove} />);

    fireEvent.click(screen.getByLabelText('Remove tag'));

    expect(onRemove).toHaveBeenCalledTimes(1);
  });

  it('disables input when disabled prop is true', () => {
    render(<TagInputRow {...defaultProps} disabled={true} />);

    const input = screen.getByPlaceholderText('Enter tag number...');
    expect(input).toBeDisabled();
  });

  it('disables remove button when disabled prop is true', () => {
    const onRemove = vi.fn();
    render(<TagInputRow {...defaultProps} onRemove={onRemove} disabled={true} />);

    const removeButton = screen.getByLabelText('Remove tag');
    expect(removeButton).toBeDisabled();
  });

  it('displays error message when error prop is provided', () => {
    render(<TagInputRow {...defaultProps} error="Tag value is required" />);

    expect(screen.getByText('Tag value is required')).toBeInTheDocument();
  });

  it('applies error styling to input when error prop is provided', () => {
    const { container } = render(<TagInputRow {...defaultProps} error="Invalid tag" />);

    const input = container.querySelector('input');
    expect(input?.className).toContain('border-red-500');
  });

  it('applies normal styling to input when no error', () => {
    const { container } = render(<TagInputRow {...defaultProps} />);

    const input = container.querySelector('input');
    expect(input?.className).toContain('border-gray-300');
    expect(input?.className).not.toContain('border-red-500');
  });

  describe('readOnly mode (tag identity is immutable)', () => {
    it('renders value as text, not an input, when readOnly is true', () => {
      const { container } = render(
        <TagInputRow {...defaultProps} value="TAG-12345" readOnly={true} />,
      );

      expect(container.querySelector('input')).not.toBeInTheDocument();
      expect(screen.getByText('TAG-12345')).toBeInTheDocument();
    });

    it('marks the read-only value with aria-readonly', () => {
      render(<TagInputRow {...defaultProps} value="TAG-12345" readOnly={true} />);

      expect(screen.getByText('TAG-12345')).toHaveAttribute('aria-readonly', 'true');
    });

    it('still renders the remove button when readOnly', () => {
      const onRemove = vi.fn();
      render(
        <TagInputRow
          {...defaultProps}
          value="TAG-12345"
          readOnly={true}
          onRemove={onRemove}
        />,
      );

      expect(screen.getByLabelText('Remove tag')).toBeInTheDocument();
    });

    it('calls onRemove when the remove button is clicked in readOnly mode', () => {
      const onRemove = vi.fn();
      render(
        <TagInputRow
          {...defaultProps}
          value="TAG-12345"
          readOnly={true}
          onRemove={onRemove}
        />,
      );

      fireEvent.click(screen.getByLabelText('Remove tag'));

      expect(onRemove).toHaveBeenCalledTimes(1);
    });

    it('renders an editable input when readOnly is false or omitted', () => {
      render(<TagInputRow {...defaultProps} value="TAG-12345" />);

      expect(screen.getByDisplayValue('TAG-12345')).toBeInTheDocument();
    });
  });
});
