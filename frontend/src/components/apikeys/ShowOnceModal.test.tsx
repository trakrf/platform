import '@testing-library/jest-dom';
import { describe, it, expect, vi, afterEach } from 'vitest';
import { render, screen, fireEvent, cleanup, act, waitFor } from '@testing-library/react';
import { ShowOnceModal } from './ShowOnceModal';

afterEach(cleanup);

// Values mirror the real POST /orgs/{id}/api-keys response shape
// ({client_id, client_secret}) so this test guards against the contract drift
// in TRA-1019 — the old test passed a synthetic single `apiKey` string and
// never exercised the live shape.
const CLIENT_ID = '098b572b-1234-4abc-9def-0123456789ab';
const CLIENT_SECRET = 'trakrf_7953cd9e0f1a2b3c4d5e6f7a8b9c0d1e';

function mockClipboard() {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.assign(navigator, { clipboard: { writeText } });
  return writeText;
}

describe('ShowOnceModal', () => {
  it('renders both the client id and the client secret', () => {
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    expect(screen.getByText(CLIENT_ID)).toBeInTheDocument();
    expect(screen.getByText(CLIENT_SECRET)).toBeInTheDocument();
  });

  it('shows the warning banner about one-time display', () => {
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    expect(screen.getByText(/only time you'?ll see/i)).toBeInTheDocument();
  });

  it('copies the client secret to the clipboard', async () => {
    const writeText = mockClipboard();
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /copy client secret/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(CLIENT_SECRET));
  });

  it('copies the client id to the clipboard', async () => {
    const writeText = mockClipboard();
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /copy client id/i }));
    await waitFor(() => expect(writeText).toHaveBeenCalledWith(CLIENT_ID));
  });

  it('disables the save button until the secret is copied', async () => {
    mockClipboard();
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
    fireEvent.click(screen.getByRole('button', { name: /copy client secret/i }));
    await waitFor(() =>
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeEnabled(),
    );
  });

  it('does not enable the save button when only the client id is copied', async () => {
    mockClipboard();
    render(
      <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
    );
    fireEvent.click(screen.getByRole('button', { name: /copy client id/i }));
    // wait for the client-id field's own "Copied" state to settle, then confirm
    // the secret gate is still closed.
    await screen.findByText('Copied');
    expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
  });

  it('enables the save button after the 3-second dwell, even without a copy', () => {
    vi.useFakeTimers();
    try {
      render(
        <ShowOnceModal clientId={CLIENT_ID} clientSecret={CLIENT_SECRET} onClose={() => {}} />,
      );
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeDisabled();
      act(() => {
        vi.advanceTimersByTime(3000);
      });
      expect(screen.getByRole('button', { name: /i'?ve saved it/i })).toBeEnabled();
    } finally {
      vi.useRealTimers();
    }
  });
});
