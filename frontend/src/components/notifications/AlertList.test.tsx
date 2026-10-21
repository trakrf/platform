import { render, screen, within, fireEvent } from '@testing-library/react';
import { describe, it, expect } from 'vitest';
import { AlertList } from './AlertList';
import type { NotificationAlert } from '@/types/notifications';

const alarm: NotificationAlert = {
  id: 1,
  asset_id: 10,
  asset_name: 'Blue Wind RV',
  asset_external_key: 'RV-0042',
  type: 'alarm',
  occurred_at: '2026-10-20T16:12:00Z',
  sent_at: '2026-10-20T16:12:30Z',
  recipient_id: 5,
  recipient_name: 'Maria Nguyen',
  channel: 'email',
  subject: 'Alert: Blue Wind RV left the yard',
  body: "Your RV 'Blue Wind' left the yard through Gate A at 4:12 PM.",
  details: { location: 'Gate A', direction: 'unknown' },
};

const move: NotificationAlert = {
  ...alarm,
  id: 2,
  type: 'location_change',
  recipient_name: 'Sam Okafor',
  channel: 'sms',
  subject: null,
  body: "Your RV 'Blue Wind' moved from Row C to Wash Bay at 9:03 AM.",
  details: { from: 'Row C', to: 'Wash Bay' },
};

const table = () => screen.getByRole('table');

describe('AlertList', () => {
  it('renders a row and a card per alert', () => {
    render(<AlertList alerts={[alarm, move]} />);

    const row = screen.getByTestId('alert-row-1');
    expect(within(row).getByText('Blue Wind RV')).toBeInTheDocument();
    expect(within(row).getByText('RV-0042')).toBeInTheDocument();
    expect(within(row).getByText('Alarm')).toBeInTheDocument();
    expect(within(row).getByText('Maria Nguyen')).toBeInTheDocument();
    expect(within(row).getByText('Email')).toBeInTheDocument();

    const second = screen.getByTestId('alert-row-2');
    expect(within(second).getByText('Location change')).toBeInTheDocument();
    expect(within(second).getByText('SMS')).toBeInTheDocument();

    expect(screen.getByTestId('alert-card-1')).toBeInTheDocument();
    expect(screen.getByTestId('alert-card-2')).toBeInTheDocument();
  });

  it('hides the asset column when compact', () => {
    render(<AlertList alerts={[alarm]} compact />);

    expect(within(table()).queryByText('Asset')).not.toBeInTheDocument();
    expect(screen.queryByText('Blue Wind RV')).not.toBeInTheDocument();
    expect(within(table()).getByText('Sent')).toBeInTheDocument();
  });

  it('expands a row to show subject, body and details, and collapses it again', () => {
    render(<AlertList alerts={[alarm, move]} />);
    expect(screen.queryByTestId('alert-expanded-1')).not.toBeInTheDocument();

    fireEvent.click(screen.getByTestId('alert-row-1'));
    const expanded = screen.getByTestId('alert-expanded-1');
    expect(within(expanded).getByText(alarm.subject!)).toBeInTheDocument();
    expect(within(expanded).getByText(alarm.body)).toBeInTheDocument();
    expect(within(expanded).getByText('location')).toBeInTheDocument();
    expect(within(expanded).getByText('Gate A')).toBeInTheDocument();

    fireEvent.click(screen.getByTestId('alert-row-1'));
    expect(screen.queryByTestId('alert-expanded-1')).not.toBeInTheDocument();
  });

  it('toggles once when the expander button is clicked', () => {
    render(<AlertList alerts={[alarm]} />);
    fireEvent.click(screen.getByRole('button', { name: 'Show message for Blue Wind RV' }));
    expect(screen.getByTestId('alert-expanded-1')).toBeInTheDocument();
  });

  it('keeps only one alert open at a time', () => {
    render(<AlertList alerts={[alarm, move]} />);

    fireEvent.click(screen.getByTestId('alert-row-1'));
    fireEvent.click(screen.getByTestId('alert-row-2'));

    expect(screen.queryByTestId('alert-expanded-1')).not.toBeInTheDocument();
    const expanded = screen.getByTestId('alert-expanded-2');
    expect(within(expanded).getByText(move.body)).toBeInTheDocument();
  });

  it('shows and hides the message on a mobile card', () => {
    render(<AlertList alerts={[move]} />);
    const card = screen.getByTestId('alert-card-2');

    fireEvent.click(within(card).getByRole('button', { name: 'Show message' }));
    expect(within(card).getByText(move.body)).toBeInTheDocument();

    fireEvent.click(within(card).getByRole('button', { name: 'Hide message' }));
    expect(within(card).queryByText(move.body)).not.toBeInTheDocument();
  });
});
