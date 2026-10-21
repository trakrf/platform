import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor, within } from '@testing-library/react';
import toast from 'react-hot-toast';
import ContactsScreen from './ContactsScreen';
import type { NotificationRecipient } from '@/types/notifications';

const deleteRecipient = vi.fn();
const recipientsState = vi.fn();
const countsState = vi.fn();

vi.mock('@/hooks/notifications', () => ({
  useNotificationRecipients: () => recipientsState(),
  useNotificationRecipientMutations: () => ({
    create: vi.fn(),
    update: vi.fn(),
    delete: deleteRecipient,
  }),
}));
vi.mock('@/hooks/notifications/useRecipientSubscriptionCounts', () => ({
  useRecipientSubscriptionCounts: () => countsState(),
}));
vi.mock('@/hooks/entitlement/useEntitlement', () => ({
  useEntitlement: () => ({
    state: 'entitled',
    isEntitled: true,
    isLocked: false,
    subscriptionExpiresAt: null,
  }),
}));
vi.mock('react-hot-toast', () => ({ default: { success: vi.fn(), error: vi.fn() } }));

function recipient(id: number, name: string, extra: Partial<NotificationRecipient> = {}) {
  return {
    id,
    org_id: 1,
    name,
    is_active: true,
    created_at: '2026-01-01T00:00:00Z',
    updated_at: '2026-01-01T00:00:00Z',
    ...extra,
  } satisfies NotificationRecipient;
}

const jane = recipient(1, 'Jane Smith', { email: 'jane@example.com' });
const omar = recipient(2, 'Omar Diaz', { phone: '+15551234567' });

const table = () => screen.getByTestId('contacts-table');
const row = (id: number) => screen.getByTestId(`contact-row-${id}`);

describe('ContactsScreen', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    recipientsState.mockReturnValue({ recipients: [jane, omar], isLoading: false, error: null });
    countsState.mockReturnValue({ counts: new Map([[1, 2]]), isLoading: false });
    deleteRecipient.mockResolvedValue(1);
  });

  afterEach(cleanup);

  it('renders the contact list', () => {
    render(<ContactsScreen />);
    expect(within(table()).getByText('Jane Smith')).toBeInTheDocument();
    expect(within(table()).getByText('jane@example.com')).toBeInTheDocument();
    expect(within(table()).getByText('+15551234567')).toBeInTheDocument();
  });

  it('filters by the search term', () => {
    render(<ContactsScreen />);
    fireEvent.change(screen.getByTestId('contacts-search'), { target: { value: '5551' } });
    expect(within(table()).queryByText('Jane Smith')).not.toBeInTheDocument();
    expect(within(table()).getByText('Omar Diaz')).toBeInTheDocument();
  });

  it('shows the subscribed-asset count per contact', () => {
    render(<ContactsScreen />);
    expect(screen.getByTestId('contact-count-1')).toHaveTextContent('2');
    expect(screen.getByTestId('contact-count-2')).toHaveTextContent('0');
    expect(screen.getByText('Subscribed to 2 assets')).toBeInTheDocument();
  });

  it('shows a placeholder while counts load', () => {
    countsState.mockReturnValue({ counts: new Map(), isLoading: true });
    render(<ContactsScreen />);
    expect(screen.getByTestId('contact-count-1')).toHaveTextContent('…');
  });

  it('deletes a contact after confirming, with the count in the message', async () => {
    render(<ContactsScreen />);
    fireEvent.click(within(row(1)).getByRole('button', { name: 'Delete Jane Smith' }));

    expect(screen.getByText('Delete contact')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Delete Jane Smith? They are subscribed to 2 assets and will stop receiving all alerts. Their alert history is kept.'
      )
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('modal-confirm-button'));
    await waitFor(() => expect(deleteRecipient).toHaveBeenCalledWith(1));
    expect(toast.success).toHaveBeenCalledWith('Contact deleted');
    await waitFor(() => expect(screen.queryByText('Delete contact')).not.toBeInTheDocument());
  });

  it('says an unsubscribed contact has no assets', () => {
    render(<ContactsScreen />);
    fireEvent.click(within(row(2)).getByRole('button', { name: 'Delete Omar Diaz' }));
    expect(
      screen.getByText(
        'Delete Omar Diaz? They are not subscribed to any assets. Their alert history is kept.'
      )
    ).toBeInTheDocument();
  });

  it('toasts the API error when delete fails', async () => {
    deleteRecipient.mockRejectedValue({ response: { data: { error: { detail: 'Nope' } } } });
    render(<ContactsScreen />);
    fireEvent.click(within(row(1)).getByRole('button', { name: 'Delete Jane Smith' }));
    fireEvent.click(screen.getByTestId('modal-confirm-button'));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('Nope'));
  });

  it('opens the create form from New contact', () => {
    render(<ContactsScreen />);
    expect(screen.queryByTestId('contact-form')).not.toBeInTheDocument();
    fireEvent.click(screen.getByRole('button', { name: /New contact/ }));
    expect(screen.getByTestId('contact-form')).toBeInTheDocument();
    expect(screen.getByTestId('contact-name')).toHaveValue('');
  });

  it('opens the edit form prefilled from a row', () => {
    render(<ContactsScreen />);
    fireEvent.click(within(row(1)).getByRole('button', { name: 'Edit Jane Smith' }));
    expect(screen.getByTestId('contact-name')).toHaveValue('Jane Smith');
    expect(screen.getByTestId('contact-active')).toBeChecked();
  });

  it('offers Add contact from the empty state', () => {
    recipientsState.mockReturnValue({ recipients: [], isLoading: false, error: null });
    render(<ContactsScreen />);
    fireEvent.click(screen.getByRole('button', { name: 'Add contact' }));
    expect(screen.getByTestId('contact-form')).toBeInTheDocument();
  });
});
