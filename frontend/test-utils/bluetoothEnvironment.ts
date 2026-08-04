/**
 * Stub the browser facts `useBluetoothSupport` reads, and put them back after.
 *
 * TRA-1078. `pool: 'forks'` + `singleFork: true` means every test file shares
 * one jsdom, so a bare `Object.defineProperty(navigator, 'userAgent', ...)`
 * outlives the file that wrote it. That is not hypothetical: pointing the UA at
 * desktop Chrome to exercise the recommendation copy flipped
 * `canShareFiles()` — which falls back to UA sniffing when `navigator.canShare`
 * is absent — and failed `exportUtils.test.ts` in full runs while passing on its
 * own. Anything that stubs a navigator property has to restore it.
 */

type Overridable = 'userAgent' | 'maxTouchPoints' | 'userAgentData' | 'bluetooth';

export interface BluetoothEnvironment {
  ua?: string;
  /** Present iff the browser implements Web Bluetooth. */
  bluetooth?: boolean;
  /** ble-mcp-test's injected bridge. */
  bridged?: boolean;
  secureContext?: boolean;
  maxTouchPoints?: number;
  /** Chromium's `navigator.userAgentData.platform`; absent on WebKit and Gecko. */
  userAgentDataPlatform?: string;
  /** Page URL. jsdom serves http://localhost:3000/, which is not a secure origin. */
  href?: string;
}

/** Descriptors as they were before the first override, so restore is exact. */
const originalNavigator = new Map<Overridable, PropertyDescriptor | undefined>();
let originalSecureContext: PropertyDescriptor | undefined;
let capturedSecureContext = false;
let originalLocation: PropertyDescriptor | undefined;
let capturedLocation = false;

function overrideNavigator(property: Overridable, value: unknown) {
  if (!originalNavigator.has(property)) {
    originalNavigator.set(property, Object.getOwnPropertyDescriptor(navigator, property));
  }
  Object.defineProperty(navigator, property, { value, configurable: true, writable: true });
}

export function setBluetoothEnvironment(environment: BluetoothEnvironment = {}) {
  const {
    ua = navigator.userAgent,
    bluetooth = false,
    bridged = false,
    secureContext = true,
    maxTouchPoints = 0,
    userAgentDataPlatform,
    href,
  } = environment;

  if (href !== undefined) {
    if (!capturedLocation) {
      originalLocation = Object.getOwnPropertyDescriptor(window, 'location');
      capturedLocation = true;
    }
    // Only `location.href` is read by the code under test, so a plain object is
    // enough — and assigning to the real jsdom Location throws on navigation.
    Object.defineProperty(window, 'location', {
      value: { ...window.location, href },
      configurable: true,
      writable: true,
    });
  }

  overrideNavigator('userAgent', ua);
  overrideNavigator('maxTouchPoints', maxTouchPoints);
  overrideNavigator('bluetooth', bluetooth ? { requestDevice: () => Promise.resolve() } : undefined);
  overrideNavigator(
    'userAgentData',
    userAgentDataPlatform ? { platform: userAgentDataPlatform } : undefined
  );

  if (!capturedSecureContext) {
    originalSecureContext = Object.getOwnPropertyDescriptor(window, 'isSecureContext');
    capturedSecureContext = true;
  }
  Object.defineProperty(window, 'isSecureContext', { value: secureContext, configurable: true });

  window.__webBluetoothBridged = bridged || undefined;
}

export function restoreBluetoothEnvironment() {
  for (const [property, descriptor] of originalNavigator) {
    if (descriptor) {
      Object.defineProperty(navigator, property, descriptor);
    } else {
      // The property did not exist before the stub; leaving `undefined` behind
      // would still shadow a real one in a later file.
      delete (navigator as unknown as Record<string, unknown>)[property];
    }
  }
  originalNavigator.clear();

  if (capturedSecureContext) {
    if (originalSecureContext) {
      Object.defineProperty(window, 'isSecureContext', originalSecureContext);
    } else {
      delete (window as unknown as Record<string, unknown>).isSecureContext;
    }
    capturedSecureContext = false;
    originalSecureContext = undefined;
  }

  if (capturedLocation) {
    if (originalLocation) {
      Object.defineProperty(window, 'location', originalLocation);
    } else {
      delete (window as unknown as Record<string, unknown>).location;
    }
    capturedLocation = false;
    originalLocation = undefined;
  }

  window.__webBluetoothBridged = undefined;
}

/** Real user agents, so the parsing under test faces what it will in the field. */
export const USER_AGENTS = {
  macChrome:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  macSafari:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
  windows:
    'Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  linux:
    'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Safari/537.36',
  android:
    'Mozilla/5.0 (Linux; Android 14; Pixel 8 Pro) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0.0.0 Mobile Safari/537.36',
  iphone:
    'Mozilla/5.0 (iPhone; CPU iPhone OS 17_4 like Mac OS X) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Mobile/15E148 Safari/604.1',
  /** iPadOS 13+ asks for desktop sites by default and reports itself as a Mac. */
  ipadDesktopMode:
    'Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/17.4 Safari/605.1.15',
} as const;
