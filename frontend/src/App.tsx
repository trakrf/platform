import { useState, useEffect, Suspense } from 'react';
import { useUIStore, useDeviceStore, useAuthStore } from '@/stores';
import type { TabType } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import TabNavigation from '@/components/TabNavigation';
import Header from '@/components/Header';
import { Toaster } from 'react-hot-toast';
import { LoadingScreen, InventoryLoadingScreen, LocateLoadingScreen, HelpLoadingScreen, SettingsLoadingScreen, BarcodeLoadingScreen } from '@/components/LoadingScreen';
import { initOpenReplay, trackPageView } from '@/lib/openreplay';
import { ErrorBoundary } from '@/components/ErrorBoundary';
import { lazyWithRetry } from '@/utils/lazyWithRetry';

const HomeScreen = lazyWithRetry(() => import('@/components/HomeScreen'));
const InventoryScreen = lazyWithRetry(() => import('@/components/InventoryScreen'));
const BarcodeScreen = lazyWithRetry(() => import('@/components/BarcodeScreen'));
const LocateScreen = lazyWithRetry(() => import('@/components/LocateScreen'));
const SettingsScreen = lazyWithRetry(() => import('@/components/SettingsScreen'));
const HelpScreen = lazyWithRetry(() => import('@/components/HelpScreen'));
const AssetsScreen = lazyWithRetry(() => import('@/components/AssetsScreen'));
const LocationsScreen = lazyWithRetry(() => import('@/components/LocationsScreen'));
const LoginScreen = lazyWithRetry(() => import('@/components/LoginScreen'));
const SignupScreen = lazyWithRetry(() => import('@/components/SignupScreen'));

const VALID_TABS: TabType[] = ['home', 'inventory', 'locate', 'barcode', 'assets', 'locations', 'settings', 'help', 'login', 'signup'];

export default function App() {
  const activeTab = useUIStore((state) => state.activeTab);
  const [isMobileMenuOpen, setIsMobileMenuOpen] = useState(false);

  useEffect(() => {
    initOpenReplay();
  }, []);

  useEffect(() => {
    // Initialize auth state from persisted storage
    useAuthStore.getState().initialize();
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
    const { tab, queryString, epc } = parseHash();

    if (epc) {
      const { useSettingsStore } = await import('@/stores/settingsStore');
      useSettingsStore.getState().setTargetEPC(epc);
    }

    const targetTab = tab && VALID_TABS.includes(tab as TabType)
      ? tab as TabType
      : isInitialLoad
        ? 'home'
        : null;

    if (targetTab) {
      useUIStore.getState().setActiveTab(targetTab);

      if (isInitialLoad && (!tab || tab !== targetTab)) {
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

  useEffect(() => {
    try {
      const hasBluetoothAPI = typeof navigator !== 'undefined' && navigator.bluetooth;
      const isMocked = typeof window !== 'undefined' && !!(window as unknown as { __webBluetoothMocked?: boolean }).__webBluetoothMocked;

      if (!hasBluetoothAPI && !isMocked && typeof window !== 'undefined') {
        console.warn('Web Bluetooth is not supported in this browser.');
        const bluetoothMessage = document.createElement('div');
        bluetoothMessage.className = 'p-4 bg-amber-100 border border-amber-300 rounded-md text-amber-800 mb-4';
        bluetoothMessage.innerHTML = 'Web Bluetooth is not supported in this browser. Please use Chrome, Edge, or Opera on desktop, or Chrome for Android.';

        const contentDiv = document.querySelector('.max-w-5xl');
        if (contentDiv && !contentDiv.querySelector('.bg-amber-100')) {
          contentDiv.insertBefore(bluetoothMessage, contentDiv.firstChild);
        }
      }
    } catch (error) {
      console.warn('Error checking Bluetooth support:', error);
    }

    const handleMockReady = () => {
      const warningBanner = document.querySelector('.bg-amber-100');
      if (warningBanner) {
        warningBanner.remove();
      }
    };

    window.addEventListener('webBluetoothMockReady', handleMockReady);

    return () => {
      window.removeEventListener('webBluetoothMockReady', handleMockReady);
    };
  }, []);

  const renderTabContent = () => {
    const tabComponents = {
      home: HomeScreen,
      inventory: InventoryScreen,
      locate: LocateScreen,
      barcode: BarcodeScreen,
      assets: AssetsScreen,
      locations: LocationsScreen,
      settings: SettingsScreen,
      help: HelpScreen,
      login: LoginScreen,
      signup: SignupScreen
    };

    const loadingScreens = {
      home: LoadingScreen,
      inventory: InventoryLoadingScreen,
      locate: LocateLoadingScreen,
      barcode: BarcodeLoadingScreen,
      assets: LoadingScreen,
      locations: LoadingScreen,
      settings: SettingsLoadingScreen,
      help: HelpLoadingScreen,
      login: LoadingScreen,
      signup: LoadingScreen
    };

    const Component = tabComponents[activeTab] || HomeScreen;
    const LoadingComponent = loadingScreens[activeTab] || LoadingScreen;

    return (
      <Suspense fallback={<LoadingComponent />}>
        <Component />
      </Suspense>
    );
  };
  if (!activeTab) {
    return <div style={{ padding: '20px', backgroundColor: 'yellow' }}>Loading - no active tab...</div>;
  }
  
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

      <div className="hidden xl:flex w-64 h-screen bg-white dark:bg-gray-800 border-r border-gray-200 dark:border-gray-700 flex-col fixed left-0 top-0 z-30">
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