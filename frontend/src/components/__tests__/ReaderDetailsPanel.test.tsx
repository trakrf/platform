import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { ReaderDetailsPanel } from '../ReaderDetailsPanel';

afterEach(cleanup);

describe('ReaderDetailsPanel', () => {
  it('shows each value the reader gave up', () => {
    render(<ReaderDetailsPanel details={{
      siliconLabsFirmware: '1.0.17',
      bluetoothFirmware: '1.0.20',
      rfidFirmware: '2.6.46',
      serialNumber: 'CS108ABC12345',
      macError: 0,
    }} />);

    expect(screen.getByText('CS108ABC12345')).toBeInTheDocument();
    expect(screen.getByText('2.6.46')).toBeInTheDocument();
    expect(screen.getByText('1.0.20')).toBeInTheDocument();
    expect(screen.getByText('1.0.17')).toBeInTheDocument();
  });

  /**
   * A value the reader did not give up must not render as a blank. Blank reads
   * as "it has none"; the truth is "it did not answer", and that is the
   * difference between a capture that is attributed and one that only looks
   * attributed.
   */
  it('says Unknown for a value that was never read, rather than leaving a gap', () => {
    render(<ReaderDetailsPanel details={{ bluetoothFirmware: '1.0.20' }} />);

    expect(screen.getByText('1.0.20')).toBeInTheDocument();
    // Silicon Labs firmware, RFID firmware, serial, MAC error — four unanswered.
    expect(screen.getAllByText('Unknown')).toHaveLength(4);
  });

  /**
   * `MAC_Error` is the RFID processor's own account of what is wrong, and zero
   * is its healthy value — so it has to be distinguishable from not having been
   * read at all. Rendered in hex because Appendix B's table is in hex.
   */
  it('renders a healthy MAC error as a value, not as an absence', () => {
    render(<ReaderDetailsPanel details={{ macError: 0 }} />);
    expect(screen.getByText('0x0000')).toBeInTheDocument();
  });

  it('renders a non-zero MAC error in the notation the vendor table uses', () => {
    render(<ReaderDetailsPanel details={{ macError: 0x0309 }} />);
    expect(screen.getByText('0x0309')).toBeInTheDocument();
  });

  /**
   * Nothing has been read yet, so there is nothing to be wrong about. A panel
   * of five Unknowns beside a disconnected reader is noise.
   */
  it('renders nothing at all when there is no reader', () => {
    const { container } = render(<ReaderDetailsPanel details={null} />);
    expect(container).toBeEmptyDOMElement();
  });
});
