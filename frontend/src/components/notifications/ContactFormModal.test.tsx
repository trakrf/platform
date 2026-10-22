import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import {
  render as rtlRender,
  screen,
  fireEvent,
  cleanup,
  waitFor,
  type RenderOptions,
} from '@testing-library/react';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import type { ReactElement, ReactNode } from 'react';
import toast from 'react-hot-toast';
import { ContactFormModal } from './ContactFormModal';
import { notificationsApi } from '@/lib/api/notifications';
import type { NotificationRecipient } from '@/types/notifications';

vi.mock('@/lib/api/notifications');
vi.mock('@/lib/auth/orgContext', () => ({
  ensureOrgContext: vi.fn().mockResolvedValue(undefined),
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

const wrapper = ({ children }: { children: ReactNode }) => {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>;
};
const render = (ui: ReactElement, options?: Omit<RenderOptions, 'wrapper'>) =>
  rtlRender(ui, { wrapper, ...options });

const contact: NotificationRecipient = {
  id: 7,
  org_id: 1,
  name: 'Jane Smith',
  email: 'jane@example.com',
  phone: '+15551234567',
  is_active: true,
  created_at: '2026-01-01T00:00:00Z',
  updated_at: '2026-01-01T00:00:00Z',
};

const type = (testId: string, value: string) =>
  fireEvent.change(screen.getByTestId(testId), { target: { value } });
const typePhone = (code: string, number: string) => {
  type('contact-phone-code', code);
  type('contact-phone-number', number);
};
const submit = () => fireEvent.click(screen.getByTestId('contact-submit'));

describe('ContactFormModal', () => {
  const onClose = vi.fn();

  beforeEach(() => {
    vi.clearAllMocks();
    vi.mocked(notificationsApi.createRecipient).mockResolvedValue({
      data: { data: contact },
    } as never);
    vi.mocked(notificationsApi.updateRecipient).mockResolvedValue({
      data: { data: contact },
    } as never);
  });

  afterEach(cleanup);

  it('renders nothing when closed', () => {
    const { container } = render(<ContactFormModal isOpen={false} mode="create" onClose={onClose} />);
    expect(container.firstChild).toBeNull();
  });

  it('creates a contact with email only', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    expect(screen.queryByTestId('contact-active')).not.toBeInTheDocument();
    type('contact-name', '  Jane Smith ');
    type('contact-email', ' jane@example.com ');
    submit();

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(notificationsApi.createRecipient).toHaveBeenCalledWith({
      name: 'Jane Smith',
      email: 'jane@example.com',
    });
    expect(toast.success).toHaveBeenCalledWith('Contact added');
  });

  it('creates a contact with phone only, normalized', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    typePhone('+1', '(555) 123-4567');
    submit();

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(notificationsApi.createRecipient).toHaveBeenCalledWith({
      name: 'Jane Smith',
      phone: '+15551234567',
    });
  });

  it('creates a contact with both email and phone', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    type('contact-email', 'jane@example.com');
    typePhone('+44', '20 7946 0958');
    submit();

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(notificationsApi.createRecipient).toHaveBeenCalledWith({
      name: 'Jane Smith',
      email: 'jane@example.com',
      phone: '+442079460958',
    });
  });

  it('blocks submit when email and phone are both empty', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    submit();

    expect(await screen.findByText('Enter an email or a phone number')).toBeInTheDocument();
    expect(notificationsApi.createRecipient).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('shows every error on submit, then updates them as the user types', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    submit();

    expect(await screen.findByText('Name is required')).toBeInTheDocument();
    expect(screen.getByText('Enter an email or a phone number')).toBeInTheDocument();

    type('contact-name', 'Jane Smith');
    expect(screen.queryByText('Name is required')).not.toBeInTheDocument();
    type('contact-email', 'jane@');
    expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();
    expect(notificationsApi.createRecipient).not.toHaveBeenCalled();
  });

  it('checks a field only once it is left', () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-email', 'jane@');
    expect(screen.queryByText('Enter a valid email address')).not.toBeInTheDocument();

    fireEvent.blur(screen.getByTestId('contact-email'));
    expect(screen.getByText('Enter a valid email address')).toBeInTheDocument();
  });

  it('checks the phone when focus leaves both parts, not between them', () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    const code = screen.getByTestId('contact-phone-code');
    const number = screen.getByTestId('contact-phone-number');

    type('contact-phone-code', '44');
    expect(code).toHaveValue('+44');
    fireEvent.blur(code, { relatedTarget: number });
    expect(screen.queryByText('Enter the phone number')).not.toBeInTheDocument();

    fireEvent.blur(number, { relatedTarget: null });
    expect(screen.getByText('Enter the phone number')).toBeInTheDocument();
    expect(number).toHaveAttribute('aria-invalid', 'true');
  });

  it('blocks submit on an invalid phone and puts the error on the wrong part', async () => {
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    typePhone('', '07911123456');
    submit();

    expect(await screen.findByText('Enter the country code, e.g. +1')).toBeInTheDocument();
    expect(screen.getByText('Leave out the leading 0')).toBeInTheDocument();
    expect(screen.getByTestId('contact-phone-code')).toHaveAttribute('aria-invalid', 'true');
    expect(screen.queryByText(/without the leading 0/)).not.toBeInTheDocument();
    expect(notificationsApi.createRecipient).not.toHaveBeenCalled();
  });

  it('edit splits the stored phone into its two parts', () => {
    render(<ContactFormModal isOpen mode="edit" contact={contact} onClose={onClose} />);
    expect(screen.getByTestId('contact-phone-code')).toHaveValue('+1');
    expect(screen.getByTestId('contact-phone-number')).toHaveValue('5551234567');
  });

  it('edit sends only the changed fields', async () => {
    render(<ContactFormModal isOpen mode="edit" contact={contact} onClose={onClose} />);
    expect(screen.getByTestId('contact-name')).toHaveValue('Jane Smith');
    type('contact-email', 'jane.smith@example.com');
    fireEvent.click(screen.getByTestId('contact-active'));
    submit();

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(notificationsApi.updateRecipient).toHaveBeenCalledWith(7, {
      email: 'jane.smith@example.com',
      is_active: false,
    });
    expect(toast.success).toHaveBeenCalledWith('Contact updated');
  });

  it('edit blocks clearing an existing email', async () => {
    render(<ContactFormModal isOpen mode="edit" contact={contact} onClose={onClose} />);
    type('contact-email', '');
    submit();

    expect(
      await screen.findByText("This can't be removed yet — replace it instead")
    ).toBeInTheDocument();
    expect(notificationsApi.updateRecipient).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
  });

  it('shows a 409 conflict inline under the email field', async () => {
    vi.mocked(notificationsApi.createRecipient).mockRejectedValue({
      response: { status: 409, data: { error: { detail: 'Resource already exists' } } },
    });
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    type('contact-email', 'jane@example.com');
    submit();

    expect(await screen.findByText('Another contact already uses this email')).toBeInTheDocument();
    expect(screen.getByTestId('contact-email')).toHaveAttribute('aria-invalid', 'true');
    expect(onClose).not.toHaveBeenCalled();
  });

  it('shows a 400 as a form-level error', async () => {
    vi.mocked(notificationsApi.createRecipient).mockRejectedValue({
      response: { status: 400, data: { error: { detail: 'invalid phone' } } },
    });
    render(<ContactFormModal isOpen mode="create" onClose={onClose} />);
    type('contact-name', 'Jane Smith');
    typePhone('+1', '5551234567');
    submit();

    expect(await screen.findByRole('alert')).toHaveTextContent('invalid phone');
    expect(onClose).not.toHaveBeenCalled();
  });
});
