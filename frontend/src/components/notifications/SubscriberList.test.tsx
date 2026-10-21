import '@testing-library/jest-dom';
import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor, within } from '@testing-library/react';
import toast from 'react-hot-toast';
import { SubscriberList } from './SubscriberList';
import type { NotificationRecipient, NotificationSubscription } from '@/types/notifications';

vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }),
  Toaster: () => null,
}));

vi.mock('@/components/entitlement', () => ({
  PaidGate: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const update = vi.fn();
const hookState = {
  subscriptions: [] as NotificationSubscription[],
  recipients: [] as NotificationRecipient[],
};
vi.mock('@/hooks/notifications', () => ({
  useAssetSubscriptions: () => ({
    subscriptions: hookState.subscriptions,
    isLoading: false,
    error: null,
  }),
  useNotificationRecipients: () => ({
    recipients: hookState.recipients,
    isLoading: false,
    error: null,
  }),
  useAssetSubscriptionMutations: () => ({ update, isUpdating: false }),
}));

const recipient = (
  id: number,
  name: string,
  details: Partial<NotificationRecipient> = {}
): NotificationRecipient => ({
  id,
  org_id: 7,
  name,
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...details,
});

const subscription = (
  id: number,
  recipientId: number,
  overrides: Partial<NotificationSubscription> = {}
): NotificationSubscription => ({
  id,
  org_id: 7,
  asset_id: 5,
  recipient_id: recipientId,
  channel: 'email',
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
  ...overrides,
});

const renderList = () => render(<SubscriberList assetId={5} assetName="Forklift" />);

describe('SubscriberList', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    update.mockResolvedValue({});
    hookState.recipients = [
      recipient(1, 'Zoe', { email: 'zoe@example.com', phone: '+15550001' }),
      recipient(2, 'Adam', { email: 'adam@example.com' }),
      recipient(3, 'Mia', { phone: '+15550003' }),
    ];
    hookState.subscriptions = [];
  });

  afterEach(() => {
    cleanup();
  });

  it('shows the empty state', () => {
    renderList();
    expect(screen.getByText('No one is subscribed to this asset yet.')).toBeInTheDocument();
  });

  it('lists active rows first, then by name, and mutes switched-off rows', () => {
    hookState.subscriptions = [
      subscription(10, 1, { is_active: false }),
      subscription(11, 1),
      subscription(12, 2),
      subscription(13, 3, { channel: 'sms' }),
    ];
    renderList();

    const ids = screen
      .getAllByTestId(/^subscriber-row-/)
      .map((row) => row.getAttribute('data-testid'));
    expect(ids).toEqual([
      'subscriber-row-12',
      'subscriber-row-13',
      'subscriber-row-11',
      'subscriber-row-10',
    ]);

    const off = screen.getByTestId('subscriber-row-10');
    expect(off.className).toContain('opacity-60');
    expect(within(off).getByText('Off')).toBeInTheDocument();
    expect(within(off).getByTestId('subscriber-on-10')).toBeInTheDocument();
    expect(within(off).queryByTestId('subscriber-channel-10')).not.toBeInTheDocument();

    const active = screen.getByTestId('subscriber-row-13');
    expect(active.className).not.toContain('opacity-60');
    expect(within(active).getByText(/\+15550003/)).toBeInTheDocument();
    expect(within(active).getByTestId('subscriber-off-13')).toBeInTheDocument();
  });

  it('labels a deleted contact and offers no actions', () => {
    hookState.subscriptions = [subscription(20, 99)];
    renderList();

    const row = screen.getByTestId('subscriber-row-20');
    expect(within(row).getByText('Deleted contact')).toHaveClass('italic');
    expect(within(row).queryByRole('button')).not.toBeInTheDocument();
    expect(within(row).queryByRole('combobox')).not.toBeInTheDocument();
  });

  it('disables a channel the contact has no detail for', () => {
    hookState.subscriptions = [subscription(12, 2)];
    renderList();

    const select = screen.getByTestId('subscriber-channel-12');
    expect(within(select).getByRole('option', { name: 'Email' })).not.toBeDisabled();
    expect(within(select).getByRole('option', { name: 'SMS' })).toBeDisabled();
  });

  it('changes the channel', async () => {
    hookState.subscriptions = [subscription(11, 1)];
    renderList();

    fireEvent.change(screen.getByTestId('subscriber-channel-11'), { target: { value: 'sms' } });

    await waitFor(() => expect(update).toHaveBeenCalledWith({ id: 11, channel: 'sms' }));
  });

  it('explains a duplicate-channel conflict', async () => {
    hookState.subscriptions = [subscription(11, 1)];
    update.mockRejectedValue({ response: { status: 409 } });
    renderList();

    fireEvent.change(screen.getByTestId('subscriber-channel-11'), { target: { value: 'sms' } });

    await waitFor(() =>
      expect(toast.error).toHaveBeenCalledWith(
        'This contact is already subscribed by SMS on this asset'
      )
    );
  });

  it('shows the API error for other failures', async () => {
    hookState.subscriptions = [subscription(11, 1)];
    update.mockRejectedValue({
      response: { status: 400, data: { error: { detail: 'phone required' } } },
    });
    renderList();

    fireEvent.change(screen.getByTestId('subscriber-channel-11'), { target: { value: 'sms' } });

    await waitFor(() => expect(toast.error).toHaveBeenCalledWith('phone required'));
  });

  it('switches off only after confirmation', async () => {
    hookState.subscriptions = [subscription(11, 1)];
    renderList();

    fireEvent.click(screen.getByTestId('subscriber-off-11'));
    expect(update).not.toHaveBeenCalled();
    expect(screen.getByText('Switch off alerts')).toBeInTheDocument();
    expect(
      screen.getByText(
        'Zoe will stop receiving Email alerts for Forklift. You can turn this back on later.'
      )
    ).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('modal-confirm-button'));

    await waitFor(() => expect(update).toHaveBeenCalledWith({ id: 11, is_active: false }));
    expect(toast.success).toHaveBeenCalledWith('Alerts switched off');
    expect(screen.queryByText('Switch off alerts')).not.toBeInTheDocument();
  });

  it('does not switch off when the confirmation is cancelled', () => {
    hookState.subscriptions = [subscription(11, 1)];
    renderList();

    fireEvent.click(screen.getByTestId('subscriber-off-11'));
    fireEvent.click(screen.getByTestId('modal-cancel-button'));

    expect(update).not.toHaveBeenCalled();
    expect(screen.queryByText('Switch off alerts')).not.toBeInTheDocument();
  });

  it('closes only the confirmation on Escape', () => {
    hookState.subscriptions = [subscription(11, 1)];
    renderList();
    const outer = vi.fn();
    document.addEventListener('keydown', outer);

    fireEvent.click(screen.getByTestId('subscriber-off-11'));
    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByText('Switch off alerts')).not.toBeInTheDocument();
    expect(outer).not.toHaveBeenCalled();
    expect(update).not.toHaveBeenCalled();
    document.removeEventListener('keydown', outer);
  });

  it('turns a switched-off subscription back on', async () => {
    hookState.subscriptions = [subscription(10, 1, { is_active: false })];
    renderList();

    fireEvent.click(screen.getByTestId('subscriber-on-10'));

    await waitFor(() => expect(update).toHaveBeenCalledWith({ id: 10, is_active: true }));
    expect(toast.success).toHaveBeenCalledWith('Alerts turned back on');
  });
});
