import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetForm } from './AssetForm';
import type { Asset } from '@/types/assets';

describe('AssetForm', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    identifier: 'LAP-001',
    name: 'Test Laptop',
    type: 'device',
    description: 'Test description',
    valid_from: '2024-01-01T00:00:00Z',
    valid_to: null,
    metadata: {},
    is_active: true,
    created_at: '2024-01-01T00:00:00Z',
    updated_at: '2024-01-01T00:00:00Z',
    deleted_at: null,
  };

  it('renders create mode form', () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    expect(screen.getByLabelText(/Identifier/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Type/)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Create Asset/ })).toBeInTheDocument();
  });

  it('renders edit mode form with asset data', () => {
    render(
      <AssetForm mode="edit" asset={mockAsset} onSubmit={mockOnSubmit} onCancel={mockOnCancel} />
    );

    expect(screen.getByDisplayValue('LAP-001')).toBeInTheDocument();
    expect(screen.getByDisplayValue('Test Laptop')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Update Asset/ })).toBeInTheDocument();
  });

  it('disables identifier field in edit mode', () => {
    render(
      <AssetForm mode="edit" asset={mockAsset} onSubmit={mockOnSubmit} onCancel={mockOnCancel} />
    );

    const identifierInput = screen.getByDisplayValue('LAP-001');
    expect(identifierInput).toBeDisabled();
  });

  it('validates required fields', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    // Clear name field and submit
    const nameInput = screen.getByLabelText(/Name/);
    fireEvent.change(nameInput, { target: { value: '' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(screen.getByText('Name is required')).toBeInTheDocument();
    });

    expect(mockOnSubmit).not.toHaveBeenCalled();
  });

  it('validates identifier format', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    const identifierInput = screen.getByLabelText(/Identifier/);
    fireEvent.change(identifierInput, { target: { value: 'invalid id!' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(
        screen.getByText(/must contain only letters, numbers, hyphens, and underscores/)
      ).toBeInTheDocument();
    });
  });

  it('calls onSubmit with form data when valid', async () => {
    mockOnSubmit.mockResolvedValue(undefined);
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    fireEvent.change(screen.getByLabelText(/Identifier/), { target: { value: 'TEST-001' } });
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Test Asset' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalled();
    });
  });

  it('calls onCancel when cancel button is clicked', () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    const cancelButton = screen.getByRole('button', { name: /Cancel/ });
    fireEvent.click(cancelButton);

    expect(mockOnCancel).toHaveBeenCalledTimes(1);
  });

  it('shows loading state during submission', () => {
    render(
      <AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} loading={true} />
    );

    expect(screen.getByText('Saving...')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Saving/ })).toBeDisabled();
  });

  it('displays error message when provided', () => {
    render(
      <AssetForm
        mode="create"
        onSubmit={mockOnSubmit}
        onCancel={mockOnCancel}
        error="Test error message"
      />
    );

    expect(screen.getByText('Test error message')).toBeInTheDocument();
  });

  it('clears field error when user starts typing', async () => {
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    // Trigger validation error
    const nameInput = screen.getByLabelText(/Name/);
    fireEvent.change(nameInput, { target: { value: '' } });
    fireEvent.click(screen.getByRole('button', { name: /Create Asset/ }));

    await waitFor(() => {
      expect(screen.getByText('Name is required')).toBeInTheDocument();
    });

    // Type in field - error should clear
    fireEvent.change(nameInput, { target: { value: 'Test' } });

    await waitFor(() => {
      expect(screen.queryByText('Name is required')).not.toBeInTheDocument();
    });
  });
});
