import type { ReactNode } from 'react';
import { render, screen, fireEvent, within } from '@testing-library/react';
import { describe, it, expect, vi, beforeEach } from 'vitest';
import NotificationsScreen from './NotificationsScreen';
import { useNotificationAlerts } from '@/hooks/notifications/useNotificationAlerts';
import type { NotificationAlert } from '@/types/notifications';

vi.mock('@/hooks/notifications/useNotificationAlerts', () => ({
  useNotificationAlerts: vi.fn(),
}));
vi.mock('@/hooks/notifications', () => ({
  useNotificationRecipients: () => ({ recipients: [{ id: 5, name: 'Maria Nguyen' }] }),
}));
vi.mock('@/hooks/assets', () => ({
  useAssets: () => ({ assets: [{ id: 10, name: 'Blue Wind RV' }] }),
}));
vi.mock('@/stores', () => ({
  useUIStore: () => ({ setActiveTab: vi.fn() }),
}));
vi.mock('@/components/notifications/NotificationsGate', () => ({
  NotificationsGate: ({ children }: { children: ReactNode }) => <>{children}</>,
}));
vi.mock('@/components/notifications/AlertList', () => ({
  AlertList: ({ alerts }: { alerts: NotificationAlert[] }) => (
    <ul data-testid="alert-list">
      {alerts.map((a) => (
        <li key={a.id}>{a.asset_name}</li>
      ))}
    </ul>
  ),
}));
vi.mock('@/components/shared/pagination', () => ({
  PaginationControls: ({ currentPage, onNext }: { currentPage: number; onNext: () => void }) => (
    <div>
      <span data-testid="current-page">{currentPage}</span>
      <button type="button" onClick={onNext}>
        Next page
      </button>
    </div>
  ),
}));

const mockAlerts = vi.mocked(useNotificationAlerts);

const alert = { id: 1, asset_name: 'Blue Wind RV' } as NotificationAlert;

function returnAlerts(alerts: NotificationAlert[], total = alerts.length) {
  mockAlerts.mockImplementation((filters = {}) => ({
    alerts,
    total,
    page: filters.page ?? 1,
    perPage: filters.per_page ?? 20,
    isLoading: false,
    error: null,
  }));
}

const lastCall = () => mockAlerts.mock.calls[mockAlerts.mock.calls.length - 1][0];

describe('NotificationsScreen', () => {
  beforeEach(() => {
    mockAlerts.mockReset();
  });

  it('renders the alert list with filters above it', () => {
    returnAlerts([alert], 45);
    render(<NotificationsScreen />);

    expect(screen.getByTestId('alert-filter-asset')).toBeInTheDocument();
    expect(screen.getByTestId('alert-list')).toHaveTextContent('Blue Wind RV');
    expect(lastCall()).toEqual({ page: 1, per_page: 20 });
  });

  it('shows the generic empty state when there are no alerts and no filters', () => {
    returnAlerts([]);
    render(<NotificationsScreen />);

    expect(screen.getByText('No alerts yet')).toBeInTheDocument();
    expect(screen.queryByTestId('alerts-no-match')).not.toBeInTheDocument();
  });

  it('shows a no-match state with a clear action when filters exclude everything', () => {
    returnAlerts([alert]);
    render(<NotificationsScreen />);

    returnAlerts([]);
    fireEvent.change(screen.getByTestId('alert-filter-channel'), { target: { value: 'sms' } });

    expect(screen.getByTestId('alerts-no-match')).toHaveTextContent(
      'No alerts match these filters.'
    );
    expect(screen.queryByText('No alerts yet')).not.toBeInTheDocument();
    expect(lastCall()).toMatchObject({ channel: 'sms' });

    returnAlerts([alert]);
    fireEvent.click(
      within(screen.getByTestId('alerts-no-match')).getByRole('button', { name: 'Clear filters' })
    );
    expect(lastCall()).toEqual({ page: 1, per_page: 20 });
    expect(screen.getByTestId('alert-list')).toBeInTheDocument();
  });

  it('resets to page 1 when a filter changes', () => {
    returnAlerts([alert], 45);
    render(<NotificationsScreen />);

    fireEvent.click(screen.getByRole('button', { name: 'Next page' }));
    expect(screen.getByTestId('current-page')).toHaveTextContent('2');
    expect(lastCall()).toMatchObject({ page: 2 });

    fireEvent.change(screen.getByTestId('alert-filter-asset'), { target: { value: '10' } });
    expect(screen.getByTestId('current-page')).toHaveTextContent('1');
    expect(lastCall()).toEqual({ asset_id: 10, page: 1, per_page: 20 });
  });
});
