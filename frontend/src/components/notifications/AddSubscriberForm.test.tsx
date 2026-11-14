import '@testing-library/jest-dom';
import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, waitFor } from '@testing-library/react';
import toast from 'react-hot-toast';
import { AddSubscriberForm } from './AddSubscriberForm';
import type {
  NotificationChannel,
  NotificationRecipient,
  NotificationSubscription,
} from '@/types/notifications';

vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }),
  Toaster: () => null,
}));

vi.mock('@/components/entitlement', () => ({
  PaidGate: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const create = vi.fn();
const subscribe = vi.fn();
const hookState = { recipients: [] as NotificationRecipient[] };
vi.mock('@/hooks/notifications', () => ({
  useNotificationRecipients: () => ({
    recipients: hookState.recipients,
    isLoading: false,
    error: null,
  }),
  useNotificationRecipientMutations: () => ({ create }),
  useAssetSubscriptionMutations: () => ({ subscribe }),
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

const sub = (id: number, recipientId: number, channel: NotificationChannel): NotificationSubscription => ({
  id,
  org_id: 7,
  asset_id: 5,
  recipient_id: recipientId,
  channel,
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
});

const apiError = (status: number, detail: string) => ({
  response: { status, data: { error: { detail } } },
});

const onDone = vi.fn();
const onCancel = vi.fn();

function renderForm() {
  return render(
    <AddSubscriberForm assetId={5} assetName="Forklift 3" onDone={onDone} onCancel={onCancel} />
  );
}

const checkbox = (channel: NotificationChannel) =>
  screen.getByTestId(`add-subscriber-channel-${channel}`) as HTMLInputElement;

describe('AddSubscriberForm', () => {
  beforeEach(() => {
    hookState.recipients = [
      recipient(1, 'Ana', { email: 'ana@example.com' }),
      recipient(2, 'Ben', { email: 'ben@example.com', phone: '+15551234567' }),
      recipient(3, 'Off Olga', { email: 'olga@example.com', is_active: false }),
    ];
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it('subscribes an existing contact by email', async () => {
    subscribe.mockResolvedValue({ succeeded: [sub(10, 1, 'email')], failed: [] });
    renderForm();

    expect(screen.getByTestId('add-subscriber-mode-existing')).toHaveAttribute('aria-pressed', 'true');
    expect(screen.queryByRole('option', { name: /Off Olga/ })).not.toBeInTheDocument();

    fireEvent.change(screen.getByTestId('add-subscriber-contact-search'), { target: { value: 'ana' } });
    fireEvent.click(screen.getByTestId('add-subscriber-contact-1'));
    expect(checkbox('email')).toBeChecked();
    fireEvent.click(screen.getByTestId('add-subscriber-submit'));

    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(subscribe).toHaveBeenCalledWith({ recipientId: 1, channels: ['email'] });
    expect(toast.success).toHaveBeenCalledWith('Ana will get Email alerts for Forklift 3');
  });

  it('disables a channel the contact has no detail for', () => {
    renderForm();
    fireEvent.click(screen.getByTestId('add-subscriber-contact-1'));

    expect(checkbox('sms')).toBeDisabled();
    expect(checkbox('sms')).not.toBeChecked();
    expect(screen.getByText('No phone on file')).toBeInTheDocument();
    expect(checkbox('email')).toBeEnabled();
  });

  it('asks for the missing detail when adding a new contact', () => {
    renderForm();
    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));

    expect(screen.getByText('Enter an email to use this')).toBeInTheDocument();
    expect(screen.getByText('Enter a phone number to use this')).toBeInTheDocument();
  });

  it('creates a new contact then subscribes by both channels with a normalized phone', async () => {
    create.mockResolvedValue(recipient(9, 'Cleo', { email: 'cleo@example.com', phone: '+15551234567' }));
    subscribe.mockResolvedValue({
      succeeded: [sub(11, 9, 'email'), sub(12, 9, 'sms')],
      failed: [],
    });
    renderForm();

    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));
    fireEvent.change(screen.getByTestId('add-subscriber-name'), { target: { value: ' Cleo ' } });
    fireEvent.change(screen.getByTestId('add-subscriber-email'), { target: { value: 'cleo@example.com' } });
    fireEvent.change(screen.getByTestId('add-subscriber-phone-code'), { target: { value: '+1' } });
    fireEvent.change(screen.getByTestId('add-subscriber-phone-number'), { target: { value: '(555) 123-4567' } });
    expect(checkbox('email')).toBeChecked();
    expect(checkbox('sms')).toBeChecked();
    fireEvent.click(screen.getByTestId('add-subscriber-submit'));

    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(create).toHaveBeenCalledWith({
      name: 'Cleo',
      email: 'cleo@example.com',
      phone: '+15551234567',
    });
    expect(subscribe).toHaveBeenCalledWith({ recipientId: 9, channels: ['email', 'sms'] });
    expect(toast.success).toHaveBeenCalledWith('Cleo will get Email and SMS alerts for Forklift 3');
  });

  it('shows a 409 from contact create inline and does not subscribe', async () => {
    create.mockRejectedValue(apiError(409, 'email already in use'));
    renderForm();

    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));
    fireEvent.change(screen.getByTestId('add-subscriber-name'), { target: { value: 'Dup' } });
    fireEvent.change(screen.getByTestId('add-subscriber-email'), { target: { value: 'ana@example.com' } });
    fireEvent.click(screen.getByTestId('add-subscriber-submit'));

    expect(await screen.findByText(/email already in use\./)).toBeInTheDocument();
    expect(screen.getByText(/pick them under "Existing contact"/)).toBeInTheDocument();
    expect(subscribe).not.toHaveBeenCalled();
    expect(onDone).not.toHaveBeenCalled();
  });

  it('keeps the contact on a partial failure and retries only the failed channel', async () => {
    create.mockResolvedValue(recipient(9, 'Cleo', { email: 'cleo@example.com', phone: '+15551234567' }));
    subscribe.mockResolvedValueOnce({
      succeeded: [sub(11, 9, 'email')],
      failed: [{ channel: 'sms', error: apiError(500, 'SMS provider unavailable') }],
    });
    renderForm();

    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));
    fireEvent.change(screen.getByTestId('add-subscriber-name'), { target: { value: 'Cleo' } });
    fireEvent.change(screen.getByTestId('add-subscriber-email'), { target: { value: 'cleo@example.com' } });
    fireEvent.change(screen.getByTestId('add-subscriber-phone-code'), { target: { value: '+1' } });
    fireEvent.change(screen.getByTestId('add-subscriber-phone-number'), { target: { value: '5551234567' } });
    fireEvent.click(screen.getByTestId('add-subscriber-submit'));

    expect(await screen.findByText('SMS failed: SMS provider unavailable')).toBeInTheDocument();
    expect(screen.getByText('Cleo is subscribed by Email.')).toBeInTheDocument();
    expect(screen.queryByTestId('add-subscriber-submit')).not.toBeInTheDocument();
    expect(screen.getByTestId('add-subscriber-name')).toBeDisabled();
    expect(onDone).not.toHaveBeenCalled();

    subscribe.mockResolvedValueOnce({ succeeded: [sub(12, 9, 'sms')], failed: [] });
    fireEvent.click(screen.getByTestId('add-subscriber-retry'));

    await waitFor(() => expect(onDone).toHaveBeenCalled());
    expect(create).toHaveBeenCalledTimes(1);
    expect(subscribe).toHaveBeenLastCalledWith({ recipientId: 9, channels: ['sms'] });
    expect(toast.success).toHaveBeenCalledWith('Cleo will get Email and SMS alerts for Forklift 3');
  });

  it('defaults to a new contact when the org has none', () => {
    hookState.recipients = [];
    renderForm();

    expect(screen.getByTestId('add-subscriber-mode-new')).toHaveAttribute('aria-pressed', 'true');
    expect(screen.getByText(/has no contacts yet/)).toBeInTheDocument();
    expect(screen.getByTestId('add-subscriber-name')).toBeInTheDocument();
  });

  it('starts with no contact picked and the channels and Subscribe disabled', () => {
    renderForm();

    expect(screen.getByRole('radiogroup', { name: 'Choose a contact' })).toBeInTheDocument();
    expect(screen.getAllByRole('radio').every((r) => !(r as HTMLInputElement).checked)).toBe(true);
    expect(checkbox('email')).toBeDisabled();
    expect(checkbox('sms')).toBeDisabled();
    expect(screen.getByText('Choose a contact first')).toBeInTheDocument();
    expect(screen.queryByText('No email on file')).not.toBeInTheDocument();
    expect(screen.getByTestId('add-subscriber-submit')).toBeDisabled();

    fireEvent.click(screen.getByTestId('add-subscriber-contact-1'));
    expect(screen.queryByText('Choose a contact first')).not.toBeInTheDocument();
    expect(screen.getByTestId('add-subscriber-submit')).toBeEnabled();
  });

  it('disables Subscribe when every channel is unticked', () => {
    renderForm();
    fireEvent.click(screen.getByTestId('add-subscriber-contact-1'));
    fireEvent.click(checkbox('email'));

    expect(screen.getByTestId('add-subscriber-submit')).toBeDisabled();
  });

  it('checks new-contact fields once they are left', () => {
    renderForm();
    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));
    fireEvent.blur(screen.getByTestId('add-subscriber-name'));
    expect(screen.getByText('Name is required')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('add-subscriber-email'), { target: { value: 'cleo@' } });
    fireEvent.blur(screen.getByTestId('add-subscriber-email'));
    expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();

    fireEvent.change(screen.getByTestId('add-subscriber-phone-number'), { target: { value: '07911123456' } });
    fireEvent.blur(screen.getByTestId('add-subscriber-phone-number'), { relatedTarget: null });
    expect(screen.getByText('Enter the country code, e.g. +1')).toBeInTheDocument();
    expect(screen.getByText('Leave out the leading 0')).toBeInTheDocument();
  });

  it('blocks a new contact with invalid fields on submit', () => {
    renderForm();
    fireEvent.click(screen.getByTestId('add-subscriber-mode-new'));
    fireEvent.change(screen.getByTestId('add-subscriber-email'), { target: { value: 'cleo@' } });
    fireEvent.click(screen.getByTestId('add-subscriber-submit'));

    expect(screen.getByText('Name is required')).toBeInTheDocument();
    expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();
    expect(create).not.toHaveBeenCalled();
  });
});
