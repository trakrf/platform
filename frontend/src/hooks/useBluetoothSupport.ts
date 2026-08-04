import { useCallback, useEffect, useState } from 'react';

/**
 * The single answer to "can this browser talk to a scanner, and if not, what
 * should this user do about it?" (TRA-1078, closing TRA-338's residual).
 *
 * Two separate questions live here, and conflating them is the bug this hook
 * exists to prevent:
 *
 *   - **Can it?** `navigator.bluetooth`. Capability-based, no UA sniffing. It is
 *     absent on desktop Safari, on Firefox, and on *every* iOS browser — Apple
 *     forbids non-WebKit engines, so Chrome for iOS is exactly as dead as
 *     Safari. A UA string must never move this answer.
 *   - **What now?** Needs the OS family, and only to pick wording. A wrong guess
 *     costs a suboptimal suggestion, never a broken gate, which is why the
 *     brittleness of UA parsing is acceptable here and nowhere else.
 */

export type Platform = 'ios' | 'android' | 'macos' | 'windows' | 'linux' | 'unknown';

export type BluetoothUnsupportedReason =
  /** Web Bluetooth needs HTTPS or localhost; over plain http the API is simply absent. */
  | 'insecure-context'
  /** The platform works, but only through a non-Safari browser Apple permits. */
  | 'ios-webkit'
  /** Ordinary "this browser doesn't implement Web Bluetooth". */
  | 'unsupported-browser';

export interface BluetoothRecommendationLink {
  label: string;
  url: string;
}

export interface BluetoothRecommendation {
  /** The browsers that work on this platform, phrased for inline use. */
  browsers: string;
  /** Why, in a sentence or two — the only place this is written down. */
  note: string;
  /** Where to get them, when there is somewhere to send the user. */
  links: BluetoothRecommendationLink[];
  /**
   * Deep link that reopens the current page in the recommended browser, for a
   * user who already has it installed. Absent unless one exists and would work.
   */
  openInBrowserUrl?: string;
}

export interface BluetoothSupport {
  supported: boolean;
  reason: BluetoothUnsupportedReason | null;
  recommendation: BluetoothRecommendation;
  platform: Platform;
}

const BLUEFY_URL = 'https://apps.apple.com/us/app/bluefy-web-ble-browser/id1492822055';

/**
 * The recommendation matrix. Every string a user reads about browser support
 * comes from here — the banner and Help both render it, so the two cannot drift
 * apart the way they had before TRA-1078.
 *
 * Verification status as of 2026-08-04:
 *   - android — platform detection confirmed on a Pixel 8 Pro: Firefox for
 *              Android (the one Android browser with no Web Bluetooth) raises
 *              the banner with this row's copy, so `userAgentData.platform`
 *              resolves to android on real hardware. The browser list itself is
 *              from MDN browser-compat-data: Chrome Android is an explicit 56+,
 *              while Edge / Opera / Samsung Internet are `mirror` entries —
 *              inherited from upstream Chromium rather than independently
 *              measured. Chrome leads the list because it is the verified one.
 *   - windows — NOT verified, and not verifiable here: the only Windows box has
 *              no working Bluetooth driver since a hardware swap. The list is
 *              the same Chromium browsers that were confirmed on macOS by real
 *              connects, and Windows has no per-app Bluetooth permission model
 *              to trip over, so the risk is low — but it is inferred, not
 *              measured. Say so rather than letting it read as checked.
 *   - macos  — fully verified on a MacBook Pro, 2026-08-04. Safari and Firefox
 *              raise the banner with this row's copy; Chrome, Edge and Opera
 *              each connected to a CS108 and read tags. The bar was a real
 *              connect, not an absent banner: Brave exposes navigator.bluetooth,
 *              passes the gate, and then fails with "Web Bluetooth API globally
 *              disabled" — so "no banner" proves nothing on its own.
 *   - linux  — the flag and BlueZ >= 5.41 caveats are repeated from the ticket,
 *              not measured. Version-dependent.
 *   - ios    — verified on an iPad, 2026-08-04. Safari raises the banner with
 *              this row's copy, and the bluefy:// link opens the page in Bluefy.
 *              Bluefy is free (App Store listing checked the same day: Free, no
 *              in-app purchases).
 *
 * WebBLE (apps.apple.com/us/app/webble/id1193531073) is the other
 * WebKit+native-BLE browser and is deliberately NOT recommended: checked
 * 2026-08-04, it is paid and poorly rated. Sending a stuck user to a paid app
 * with bad reviews is worse than sending them nowhere, and Bluefy being free
 * makes it unnecessary. Do not add it back as a "second option" — see the guard
 * in useBluetoothSupport.test.ts.
 */
const RECOMMENDATIONS: Record<Platform, BluetoothRecommendation> = {
  windows: {
    browsers: 'Chrome, Edge, or Opera',
    note: "Safari and Firefox can't connect to your scanner — they don't support the Bluetooth features this app needs.",
    links: [],
  },
  macos: {
    browsers: 'Chrome, Edge, or Opera',
    note: "Safari and Firefox can't connect to your scanner — they don't support the Bluetooth features this app needs.",
    links: [],
  },
  android: {
    browsers: 'Chrome, Edge, Opera, or Samsung Internet',
    note: "Firefox for Android can't connect to your scanner — it doesn't support the Bluetooth features this app needs.",
    links: [],
  },
  ios: {
    browsers: 'Bluefy',
    note: 'On iPhone and iPad, Apple only allows its own browser engine, so Safari — and Chrome, which uses the same engine — cannot reach Bluetooth. Bluefy is a free browser that can. Install it and open this app there. A desktop browser works too, if you would rather not add another browser.',
    links: [{ label: 'Get Bluefy on the App Store', url: BLUEFY_URL }],
  },
  linux: {
    browsers: 'Chrome or Chromium',
    note: "Firefox can't connect to your scanner. On some distributions Chrome also needs BlueZ 5.41 or newer, and Web Bluetooth enabled at chrome://flags/#enable-experimental-web-platform-features.",
    links: [],
  },
  unknown: {
    browsers: 'Chrome, Edge, or Opera',
    note: "Safari and Firefox can't connect to your scanner — they don't support the Bluetooth features this app needs.",
    links: [],
  },
};

