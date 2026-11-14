import '@testing-library/jest-dom';
import type { ReactNode } from 'react';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import { AssetNotificationsModal } from './AssetNotificationsModal';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';
import type { Entitlement } from '@/hooks/entitlement/useEntitlement';
import type { NotificationAlert } from '@/types/notifications';

vi.mock('react-hot-toast', () => ({
  default: Object.assign(vi.fn(), { success: vi.fn(), error: vi.fn() }),
  Toaster: () => null,
}));

vi.mock('@/components/entitlement', () => ({
  PaidGate: ({ children }: { children: ReactNode }) => <>{children}</>,
}));

const useEntitlement = vi.fn<() => Entitlement>();
vi.mock('@/hooks/entitlement/useEntitlement', () => ({
  useEntitlement: () => useEntitlement(),
}));

vi.mock('@/hooks/notifications', () => ({
  useAssetSubscriptions: () => ({ subscriptions: [], isLoading: false, error: null }),
  useAssetSubscriptionMutations: () => ({
    subscribe: vi.fn(),
    update: vi.fn(),
    isUpdating: false,
  }),
  useNotificationRecipients: () => ({ recipients: [], isLoading: false, error: null }),
  useNotificationRecipientMutations: () => ({ create: vi.fn() }),
}));

vi.mock('@/hooks/notifications/useNotificationAlerts', () => ({
  useNotificationAlerts: vi.fn(),
}));

const mockAlerts = vi.mocked(useNotificationAlerts);

function entitlement(state: Entitlement['state']): Entitlement {
  const isEntitled = state === 'entitled';
  return { state, isEntitled, isLocked: !isEntitled, subscriptionExpiresAt: null };
}

const alert = (id: number): NotificationAlert => ({
  id,
  asset_id: 5,
  asset_name: 'Forklift 3',
  asset_external_key: 'FL-003',
  type: 'alarm',
  occurred_at: '2026-10-20T10:00:00Z',
  sent_at: '2026-10-20T10:00:05Z',
  recipient_id: 1,
  recipient_name: 'Ana',
  channel: 'email',
  subject: 'Alarm',
  body: 'Forklift 3 triggered an alarm',
  details: { location: 'Gate A' },
});

function returnAlerts(alerts: NotificationAlert[], total = alerts.length, isLoading = false) {
  mockAlerts.mockImplementation((filters = {}) => ({
    alerts,
    total,
    page: filters.page ?? 1,
    perPage: filters.per_page ?? 20,
    isLoading,
    error: null,
  }));
}

const lastCall = () => mockAlerts.mock.calls[mockAlerts.mock.calls.length - 1][0];

const onClose = vi.fn();

function renderModal(isOpen = true) {
  return render(
    <AssetNotificationsModal
      isOpen={isOpen}
      onClose={onClose}
      assetId={5}
      assetName="Forklift 3"
      assetKey="FL-003"
    />
  );
}

describe('AssetNotificationsModal', () => {
  beforeEach(() => {
    mockAlerts.mockReset();
    onClose.mockReset();
    useEntitlement.mockReturnValue(entitlement('entitled'));
    returnAlerts([]);
  });

  afterEach(cleanup);

  it('renders the title and the asset name and key', () => {
    renderModal();

    expect(screen.getByRole('dialog', { name: 'Notifications' })).toBeInTheDocument();
    expect(screen.getByText('Forklift 3 · FL-003')).toBeInTheDocument();
    expect(screen.getByText('Subscribers')).toBeInTheDocument();
    expect(screen.getByText('Alerts')).toBeInTheDocument();
  });

  it('renders nothing and reads no alerts while closed', () => {
    renderModal(false);

    expect(screen.queryByTestId('asset-notifications-modal')).not.toBeInTheDocument();
    expect(mockAlerts).not.toHaveBeenCalled();
  });

  it('shows the upsell instead of the sections when the org is locked', () => {
    useEntitlement.mockReturnValue(entitlement('lapsed'));
    renderModal();

    expect(screen.getByTestId('capability-upsell-notifications')).toBeInTheDocument();
    expect(screen.queryByText('Subscribers')).not.toBeInTheDocument();
    expect(screen.queryByText('Alerts')).not.toBeInTheDocument();
  });

  it("lists this asset's alerts without the asset column", () => {
    returnAlerts([alert(1), alert(2)]);
    renderModal();

    expect(lastCall()).toEqual({ asset_id: 5, page: 1, per_page: 10 });
    expect(screen.getByTestId('alert-row-1')).toBeInTheDocument();
    expect(screen.getByTestId('alert-row-2')).toBeInTheDocument();
    expect(screen.queryByRole('columnheader', { name: 'Asset' })).not.toBeInTheDocument();
    // One page only: no paging controls.
    expect(screen.queryByTestId('asset-alerts-page')).not.toBeInTheDocument();
  });

  it('shows loading text while alerts load', () => {
    returnAlerts([], 0, true);
    renderModal();

    expect(screen.getByText('Loading alerts…')).toBeInTheDocument();
  });

  it('shows the empty text when the asset has no alerts', () => {
    renderModal();

    expect(screen.getByText('No alerts for this asset yet.')).toBeInTheDocument();
    expect(screen.queryByTestId('alert-row-1')).not.toBeInTheDocument();
  });

  it('pages forward and back', () => {
    returnAlerts([alert(1)], 25);
    renderModal();

    expect(screen.getByTestId('asset-alerts-page')).toHaveTextContent('Page 1 of 3');
    expect(screen.getByTestId('asset-alerts-prev')).toBeDisabled();

    fireEvent.click(screen.getByTestId('asset-alerts-next'));
    expect(lastCall()).toEqual({ asset_id: 5, page: 2, per_page: 10 });
    expect(screen.getByTestId('asset-alerts-page')).toHaveTextContent('Page 2 of 3');

    fireEvent.click(screen.getByTestId('asset-alerts-next'));
    expect(screen.getByTestId('asset-alerts-page')).toHaveTextContent('Page 3 of 3');
    expect(screen.getByTestId('asset-alerts-next')).toBeDisabled();

    fireEvent.click(screen.getByTestId('asset-alerts-prev'));
    expect(lastCall()).toEqual({ asset_id: 5, page: 2, per_page: 10 });
    expect(screen.getByTestId('asset-alerts-page')).toHaveTextContent('Page 2 of 3');
  });

  it('closes on Escape', () => {
    renderModal();

    fireEvent.keyDown(document, { key: 'Escape' });
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it('closes from the header X and the footer Close button', () => {
    renderModal();

    fireEvent.click(screen.getByRole('button', { name: 'Close modal' }));
    fireEvent.click(screen.getByRole('button', { name: 'Close' }));
    expect(onClose).toHaveBeenCalledTimes(2);
  });
});
