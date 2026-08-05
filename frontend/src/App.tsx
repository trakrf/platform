import { useState, useEffect, Suspense } from 'react';
import { useUIStore, useDeviceStore, useAuthStore } from '@/stores';
import type { TabType } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import TabNavigation from '@/components/TabNavigation';
import Header from '@/components/Header';
import { Toaster } from 'react-hot-toast';
import { LoadingScreen, InventoryLoadingScreen, LocateLoadingScreen, HelpLoadingScreen, SettingsLoadingScreen } from '@/components/LoadingScreen';
import { initOpenReplay, trackPageView } from '@/lib/openreplay';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { lazyWithRetry } from '@/utils/lazyWithRetry';
import { EnvironmentBanner } from '@/components/EnvironmentBanner';
import { DEFAULT_TAB, resolveLegacyTab, isLegacyTab } from '@/utils/tabRedirects';
import { useCapabilityRouteGate } from '@/hooks/capability/useCapability';
import { capabilityEntryForRoute } from '@/components/capability/registry';
import CapabilityUpsell from '@/components/capability/CapabilityUpsell';
import { routeAuthGate } from '@/lib/routing/routePolicy';
import SignedOutUpsell from '@/components/auth/SignedOutUpsell';

const InventoryScreen = lazyWithRetry(() => import('@/components/InventoryScreen'));
const LocateScreen = lazyWithRetry(() => import('@/components/LocateScreen'));
const SettingsScreen = lazyWithRetry(() => import('@/components/SettingsScreen'));
const HelpScreen = lazyWithRetry(() => import('@/components/HelpScreen'));
const AssetsScreen = lazyWithRetry(() => import('@/components/AssetsScreen'));
const LocationsScreen = lazyWithRetry(() => import('@/components/LocationsScreen'));
const ScanDevicesScreen = lazyWithRetry(() => import('@/components/ScanDevicesScreen'));
const OutputDevicesScreen = lazyWithRetry(() => import('@/components/OutputDevicesScreen'));
const LiveReadsScreen = lazyWithRetry(() => import('@/components/LiveReadsScreen'));
const LoginScreen = lazyWithRetry(() => import('@/components/LoginScreen'));
const SignupScreen = lazyWithRetry(() => import('@/components/SignupScreen'));
const ForgotPasswordScreen = lazyWithRetry(() => import('@/components/ForgotPasswordScreen'));
const ResetPasswordScreen = lazyWithRetry(() => import('@/components/ResetPasswordScreen'));
const CreateOrgScreen = lazyWithRetry(() => import('@/components/CreateOrgScreen'));
const MembersScreen = lazyWithRetry(() => import('@/components/MembersScreen'));
const OrgSettingsScreen = lazyWithRetry(() => import('@/components/OrgSettingsScreen'));
const OrgGeofenceDefaultsScreen = lazyWithRetry(() => import('@/components/OrgGeofenceDefaultsScreen'));
const AcceptInviteScreen = lazyWithRetry(() => import('@/components/AcceptInviteScreen'));
const APIKeysScreen = lazyWithRetry(() => import('@/components/APIKeysScreen'));
const WebhooksScreen = lazyWithRetry(() => import('@/components/WebhooksScreen'));
const ReportsScreen = lazyWithRetry(() => import('@/components/ReportsScreen'));
const ReportsHistoryScreen = lazyWithRetry(() => import('@/components/ReportsHistoryScreen'));
const SuperadminOrgsScreen = lazyWithRetry(() => import('@/components/SuperadminOrgsScreen'));
const MusteringScreen = lazyWithRetry(() => import('@/components/mustering/MusteringScreen'));
const KitsScreen = lazyWithRetry(() => import('@/components/kits/KitsScreen'));
const ProfileScreen = lazyWithRetry(() => import('@/components/ProfileScreen'));

