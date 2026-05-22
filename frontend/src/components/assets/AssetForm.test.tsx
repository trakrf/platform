import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import { AssetForm } from './AssetForm';
import type { Asset } from '@/types/assets';
import { checkTagConflict } from '@/lib/tags/conflictCheck';

vi.mock('@/lib/tags/conflictCheck');

describe('AssetForm', () => {
  afterEach(() => {
    cleanup();
  });

  const mockOnSubmit = vi.fn();
  const mockOnCancel = vi.fn();

  const mockAsset: Asset = {
    id: 1,
    org_id: 1,
    external_key: 'LAP-001',
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

    expect(screen.getByLabelText(/Asset ID/)).toBeInTheDocument();
    expect(screen.getByLabelText(/Name/)).toBeInTheDocument();
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

    const identifierInput = screen.getByLabelText(/Asset ID/);
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

    fireEvent.change(screen.getByLabelText(/Asset ID/), { target: { value: 'TEST-001' } });
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Test Asset' } });

    const submitButton = screen.getByRole('button', { name: /Create Asset/ });
    fireEvent.click(submitButton);

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalled();
    });
  });

  // TRA-624 / BB20 §F1: empty valid_to must serialize as null, never the
  // 2099-12-31 sentinel. The docs forbid sentinel emission server-side, and
  // a docs-compliant null-checking client treats 2099-12-31 as "expires in
  // 2099" rather than "no expiry" — silently inverting meaning.
  it('submits valid_to as null when the user leaves the field empty', async () => {
    mockOnSubmit.mockResolvedValue(undefined);
    render(<AssetForm mode="create" onSubmit={mockOnSubmit} onCancel={mockOnCancel} />);

    fireEvent.change(screen.getByLabelText(/Asset ID/), { target: { value: 'TEST-001' } });
    fireEvent.change(screen.getByLabelText(/Name/), { target: { value: 'Test Asset' } });

    fireEvent.click(screen.getByRole('button', { name: /Create Asset/ }));

    await waitFor(() => {
      expect(mockOnSubmit).toHaveBeenCalled();
    });

    const submitted = mockOnSubmit.mock.calls[0]![0] as { valid_to: string | null };
    expect(submitted.valid_to).toBeNull();
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

describe('AssetForm - Tag conflict', () => {
  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('renders an inline conflict error and disables Save', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(
      'Tag already attached to location "Dock 3" — remove it there before attaching here.',
    );

    render(<AssetForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    // Click "Add Tag" to add a tag row (create mode already has one blank row)
    const addTagButton = screen.getByRole('button', { name: /Add Tag/i });
    fireEvent.click(addTagButton);

    // Get the first tag input (index 0, already present in create mode)
    const tagInputs = screen.getAllByPlaceholderText('Enter tag number...');
    const tagInput = tagInputs[0];

    // Type a value and blur to trigger the conflict check
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    // The conflict message should appear
    await screen.findByText(/already attached to location "Dock 3"/);

    // Save button should be disabled
    expect(screen.getByRole('button', { name: /create/i })).toBeDisabled();
  });

  it('clears the conflict when the tag is free', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<AssetForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    const tagInput = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    await waitFor(() => {
      expect(
        screen.queryByText(/already attached/),
      ).not.toBeInTheDocument();
    });

    // Save button should NOT be disabled (only by conflict — loading is false)
    expect(screen.getByRole('button', { name: /create asset/i })).not.toBeDisabled();
  });

  it('the "Reassign" modal is gone', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<AssetForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    const tagInput = screen.getByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInput, { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInput);

    await waitFor(() => {
      expect(screen.queryByText('Tag Already Assigned')).toBeNull();
    });
  });

  it('warns inline and disables Save when two rows have the same typed value', async () => {
    vi.mocked(checkTagConflict).mockResolvedValue(null);

    render(<AssetForm mode="create" onSubmit={vi.fn()} onCancel={vi.fn()} />);

    // Add a second tag row.
    fireEvent.click(screen.getByRole('button', { name: /Add Tag/i }));

    // Type the same value into both rows, blurring each to trigger the check.
    const tagInputs = screen.getAllByPlaceholderText('Enter tag number...');
    fireEvent.change(tagInputs[0], { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInputs[0]);
    fireEvent.change(tagInputs[1], { target: { value: 'AABBCCDD' } });
    fireEvent.blur(tagInputs[1]);

    // The same-form duplicate warning should appear.
    await screen.findByText(/already in this form's tag list/i);

    // Save button should be disabled.
    expect(screen.getByRole('button', { name: /create/i })).toBeDisabled();
  });
});