/**
 * Bluefy's custom URL scheme, so an iOS user who already installed it can jump
 * straight over instead of reinstalling from the App Store.
 *
 * Verified on a real iPad, 2026-08-04: `bluefy://` prompts "Open this page in
 * Bluefy?" and opens the app; `bluefy://app.preview.trakrf.id` prompts with the
 * host attached; `bluefys://` errors. There is no TLS variant because https is
 * implied — Bluefy only loads secure origins, which is also why an http page
 * gets no link at all rather than one that fails on arrival.
 *
 * iOS gives a web page no way to ask whether an app is installed, so this is
 * offered alongside the App Store link rather than instead of it. Do not try to
 * detect installation with a scheme-plus-timeout race: it is unreliable, and on
 * modern iOS the "address is invalid" dialog fires anyway.
 */
export function bluefyLinkFor(href: string): string | undefined {
  const HTTPS = 'https://';
  if (!href.startsWith(HTTPS)) return undefined;
  return `bluefy://${href.slice(HTTPS.length)}`;
}

const INSECURE_CONTEXT_NOTE =
  'Bluetooth is only available over a secure connection. Open this app at an https:// address (or on localhost) and reload — your browser is fine.';

interface UserAgentData {
  platform?: string;
}

/** Chromium exposes the OS directly; everything else falls through to the UA string. */
function platformFromUserAgentData(): Platform | null {
  const uaData = (navigator as Navigator & { userAgentData?: UserAgentData }).userAgentData;
  const platform = uaData?.platform?.toLowerCase();
  if (!platform) return null;

  if (platform.includes('android')) return 'android';
  if (platform.includes('windows')) return 'windows';
  if (platform.includes('mac')) return 'macos';
  if (platform.includes('linux') || platform.includes('chrome os')) return 'linux';
  return null;
}

/**
 * UA-string fallback, following the precedent already set in
 * `utils/shareUtils.ts` rather than introducing a second approach.
 */
function platformFromUserAgent(ua: string): Platform {
  if (/iPhone|iPad|iPod/.test(ua)) return 'ios';

  // iPadOS 13+ requests desktop sites by default and reports itself as a Mac.
  // Touch points is the standard discriminator; Macs report 0.
  if (/Macintosh/.test(ua) && navigator.maxTouchPoints > 1) return 'ios';

  // Android's UA contains "Linux", so it has to be tested first.
  if (/Android/.test(ua)) return 'android';
  if (/Windows/.test(ua)) return 'windows';
  if (/Macintosh|Mac OS X/.test(ua)) return 'macos';
  if (/Linux|X11|CrOS/.test(ua)) return 'linux';
  return 'unknown';
}

export function detectPlatform(): Platform {
  if (typeof navigator === 'undefined') return 'unknown';

  // userAgentData never reports iOS — no Chromium ships there — so consulting
  // it first cannot mask an iPhone.
  return platformFromUserAgentData() ?? platformFromUserAgent(navigator.userAgent ?? '');
}

/**
 * Reads the environment once. Pure with respect to React so it can be called
 * from anywhere and tested without a renderer.
 */
export function detectBluetoothSupport(): BluetoothSupport {
  const platform = detectPlatform();
  const recommendation = RECOMMENDATIONS[platform];

  let hasBluetoothAPI = false;
  try {
    hasBluetoothAPI = typeof navigator !== 'undefined' && !!navigator.bluetooth;
  } catch {
    // Some embedded browsers throw on property access rather than returning
    // undefined. That is a "no", not a crash.
    hasBluetoothAPI = false;
  }

  // ble-mcp-test injects a bridge without ever defining navigator.bluetooth,
  // and it must keep reporting supported or Playwright loses its connect path.
  const isBridged = typeof window !== 'undefined' && !!window.__webBluetoothBridged;

  if (hasBluetoothAPI || isBridged) {
    return { supported: true, reason: null, recommendation, platform };
  }

  const isSecure = typeof window === 'undefined' || window.isSecureContext !== false;

  if (!isSecure) {
    return {
      supported: false,
      reason: 'insecure-context',
      recommendation: { ...recommendation, note: INSECURE_CONTEXT_NOTE },
      platform,
    };
  }

  if (platform === 'ios') {
    const openInBrowserUrl =
      typeof window === 'undefined' ? undefined : bluefyLinkFor(window.location.href);

    return {
      supported: false,
      reason: 'ios-webkit',
      recommendation: { ...recommendation, openInBrowserUrl },
      platform,
    };
  }

  return { supported: false, reason: 'unsupported-browser', recommendation, platform };
}

/**
 * React binding. Re-checks when the mock bridge announces itself, which is the
 * only way this answer changes without a reload.
 */
export function useBluetoothSupport(): BluetoothSupport {
  const [support, setSupport] = useState<BluetoothSupport>(detectBluetoothSupport);

  const recheck = useCallback(() => setSupport(detectBluetoothSupport()), []);

  useEffect(() => {
    recheck();

    window.addEventListener('webBluetoothMockReady', recheck);
    return () => window.removeEventListener('webBluetoothMockReady', recheck);
  }, [recheck]);

  return support;
}
