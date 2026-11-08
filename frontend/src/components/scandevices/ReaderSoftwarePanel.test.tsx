import '@testing-library/jest-dom';
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, cleanup, fireEvent } from '@testing-library/react';
import { ReaderSoftwarePanel } from './ReaderSoftwarePanel';
import { useReaderSoftware } from '@/hooks/scandevices';
import type { ReaderSoftware } from '@/types/scandevices';

vi.mock('@/hooks/scandevices');

const startUpdate = vi.fn(() => Promise.resolve());

type State = ReturnType<typeof useReaderSoftware>;
function setup(over: Partial<State> & { software?: ReaderSoftware }) {
  const state: State = {
    software: undefined,
    isLoading: false,
    error: null,
    startUpdate,
    isStarting: false,
    startError: null,
    target: null,
    outcome: null,
    ...over,
  } as State;
  vi.mocked(useReaderSoftware).mockReturnValue(state);
  return render(<ReaderSoftwarePanel deviceId={10} />);
}

describe('ReaderSoftwarePanel', () => {
  beforeEach(() => startUpdate.mockClear());
  afterEach(() => cleanup());

  it('shows the running daemon version and the last update result', () => {
    setup({
      software: {
        daemon: {
          version: 'v1.5.0-292',
          update: {
            state: 'rolled_back',
            target: 'v1.5.0-300',
            error: 'crash_loop',
          },
        },
        supports_update: true,
      },
    });
    expect(screen.getByText('v1.5.0-292')).toBeInTheDocument();
    expect(screen.getByText(/rolled back from v1\.5\.0-300/i)).toBeInTheDocument();
    expect(screen.getByText(/crash_loop/)).toBeInTheDocument();
  });

  it('starts an update only after confirmation', () => {
    setup({ software: { daemon: { version: 'v1' }, supports_update: true } });
    fireEvent.change(screen.getByLabelText(/target version/i), {
      target: { value: ' v2 ' },
    });
    fireEvent.click(screen.getByRole('button', { name: /^update$/i }));
    expect(startUpdate).not.toHaveBeenCalled();
    fireEvent.click(screen.getByRole('button', { name: /confirm/i }));
    expect(startUpdate).toHaveBeenCalledWith('v2');
  });

  it('does not offer an update the reader cannot take', () => {
    setup({
      software: { daemon: { version: 'legacy' }, supports_update: false },
    });
    expect(screen.queryByRole('button', { name: /^update$/i })).not.toBeInTheDocument();
    expect(screen.getByText(/reinstall over ssh/i)).toBeInTheDocument();
  });

  it('shows progress while watching and the outcome when it lands', () => {
    const { unmount } = setup({
      software: { daemon: { version: 'v1' }, supports_update: true },
      target: 'v2',
      outcome: 'watching',
    });
    expect(screen.getByText(/updating to v2/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /^update$/i })).toBeDisabled();
    unmount();

    setup({
      software: {
        daemon: { version: 'v2', update: { state: 'committed', target: 'v2' } },
        supports_update: true,
      },
      target: 'v2',
      outcome: 'done',
    });
    expect(screen.getByText(/now running v2/i)).toBeInTheDocument();
  });

  it('explains a rollback to the operator', () => {
    setup({
      software: {
        daemon: {
          version: 'v1',
          update: { state: 'rolled_back', target: 'v2', error: 'no_commit' },
        },
        supports_update: true,
      },
      target: 'v2',
      outcome: 'rolled_back',
    });
    expect(screen.getByText(/rolled back to v1/i)).toBeInTheDocument();
  });

  it('shows why the reader refused an update', () => {
    setup({
      software: { daemon: { version: 'v1' }, supports_update: true },
      startError: {
        response: {
          status: 422,
          data: {
            error: 'update_refused',
            reason: 'an update is already in progress',
          },
        },
      },
    });
    expect(screen.getByText(/an update is already in progress/)).toBeInTheDocument();
  });

  it('reports an unreachable reader without crashing', () => {
    setup({ error: new Error('502') });
    expect(screen.getByText(/could not reach the reader/i)).toBeInTheDocument();
  });
});