// Capability-gated tabs (mustering, output-devices, org-geofence-defaults) are
// valid hash targets here; whether they *resolve* is decided by the capability
// registry at render time, not by this list. Resolving them here would bounce a
// granted user's bookmark on every cold load, before the profile lands.
const VALID_TABS: TabType[] = ['scan', 'locate', 'kits', 'assets', 'locations', 'scan-devices', 'output-devices', 'live-reads', 'reports', 'reports-history', 'mustering', 'settings', 'help', 'login', 'signup', 'forgot-password', 'reset-password', 'create-org', 'org-members', 'org-settings', 'org-geofence-defaults', 'accept-invite', 'api-keys', 'webhooks', 'admin-orgs', 'profile'];

export default function App() {
  const activeTab = useUIStore((state) => state.activeTab);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  // TRA-1026 / ADR 0002: capability gate for the active route, evaluated at the
  // route definition rather than inside the screen, so an ungated org never
  // downloads the gated surface's chunk.
  const capabilityGate = useCapabilityRouteGate(activeTab);
  const isAuthenticated = useAuthStore((state) => state.isAuthenticated);
  // Persisted token rehydrates synchronously, before the mount effect below
  // calls initialize() to flip isAuthenticated. Its presence on first paint
  // is the "answer not known yet" signal the auth gate needs (TRA-1057).
  const hasPersistedToken = useAuthStore((state) => !!state.token);

  useEffect(() => {
    initOpenReplay();
  }, []);

  useEffect(() => {
    // Initialize auth state from persisted storage
    useAuthStore.getState().initialize();
    // If already authenticated, fetch profile to get org data
    if (useAuthStore.getState().isAuthenticated) {
      useAuthStore.getState().fetchProfile();
    }
  }, []);

  const parseHash = (hash: string = window.location.hash.slice(1)) => {
    const queryIndex = hash.indexOf('?');
    const tab = queryIndex !== -1 ? hash.slice(0, queryIndex) : hash;
    const queryString = queryIndex !== -1 ? hash.slice(queryIndex + 1) : '';
    const params = new URLSearchParams(queryString);

    return {
      tab,
      queryString,
      params,
      epc: params.get('epc') ? decodeURIComponent(params.get('epc')!) : null
    };
  };

  const buildHash = (tab: string, queryString: string = '') => {
    return `#${tab}${queryString ? '?' + queryString : ''}`;
  };

  const handleUrlNavigation = async (isInitialLoad = false) => {
    const { tab, queryString, params, epc } = parseHash();

    if (epc) {
      const { useSettingsStore } = await import('@/stores/settingsStore');
      useSettingsStore.getState().setTargetEPC(epc);
    }

    // Kit verify → Locate handoff (TRA-1033): a `return=kits` param arms the
    // "back to kit results" button; any locate navigation without it disarms.
    useUIStore.getState().setLocateReturnTab(params.get('return') === 'kits' ? 'kits' : null);

    // Resolve retired ids (#home/#inventory/#barcode) to their successor before validating.
    const resolvedTab = resolveLegacyTab(tab);

    const targetTab = resolvedTab && VALID_TABS.includes(resolvedTab as TabType)
      ? resolvedTab as TabType
      : isInitialLoad
        ? DEFAULT_TAB
        : null;

    if (targetTab) {
      useUIStore.getState().setActiveTab(targetTab);

      // Rewrite the URL on initial load (when it changed) OR whenever we followed
      // a legacy redirect, so the address bar reflects the canonical #scan.
      if ((isInitialLoad && (!tab || tab !== targetTab)) || isLegacyTab(tab)) {
        const newHash = buildHash(targetTab, queryString);
        window.history.replaceState({ tab: targetTab }, '', newHash);
      }

      trackPageView(targetTab);
    }
  };

  useEffect(() => {
    handleUrlNavigation(true);
  }, []);

  useEffect(() => {
    const unsubActiveTab = useUIStore.subscribe(
      (state) => {
        const { tab: currentTab, queryString } = parseHash();

        if (currentTab !== state.activeTab) {
          const newHash = buildHash(state.activeTab, queryString);
          window.history.replaceState({ tab: state.activeTab }, '', newHash);
        }

        trackPageView(state.activeTab);

        setIsMobileMenuOpen(false);
      }
    );

    return () => {
      unsubActiveTab();
    };
  }, []);

  // An ungated `absent` capability leaves no trace, so its route resolves the
  // way any unknown hash does: fall back to the default tab and rewrite the URL
  // so the unreachable one doesn't linger in the address bar. Runs only once the
  // capability set is known, so a granted user's bookmark survives a cold load.
  useEffect(() => {
    if (capabilityGate !== 'not-found') return;
    useUIStore.getState().setActiveTab(DEFAULT_TAB);
    window.history.replaceState({ tab: DEFAULT_TAB }, '', `#${DEFAULT_TAB}`);
  }, [capabilityGate]);

  useEffect(() => {
    const handleHashChange = () => handleUrlNavigation(false);

    window.addEventListener('hashchange', handleHashChange);
    window.addEventListener('popstate', handleHashChange);

    return () => {
      window.removeEventListener('hashchange', handleHashChange);
      window.removeEventListener('popstate', handleHashChange);
    };
  }, []);

  useEffect(() => {
    return () => {
      const cleanup = async () => {
        const readerState = useDeviceStore.getState().readerState;
        if (readerState === ReaderState.DISCONNECTED) return;

        const { disconnect } = useDeviceStore.getState();
        setTimeout(async () => {
          try {
            await disconnect();
          } catch (e) {
            console.error('Error disconnecting device during page unmount:', e);
          }
        }, 500);
      };

      cleanup();
    };
  }, []);

  const renderTabContent = () => {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    const tabComponents: Record<string, React.ComponentType<any>> = {
      scan: InventoryScreen,
      locate: LocateScreen,
      kits: KitsScreen,
      assets: AssetsScreen,
      locations: LocationsScreen,
      'scan-devices': ScanDevicesScreen,
      'output-devices': OutputDevicesScreen,
      'live-reads': LiveReadsScreen,
      settings: SettingsScreen,
      help: HelpScreen,
      login: LoginScreen,
      signup: SignupScreen,
      'forgot-password': ForgotPasswordScreen,
      'reset-password': ResetPasswordScreen,
      'create-org': CreateOrgScreen,
      'org-members': MembersScreen,
      'org-settings': OrgSettingsScreen,
      'org-geofence-defaults': OrgGeofenceDefaultsScreen,
      'accept-invite': AcceptInviteScreen,
      reports: ReportsScreen,
      'reports-history': ReportsHistoryScreen,
      mustering: MusteringScreen,
      'api-keys': APIKeysScreen,
      'webhooks': WebhooksScreen,
      'admin-orgs': SuperadminOrgsScreen,
      profile: ProfileScreen,
    };

    const loadingScreens: Record<string, React.ComponentType> = {
      scan: InventoryLoadingScreen,
      locate: LocateLoadingScreen,
      kits: LoadingScreen,
      assets: LoadingScreen,
      locations: LoadingScreen,
      'scan-devices': LoadingScreen,
      'output-devices': LoadingScreen,
      'live-reads': LoadingScreen,
      settings: SettingsLoadingScreen,
      help: HelpLoadingScreen,
      login: LoadingScreen,
      signup: LoadingScreen,
      'forgot-password': LoadingScreen,
      'reset-password': LoadingScreen,
      'create-org': LoadingScreen,
      'org-members': LoadingScreen,
      'org-settings': LoadingScreen,
      'org-geofence-defaults': LoadingScreen,
      'accept-invite': LoadingScreen,
      reports: LoadingScreen,
      'reports-history': LoadingScreen,
      mustering: LoadingScreen,
      'api-keys': LoadingScreen,
      'webhooks': LoadingScreen,
      'admin-orgs': LoadingScreen,
      profile: LoadingScreen,
    };

    const Component = tabComponents[activeTab] || InventoryScreen;
    const LoadingComponent = loadingScreens[activeTab] || LoadingScreen;

    // Auth gate (TRA-1057), first of the four gates: auth → entitlement →
    // capability → role. Returning here — before the lazy component is
    // referenced — keeps a signed-out visitor from downloading a surface chunk
    // they cannot use, and keeps the capability gate from being asked a
    // question that has no meaning without an org.
    const authGate = routeAuthGate(activeTab, isAuthenticated, hasPersistedToken);
    if (authGate === 'pending') {
      // A persisted token survived rehydration but initialize() (mount
      // effect) hasn't resolved it yet — same "not yet knowable" treatment
      // the capability gate gives its own `loading` state below. Rendering
      // the verdict here would flash the signed-out card at an actually
      // signed-in visitor who just reloaded the page.
      return <LoadingComponent />;
    }
    if (authGate === 'signed-out') {
      return <SignedOutUpsell route={activeTab} />;
    }

    // Capability gate (TRA-1026). Every non-`allow` branch returns before the
    // lazy component is referenced in the tree, which is what keeps a gated
    // org from ever requesting the surface's chunk.
    if (capabilityGate === 'loading' || capabilityGate === 'not-found') {
      // Set not yet known, or the redirect effect above is about to fire.
      return <LoadingComponent />;
    }

    const capabilityEntry = capabilityEntryForRoute(activeTab);
    if (capabilityGate === 'upsell' && capabilityEntry) {
      return (
        <CapabilityUpsell
          capability={capabilityEntry.capability}
          label={capabilityEntry.label}
        />
      );
    }

    // Get token from URL for reset-password screen
    const { params } = parseHash();
    const token = params.get('token');

    return (
      <Suspense fallback={<LoadingComponent />}>
        {activeTab === 'reset-password' || activeTab === 'accept-invite' ? (
          <Component token={token} />
        ) : (
          <Component />
        )}
      </Suspense>
    );
  };

  return (
    <div className="min-h-screen bg-gray-50 dark:bg-gray-900 flex relative">
      <Toaster 
        position="top-center"
        toastOptions={{
          duration: 4000,
          style: {
            background: '#1f2937',
            color: '#ffffff',
            borderRadius: '8px',
            border: '1px solid #374151',
          },
        }}
      />

      <div className="hidden xl:flex w-64 h-screen bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 flex-col fixed left-0 top-0 z-30" data-testid="desktop-sidebar">
        <ErrorBoundary name="TabNavigation">
          <TabNavigation />
        </ErrorBoundary>
      </div>

      {isMobileMenuOpen && (
        <>
          <div
            className="xl:hidden fixed inset-0 bg-black bg-opacity-50 z-40"
            onClick={() => setIsMobileMenuOpen(false)}
            data-testid="mobile-menu-overlay"
          />
          <div className="xl:hidden fixed left-0 top-0 h-full w-64 bg-white dark:bg-gray-800 z-50 shadow-lg" data-testid="hamburger-dropdown">
            <ErrorBoundary name="TabNavigation Mobile">
              <TabNavigation />
            </ErrorBoundary>
          </div>
        </>
      )}

      <div className="flex-1 flex flex-col xl:ml-64">
        <EnvironmentBanner />
        <ErrorBoundary name="Header">
          <Header onMenuToggle={() => setIsMobileMenuOpen(!isMobileMenuOpen)} isMobileMenuOpen={isMobileMenuOpen} />
        </ErrorBoundary>

        <div className="flex-1 p-2 md:p-8 bg-gray-50 dark:bg-gray-900">
          <ErrorBoundary name="Tab Content">
            {renderTabContent()}
          </ErrorBoundary>
        </div>
      </div>
    </div>
  );
}