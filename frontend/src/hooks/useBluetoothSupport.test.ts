import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest';
import { renderHook, act } from '@testing-library/react';
import {
  detectBluetoothSupport,
  useBluetoothSupport,
  bluefyLinkFor,
} from '@/hooks/useBluetoothSupport';
import {
  setBluetoothEnvironment,
  restoreBluetoothEnvironment,
  USER_AGENTS,
} from '../../test-utils/bluetoothEnvironment';

/**
 * The gate is capability-based (`navigator.bluetooth`) and must stay that way —
 * these tests never let a UA string change `supported`, only the wording of the
 * advice. TRA-1078.
 */

const UA = USER_AGENTS;

const setEnvironment = setBluetoothEnvironment;

describe('detectBluetoothSupport', () => {
  afterEach(() => {
    restoreBluetoothEnvironment();
  });

  describe('the capability gate', () => {
    it('reports supported when navigator.bluetooth exists', () => {
      setEnvironment({ ua: UA.macChrome, bluetooth: true });

      const { supported, reason } = detectBluetoothSupport();

      expect(supported).toBe(true);
      expect(reason).toBeNull();
    });

    it('reports supported when only the mock bridge is present', () => {
      // ble-mcp-test injects the bridge without ever defining navigator.bluetooth.
      setEnvironment({ ua: UA.macChrome, bluetooth: false, bridged: true });

      expect(detectBluetoothSupport().supported).toBe(true);
    });

    it('keeps the mock bridge supported over plain http', () => {
      // Playwright runs against http://localhost in some configurations; the
      // bridge answer must outrank the secure-context diagnosis.
      setEnvironment({ ua: UA.macChrome, bridged: true, secureContext: false });

      expect(detectBluetoothSupport().supported).toBe(true);
    });

    it('reports unsupported on a browser without the API', () => {
      setEnvironment({ ua: UA.macSafari, bluetooth: false });

      expect(detectBluetoothSupport().supported).toBe(false);
    });

    it('does not let an iOS user agent flip a working API to unsupported', () => {
      setEnvironment({ ua: UA.iphone, bluetooth: true });

      expect(detectBluetoothSupport().supported).toBe(true);
    });
  });

  describe('reasons', () => {
    it('blames the insecure context rather than the browser over plain http', () => {
      setEnvironment({ ua: UA.macChrome, bluetooth: false, secureContext: false });

      const { reason, recommendation } = detectBluetoothSupport();

      expect(reason).toBe('insecure-context');
      expect(recommendation.note).toMatch(/https/i);
    });

    it('blames WebKit on iOS rather than the browser choice', () => {
      setEnvironment({ ua: UA.iphone, bluetooth: false });

      expect(detectBluetoothSupport().reason).toBe('ios-webkit');
    });

    it('blames the browser everywhere else', () => {
      setEnvironment({ ua: UA.macSafari, bluetooth: false });

      expect(detectBluetoothSupport().reason).toBe('unsupported-browser');
    });
  });

  describe('platform detection', () => {
    it('reads the platform from userAgentData when Chromium provides it', () => {
      setEnvironment({ ua: UA.macChrome, userAgentDataPlatform: 'Windows' });

      expect(detectBluetoothSupport().platform).toBe('windows');
    });

    it('falls back to the user agent string when userAgentData is absent', () => {
      setEnvironment({ ua: UA.linux });

      expect(detectBluetoothSupport().platform).toBe('linux');
    });

    it('reads Android from the user agent before Linux', () => {
      // The Android UA contains "Linux"; order matters.
      setEnvironment({ ua: UA.android });

      expect(detectBluetoothSupport().platform).toBe('android');
    });

    it('detects an iPad running in desktop mode as iOS', () => {
      setEnvironment({ ua: UA.ipadDesktopMode, maxTouchPoints: 5 });

      expect(detectBluetoothSupport().platform).toBe('ios');
    });

    it('does not mistake a touchscreen Mac laptop for an iPad', () => {
      setEnvironment({ ua: UA.macChrome, maxTouchPoints: 0 });

      expect(detectBluetoothSupport().platform).toBe('macos');
    });
  });

  describe('recommendations', () => {
    it('names Bluefy with a store link on iOS', () => {
      setEnvironment({ ua: UA.iphone });

      const { recommendation } = detectBluetoothSupport();

      expect(recommendation.browsers).toMatch(/Bluefy/);
      expect(recommendation.links.map((l) => l.url)).toContain(
        'https://apps.apple.com/us/app/bluefy-web-ble-browser/id1492822055'
      );
    });

    it('does not call Bluefy paid, and does not frame it as a cost', () => {
      // An earlier revision of TRA-1078 said Bluefy was paid. It is free, and
      // the copy must not reintroduce a cost objection to work around.
      setEnvironment({ ua: UA.iphone });

      const { recommendation } = detectBluetoothSupport();
      const copy = `${recommendation.browsers} ${recommendation.note}`;

      expect(copy).not.toMatch(/paid|purchase|\$|costs?\b/i);
      expect(copy).toMatch(/free/i);
    });

    it('never recommends WebBLE', () => {
      // Checked 2026-08-04: WebBLE is paid and poorly rated. Sending a stuck
      // user to a paid app with bad reviews is worse than sending them nowhere,
      // and Bluefy being free makes it unnecessary. This guard exists because
      // "Bluefy or WebBLE" reads like an obvious improvement to anyone who has
      // not looked at the listing.
      setEnvironment({ ua: UA.iphone });

      const { recommendation } = detectBluetoothSupport();
      const copy = `${recommendation.browsers} ${recommendation.note}`;

      expect(copy).not.toMatch(/WebBLE/i);
      expect(recommendation.links.map((link) => link.url).join(' ')).not.toMatch(/webble/i);
    });

    it('never tells an iOS user to switch to Chrome', () => {
      // Apple forbids non-WebKit engines, so Chrome for iOS is equally dead.
      setEnvironment({ ua: UA.iphone });

      expect(detectBluetoothSupport().recommendation.browsers).not.toMatch(/Chrome/);
    });

    it('names the Chromium browsers on desktop', () => {
      setEnvironment({ ua: UA.windows });

      expect(detectBluetoothSupport().recommendation.browsers).toMatch(/Chrome/);
    });

    it('mentions the BlueZ and flag caveats on Linux', () => {
      setEnvironment({ ua: UA.linux });

      const { recommendation } = detectBluetoothSupport();

      expect(recommendation.note).toMatch(/BlueZ/);
    });

    it('offers to reopen the current page in Bluefy on iOS', () => {
      setEnvironment({ ua: UA.iphone, href: 'https://app.trakrf.id/?tab=scan' });

      expect(detectBluetoothSupport().recommendation.openInBrowserUrl).toBe(
        'bluefy://app.trakrf.id/?tab=scan'
      );
    });

    it('offers no reopen link on a platform with no such browser', () => {
      setEnvironment({ ua: UA.macSafari, href: 'https://app.trakrf.id/' });

      expect(detectBluetoothSupport().recommendation.openInBrowserUrl).toBeUndefined();
    });

    it('offers no reopen link when the page itself is the problem', () => {
      // Bluefy requires https, so handing it an http URL just moves the same
      // failure into a second browser.
      setEnvironment({ ua: UA.iphone, href: 'http://app.trakrf.id/', secureContext: false });

      const { reason, recommendation } = detectBluetoothSupport();

      expect(reason).toBe('insecure-context');
      expect(recommendation.openInBrowserUrl).toBeUndefined();
    });

    it('still answers which browsers to use on a supported browser', () => {
      // Help asks the question regardless of whether the user is stuck.
      setEnvironment({ ua: UA.macChrome, bluetooth: true });

      const { supported, recommendation } = detectBluetoothSupport();

      expect(supported).toBe(true);
      expect(recommendation.browsers).toMatch(/Chrome/);
      expect(recommendation.note).not.toHaveLength(0);
    });
  });

  /**
   * TRA-1100. Windows will not let the browser reach a CS108 that has never been
   * paired in Settings, and says so only as a generic NetworkError. The
   * prerequisite is a property of the OS, not of the browser or of whether the
   * connect has failed yet, so it is answered here rather than guessed at from
   * an exception.
   */
  describe('the setup prerequisite', () => {
    it('warns a Windows user that the chooser will not name the scanner', () => {
      setEnvironment({ ua: UA.windows });

      const { setupPrerequisite } = detectBluetoothSupport();

      expect(setupPrerequisite).not.toBeNull();
      expect(setupPrerequisite?.helpStep).toMatch(/pair/i);
      expect(setupPrerequisite?.helpStep).toMatch(/Bluetooth/);
    });

    it('quotes the label Edge actually shows, since that is the confirmed fact', () => {
      // Screenshotted on the GMKtec M6 with the CS108 removed from Bluetooth
      // settings, 2026-08-06: "Unknown or unsupported device (6C:79:B8:26:03:A7)".
      // A customer reading "unsupported" next to a hex string cancels the dialog.
      setEnvironment({ ua: UA.windows });

      expect(detectBluetoothSupport().setupPrerequisite?.helpStep).toMatch(
        /Unknown or unsupported device/
      );
    });

    it('does not tell a Windows user that connecting will fail', () => {
      // It did not fail on 2026-08-06 — selecting the unnamed device and
      // clicking Pair connected and read tags. Stating failure as a certainty
      // sends people into system settings for a step they may not need.
      setEnvironment({ ua: UA.windows });

      const helpStep = detectBluetoothSupport().setupPrerequisite?.helpStep ?? '';

      expect(helpStep).not.toMatch(/connecting fails|will fail|cannot connect/i);
    });

    it('hedges the system-settings pairing rather than demanding it', () => {
      // Whether Windows ever truly requires it is unsettled — see the note on
      // SETUP_PREREQUISITES. "You may need to" is the strongest claim the
      // evidence supports; anything firmer outruns it.
      setEnvironment({ ua: UA.windows });

      const helpStep = detectBluetoothSupport().setupPrerequisite?.helpStep ?? '';

      expect(helpStep).toMatch(/Settings . Bluetooth/);
      expect(helpStep).toMatch(/may need to/i);
    });

    it('says what adding it in system settings buys you, so the step is not a blind maybe', () => {
      // Once Windows has bonded the scanner it can read the GAP name, so the
      // chooser stops saying "Unknown or unsupported device". That is a reason
      // to bother even on a machine where connecting works without it.
      setEnvironment({ ua: UA.windows });

      const helpStep = detectBluetoothSupport().setupPrerequisite?.helpStep ?? '';
      const afterSettingsMention = helpStep.slice(helpStep.search(/Settings . Bluetooth/));

      expect(afterSettingsMention).toMatch(/name/i);
    });

    it('asks for nothing extra on macOS', () => {
      // Verified on a MacBook Pro, 2026-08-04: Chrome, Edge and Opera each
      // connected to a CS108 with no OS-level pairing at all.
      setEnvironment({ ua: UA.macChrome });

      expect(detectBluetoothSupport().setupPrerequisite).toBeNull();
    });

    it('asks for nothing extra on the platforms that were never observed needing it', () => {
      // Only Windows was seen to require bonding. A prerequisite invented for a
      // platform nobody tested is advice that wastes the user's time.
      for (const ua of [UA.linux, UA.android, UA.iphone]) {
        setEnvironment({ ua });

        expect(detectBluetoothSupport().setupPrerequisite).toBeNull();
      }
    });

    it('gives Windows a self-identifying string the other desktop rows do not have', () => {
      // windows, macos and unknown share word-for-word identical recommendation
      // copy, so until now nothing on screen could prove which row rendered.
      setEnvironment({ ua: UA.windows });
      const windows = detectBluetoothSupport().setupPrerequisite;

      setEnvironment({ ua: UA.macSafari });
      const macos = detectBluetoothSupport().setupPrerequisite;

      expect(windows).not.toEqual(macos);
    });

    it('answers on Windows even when the browser already works', () => {
      // The prerequisite is not a failure diagnosis — Help states it up front,
      // before the user has attempted anything.
      setEnvironment({ ua: UA.windows, bluetooth: true });

      const { supported, setupPrerequisite } = detectBluetoothSupport();

      expect(supported).toBe(true);
      expect(setupPrerequisite?.helpStep).toMatch(/pair/i);
    });

    it('answers on Windows even when the browser cannot do Bluetooth at all', () => {
      // Firefox on Windows raises the banner; Help still has to be able to state
      // the pairing step, so this must not be gated on `supported`.
      setEnvironment({ ua: UA.windows, bluetooth: false, secureContext: false });

      const { supported, setupPrerequisite } = detectBluetoothSupport();

      expect(supported).toBe(false);
      expect(setupPrerequisite?.helpStep).toMatch(/pair/i);
    });

    it('hedges the connect-failure hint instead of diagnosing the cause', () => {
      // "NetworkError: Connection attempt failed." is Chromium's generic GATT
      // failure — equally a scanner that is off, out of range, or flat. Asserting
      // that pairing is the cause would be wrong more often than right.
      setEnvironment({ ua: UA.windows });

      const hint = detectBluetoothSupport().setupPrerequisite?.connectHint ?? '';

      expect(hint).toMatch(/\bif\b/i);
      expect(hint).toMatch(/first time/i);
    });
  });
});

