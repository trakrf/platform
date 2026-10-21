import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup } from '@testing-library/react';
import '@testing-library/jest-dom';
import { useUIStore } from '@/stores/uiStore';
import type { NotificationAlert } from '@/types/notifications';

const authState = { isAuthenticated: true };
const entitlement = { isLocked: false };
let alerts: NotificationAlert[] = [];

vi.mock('@/stores', async () => {
  const { useUIStore: realUIStore } = await vi.importActual<
    typeof import('@/stores/uiStore')
  >('@/stores/uiStore');
  return {
    useUIStore: realUIStore,
    useAuthStore: (selector?: (state: typeof authState) => unknown) =>
      selector ? selector(authState) : authState,
  };
});

vi.mock('@/hooks/entitlement/useEntitlement', () => ({
  useEntitlement: () => entitlement,
}));

vi.mock('@/hooks/notifications/useNotificationAlerts', () => ({
  useNotificationAlerts: () => ({
    alerts,
    total: alerts.length,
    page: 1,
    perPage: 5,
    isLoading: false,
    error: null,
  }),
}));

const { NotificationBell } = await import('./NotificationBell');

function makeAlert(id: number): NotificationAlert {
  const sent = new Date(Date.now() - id * 60 * 60 * 1000).toISOString();
  return {
    id,
    asset_id: id,
    asset_name: `Asset ${id}`,
    asset_external_key: `A-${id}`,
    type: id % 2 === 0 ? 'alarm' : 'location_change',
    occurred_at: sent,
    sent_at: sent,
    recipient_id: 1,
    recipient_name: 'Pat',
    channel: 'email',
    subject: null,
    body: 'Moved',
    details: {},
  };
}

const openPanel = () => fireEvent.click(screen.getByTestId('header-notifications-bell'));

describe('NotificationBell', () => {
  beforeEach(() => {
    authState.isAuthenticated = true;
    entitlement.isLocked = false;
    alerts = [];
    useUIStore.setState({ activeTab: 'scan' });
  });

  afterEach(() => {
    cleanup();
  });

  it('renders nothing when signed out', () => {
    authState.isAuthenticated = false;
    const { container } = render(<NotificationBell />);
    expect(container).toBeEmptyDOMElement();
  });

  it('shows no dot and an empty message when there are no alerts', () => {
    render(<NotificationBell />);
    expect(screen.queryByTestId('header-notifications-dot')).not.toBeInTheDocument();
    openPanel();
    expect(screen.getByText('No alerts yet')).toBeInTheDocument();
  });

  it('shows the dot and up to five recent alerts', () => {
    alerts = [1, 2, 3, 4, 5, 6].map(makeAlert);
    render(<NotificationBell />);
    expect(screen.getByTestId('header-notifications-dot')).toBeInTheDocument();

    const bell = screen.getByTestId('header-notifications-bell');
    expect(bell).toHaveAttribute('aria-expanded', 'false');
    openPanel();
    expect(bell).toHaveAttribute('aria-expanded', 'true');

    const panel = screen.getByTestId('header-notifications-panel');
    expect(panel).toBeInTheDocument();
    expect(screen.getAllByTestId(/^header-notifications-row-/)).toHaveLength(5);
    expect(screen.getByText('Asset 1')).toBeInTheDocument();
    expect(screen.queryByText('Asset 6')).not.toBeInTheDocument();
    expect(screen.getAllByText('Alarm').length).toBeGreaterThan(0);
    expect(screen.getAllByText('Location change').length).toBeGreaterThan(0);
    expect(screen.getByText('1 hour ago')).toBeInTheDocument();
  });

  it('"View all" opens the notifications page and closes the panel', () => {
    const pushState = vi.spyOn(window.history, 'pushState');
    alerts = [makeAlert(1)];
    render(<NotificationBell />);
    openPanel();

    fireEvent.click(screen.getByTestId('header-notifications-view-all'));

    expect(useUIStore.getState().activeTab).toBe('notifications');
    expect(pushState).toHaveBeenCalledWith({ tab: 'notifications' }, '', '#notifications');
    expect(screen.queryByTestId('header-notifications-panel')).not.toBeInTheDocument();
    pushState.mockRestore();
  });

  it('clicking a row opens the notifications page', () => {
    alerts = [makeAlert(1)];
    render(<NotificationBell />);
    openPanel();

    fireEvent.click(screen.getByTestId('header-notifications-row-1'));

    expect(useUIStore.getState().activeTab).toBe('notifications');
    expect(screen.queryByTestId('header-notifications-panel')).not.toBeInTheDocument();
  });

  it('closes on Escape', () => {
    render(<NotificationBell />);
    openPanel();
    expect(screen.getByTestId('header-notifications-panel')).toBeInTheDocument();

    fireEvent.keyDown(document, { key: 'Escape' });

    expect(screen.queryByTestId('header-notifications-panel')).not.toBeInTheDocument();
  });

  it('closes on an outside click', () => {
    const { container } = render(<NotificationBell />);
    openPanel();

    const overlay = container.querySelector('.fixed.inset-0');
    expect(overlay).not.toBeNull();
    fireEvent.click(overlay!);

    expect(screen.queryByTestId('header-notifications-panel')).not.toBeInTheDocument();
  });

  it('shows the not-enabled line and no dot for a locked org', () => {
    entitlement.isLocked = true;
    alerts = [makeAlert(1), makeAlert(2)];
    render(<NotificationBell />);
    expect(screen.queryByTestId('header-notifications-dot')).not.toBeInTheDocument();

    openPanel();

    expect(
      screen.getByText("Notifications aren't enabled for your organization.")
    ).toBeInTheDocument();
    expect(screen.queryByTestId(/^header-notifications-row-/)).not.toBeInTheDocument();
    expect(screen.getByTestId('header-notifications-view-all')).toBeInTheDocument();
  });
});
