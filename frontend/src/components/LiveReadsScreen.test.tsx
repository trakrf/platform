import '@testing-library/jest-dom';
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import LiveReadsScreen from './LiveReadsScreen';
import { NAV_LABELS } from '@/lib/routing/navLabels';

/**
 * The feed itself is covered by LiveReadsFeed.test.tsx; this file is about the
 * page chrome — the heading and the sentence under it.
 */
vi.mock('@/components/readerfeed/LiveReadsFeed', () => ({
  LiveReadsFeed: () => <div data-testid="live-reads-feed" />,
}));

describe('LiveReadsScreen copy', () => {
  afterEach(() => cleanup());

  it('titles itself with the label the sidebar shows', () => {
    render(<LiveReadsScreen />);

    expect(screen.getByRole('heading', { name: NAV_LABELS['live-reads'] })).toBeInTheDocument();
  });

  /**
   * TRA-1081: "Inventory" is reserved for the planned Inventory module and is a
   * capability name in the ADR 0002 set — TRA-1029 renamed the Inventory tab to
   * Scan for exactly that reason. It must not come back as a generic word for
   * scanning, which is what "Live tag inventory" made it.
   */
  it('describes the feed without reusing the reserved word "inventory"', () => {
    const { container } = render(<LiveReadsScreen />);

    expect(container.textContent).not.toMatch(/inventory/i);
  });
});