describe('bluefyLinkFor', () => {
  // Verified on a real iPad, 2026-08-04: `bluefy://` prompts and opens Bluefy,
  // `bluefy://app.preview.trakrf.id` prompts with the host attached, and
  // `bluefys://` errors — there is no TLS variant, https is implied.
  const ORIGIN = 'https://app.trakrf.id';

  it('swaps the https scheme for bluefy', () => {
    expect(bluefyLinkFor('https://app.trakrf.id/', ORIGIN)).toBe('bluefy://app.trakrf.id/');
  });

  it('carries the path, query and hash across', () => {
    expect(bluefyLinkFor('https://app.trakrf.id/scan?tab=rfid#tags', ORIGIN)).toBe(
      'bluefy://app.trakrf.id/scan?tab=rfid#tags'
    );
  });

  it('refuses an http page, which Bluefy cannot use either', () => {
    expect(bluefyLinkFor('http://app.trakrf.id/', 'http://app.trakrf.id')).toBeUndefined();
  });

  it('refuses a host that is not the page we are on', () => {
    // This only ever reopens the current page. Anything else is somebody
    // steering the scheme handler somewhere we did not intend.
    expect(bluefyLinkFor('https://evil.example/scan', ORIGIN)).toBeUndefined();
  });

  it('is not fooled by a host smuggled into the credentials', () => {
    // `https://app.trakrf.id@evil.example/` parses to host evil.example, so
    // stripping credentials is not enough on its own — the origin check is
    // what actually closes this.
    expect(bluefyLinkFor('https://app.trakrf.id@evil.example/', ORIGIN)).toBeUndefined();
  });

  it('refuses a string that is not a URL at all', () => {
    expect(bluefyLinkFor('https:/ /not a url', ORIGIN)).toBeUndefined();
    expect(bluefyLinkFor('', ORIGIN)).toBeUndefined();
  });
});

describe('useBluetoothSupport', () => {
  beforeEach(() => {
    setEnvironment({ ua: UA.macChrome, bluetooth: false });
  });

  afterEach(() => {
    restoreBluetoothEnvironment();
  });

  it('re-checks when the mock bridge announces itself', () => {
    const { result } = renderHook(() => useBluetoothSupport());
    expect(result.current.supported).toBe(false);

    act(() => {
      window.__webBluetoothBridged = true;
      window.dispatchEvent(new Event('webBluetoothMockReady'));
    });

    expect(result.current.supported).toBe(true);
  });

  it('stops listening once unmounted', () => {
    const removeSpy = vi.spyOn(window, 'removeEventListener');

    const { unmount } = renderHook(() => useBluetoothSupport());
    unmount();

    expect(removeSpy).toHaveBeenCalledWith('webBluetoothMockReady', expect.any(Function));
    removeSpy.mockRestore();
  });
});
