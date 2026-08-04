import '@testing-library/jest-dom';
import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, cleanup } from '@testing-library/react';
import { BrowserSupportBanner } from '@/components/inventory/BrowserSupportBanner';
import {
  setBluetoothEnvironment as setEnvironment,
  restoreBluetoothEnvironment,
  USER_AGENTS,
} from '../../../../test-utils/bluetoothEnvironment';

/**
 * The banner is the one place an unsupported user is told what to do, so it has
 * to say something they can act on — not "supported browsers: Chrome" to
 * someone holding an iPad, which is what it did before TRA-1078.
 */

function renderBanner() {
  return render(<BrowserSupportBanner />);
}

describe('BrowserSupportBanner', () => {
  afterEach(() => {
    cleanup();
    restoreBluetoothEnvironment();
  });

  it('renders nothing when the browser is supported', () => {
    setEnvironment({ ua: USER_AGENTS.macChrome, bluetooth: true });

    const { container } = renderBanner();

    expect(container).toBeEmptyDOMElement();
  });

  it('sends an iPhone user to Bluefy with a working store link', () => {
    setEnvironment({ ua: USER_AGENTS.iphone });

    renderBanner();

    const link = screen.getByRole('link', { name: /Bluefy/i });
    expect(link).toHaveAttribute(
      'href',
      'https://apps.apple.com/us/app/bluefy-web-ble-browser/id1492822055'
    );
  });

  it('does not offer an iPhone user Chrome as the remedy', () => {
    // Chrome for iOS runs Apple's engine, so it is exactly as dead as Safari.
    // Naming it to rule it out is useful; offering it as the fix is the bug.
    setEnvironment({ ua: USER_AGENTS.iphone });

    renderBanner();

    expect(screen.queryByRole('link', { name: /Chrome/i })).toBeNull();
    expect(screen.getByText(/Bluefy/, { selector: 'span' })).toBeInTheDocument();
    expect(document.body.textContent).toMatch(/Chrome[^.]*cannot reach Bluetooth/);
  });

  it('names the browsers to switch to on an unsupported desktop browser', () => {
    setEnvironment({ ua: USER_AGENTS.macChrome });

    const { container } = renderBanner();

    expect(container.textContent).toMatch(/Chrome, Edge, or Opera/);
  });

  it('diagnoses an insecure context instead of blaming the browser', () => {
    setEnvironment({ ua: USER_AGENTS.macChrome, secureContext: false });

    const { container } = renderBanner();

    expect(container.textContent).toMatch(/https/i);
    expect(container.textContent).not.toMatch(/Chrome, Edge, or Opera/);
  });
});
