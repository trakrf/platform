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

/**
 * A step the operating system demands before the browser can reach a scanner at
 * all — distinct from `BluetoothRecommendation`, which is only ever about which
 * browser to run. Two phrasings because the two surfaces are asking different
 * questions, and both come from here so they cannot drift (TRA-1100).
 */
export interface BluetoothSetupPrerequisite {
  /** Stated up front in Help, before the user has attempted anything. */
  helpStep: string;
  /**
   * Offered after a connect has already failed, and therefore hedged: the
   * exception that gets this far does not say what went wrong.
   */
  connectHint: string;
}

export interface BluetoothSupport {
  supported: boolean;
  reason: BluetoothUnsupportedReason | null;
  recommendation: BluetoothRecommendation;
  platform: Platform;
  /** What this OS needs doing once, before the first connect. Usually nothing. */
  setupPrerequisite: BluetoothSetupPrerequisite | null;
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
 *   - windows — verified on a GMKtec M6 once its MediaTek driver was installed,
 *              2026-08-04: Edge connects to a CS108 and works, and
 *              navigator.userAgentData.platform reads 'Windows' — checked
 *              directly, because this row's copy cannot prove it (see below).
 *              The pairing prerequisite Windows also imposes is NOT stated in
 *              this row — it is a property of the OS rather than of the browser
 *              choice, so it lives in SETUP_PREREQUISITES below (TRA-1100).
 *   - macos  — fully verified on a MacBook Pro, 2026-08-04: Chrome, Edge and
 *              Opera each connected to a CS108 and read tags, Safari and Firefox
 *              raise the banner, and navigator.userAgentData.platform reads
 *              'macOS' — checked directly, since this row's copy cannot prove it
 *              (see below). The bar was a real connect,
 *              not an absent banner: Brave exposes navigator.bluetooth, passes
 *              the gate, and then fails with "Web Bluetooth API globally
 *              disabled" — so "no banner" proves nothing on its own.
 *
 * Note what the rendered copy can and cannot prove. macos, windows and unknown
 * are deliberately word-for-word identical, so seeing "Chrome, Edge, or Opera"
 * on screen confirms the path renders but NOT which of the three produced it —
 * a fallthrough to `unknown` looks exactly the same. Only ios ("Bluefy"),
 * android ("...or Samsung Internet") and linux ("Chrome or Chromium") are
 * self-identifying. (Since TRA-1100 the windows *prerequisite* is
 * self-identifying, but that is a separate string from this matrix, and only on
 * the surfaces that render it.) The other two were confirmed by reading
 * navigator.userAgentData.platform directly on real hardware — 'Windows' and
 * 'macOS', both 2026-08-04 — rather than by trusting what appeared on screen.
 * Do the same for any row added later: identical copy makes a fallthrough to
 * unknown invisible, and a detection bug would hide behind advice that still
 * happens to read correctly.
 *   - linux  — verified on an Intel NUC6 running Ubuntu with Chrome stable,
 *              2026-08-04, and the flag caveat is REAL rather than folklore.
 *              Without chrome://flags/#enable-experimental-web-platform-features
 *              navigator.bluetooth is absent, so the banner fires — and this
 *              note is what told the tester to set the flag, after which a CS108
 *              connected and scanned tags. Do not tidy that sentence away for
 *              sounding alarmist; it is load-bearing.
 *              The BlueZ >= 5.41 threshold is a different matter: still
 *              unverified, and NOT settleable by observation. A machine that
 *              works proves only that its own version suffices, never that 5.41
 *              is the floor. Changing that number needs the Chromium source or
 *              a deliberately downgraded stack, not another passing test.
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
 * What the OS itself makes awkward before any of the above matters. Windows is
 * the only entry (TRA-1100), and the copy is worded around two observations on
 * the same GMKtec M6 / Windows 11 25H2 / Edge box that do not agree.
 *
 * CONFIRMED, both runs — the chooser lists the CS108 *unnamed*. We filter on the
 * CS108 service UUID so the right device is offered, but Windows will not
 * surface the GAP name before bonding, and Edge fills the gap with
 * "Unknown or unsupported device (6C:79:B8:26:03:A7)". That literal wording is
 * the reason this entry exists at all: a customer reading "unsupported" beside a
 * hex string concludes they have the wrong device and cancels the dialog. It is
 * the one thing here we can state flatly.
 *
 * UNSETTLED — whether OS-level pairing is ever actually *required*.
 *   - 2026-08-04: a CS108 never paired in Settings → Bluetooth & devices failed
 *     to connect; pairing it there (classic pairing, PIN 0000) fixed it.
 *   - 2026-08-06: the CS108 was removed from Bluetooth settings and the flow run
 *     again. The chooser showed it unnamed as before, but selecting it and
 *     clicking Pair connected and read 12 tags. No failure at all.
 * The likeliest explanation is that Chromium's own chooser initiates the bond,
 * and that the first run hit something else — or that Windows kept registry
 * residue from the earlier pairing, which a "Remove device" does not reliably
 * clear. Settling it needs a clean Windows install on hardware that has never
 * seen this reader; short of that, no observation on this box can distinguish
 * the two. So `helpStep` says "you may need to" and never asserts a failure that
 * did not happen the second time.
 *
 * It does still point at Settings → Bluetooth & devices, because that step earns
 * its place whether or not it is ever required. CONFIRMED by screenshot
 * 2026-08-06: after pairing there, the browser chooser reads
 * "CS108Reader2603A7 - Paired" where it had read "Unknown or unsupported device
 * (6C:79:B8:26:03:A7)". Windows also demands a PIN during that pairing —
 * "Enter the PIN for CS108Reader2603A7", answered with 0000 — which is why the
 * copy carries the PIN rather than leaving the user stuck at the prompt. So the
 * sentence is not a blind maybe: it buys a legible device name on any machine.
 *
 * That same session exposed something this PR does not try to fix. Windows
 * Settings' own "Add a device" scan lists the reader as "CS108Reader2603A7"
 * *without* any prior bonding, so the CS108 clearly advertises a usable name and
 * the OS can read it. Only Chromium's pre-bond chooser cannot. That asymmetry
 * suggests the unnamed device may be fixable at the source — a scan-response vs
 * advertisement-payload question, or how Chromium's WinRT advertisement watcher
 * is configured — rather than being something we are stuck explaining in Help.
 * Worth its own ticket; it is out of scope here, and this entry is the interim
 * answer either way.
 *
 * macOS needs none of it: Chrome, Edge and Opera on a MacBook Pro each connected
 * and read tags with no OS-level pairing at all, so this is Windows-specific
 * rather than a CS108 quirk.
 *
 * Note what is deliberately NOT here. The 2026-08-04 failure surfaced as
 * `NetworkError: Connection attempt failed.` — Chromium's generic GATT failure,
 * which equally means the scanner is switched off, out of range, already
 * claimed by another host, or flat. So there is no branch on that exception, and
 * `connectHint` never asserts pairing is the cause. That restraint reads better
 * now than when it was written: had we branched on it, every Mac user with a
 * flat reader would be reading about Windows Bluetooth settings, to fix a
 * prerequisite that may not exist.
 *
 * Only add a platform here on the strength of a real device that needed it. A
 * prerequisite invented for a platform nobody tested sends users into system
 * settings for nothing.
 */
const SETUP_PREREQUISITES: Partial<Record<Platform, BluetoothSetupPrerequisite>> = {
  windows: {
    helpStep:
      'On Windows your scanner shows up as "Unknown or unsupported device" followed by a string of numbers, rather than by name. That is normal, and it is the right device — select it and click Pair. Adding it in Settings → Bluetooth & devices first (PIN 0000 if it asks) makes it show up by name instead, and you may need to do that anyway if it will not connect.',
    connectHint:
      'If this is your first time connecting this scanner on Windows, you may need to pair it in Settings → Bluetooth & devices first.',
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
export function bluefyLinkFor(
  href: string,
  origin: string = typeof window === 'undefined' ? '' : window.location.origin
): string | undefined {
  let url: URL;
  try {
    url = new URL(href);
  } catch {
    return undefined;
  }

  // Bluefy only loads secure origins, so an http page would just move the same
  // failure into a second browser.
  if (url.protocol !== 'https:') return undefined;

  // This reopens *the page you are on*, so the only origin it may ever produce
  // is the one already loaded. Without this the authority comes from the input:
  // `https://app.trakrf.id@evil.com/` parses to host evil.com, and stripping
  // credentials would not save us. CodeQL flags the location -> href flow for
  // exactly this reason, and it is right that the guard belongs here rather
  // than in an unwritten assumption about who calls this.
  if (url.origin !== origin) return undefined;

  // Rebuilt from parsed components so nothing in the input can smuggle its way
  // into the authority.
  return `bluefy://${url.host}${url.pathname}${url.search}${url.hash}`;
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

  // Derived from the OS alone, so it is the same answer on every branch below —
  // Help states it before the user has attempted anything, and a browser that
  // cannot do Bluetooth at all does not make the pairing step untrue.
  const setupPrerequisite = SETUP_PREREQUISITES[platform] ?? null;

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
    return { supported: true, reason: null, recommendation, platform, setupPrerequisite };
  }

  const isSecure = typeof window === 'undefined' || window.isSecureContext !== false;

  if (!isSecure) {
    return {
      supported: false,
      reason: 'insecure-context',
      recommendation: { ...recommendation, note: INSECURE_CONTEXT_NOTE },
      platform,
      setupPrerequisite,
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
      setupPrerequisite,
    };
  }

  return {
    supported: false,
    reason: 'unsupported-browser',
    recommendation,
    platform,
    setupPrerequisite,
  };
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
