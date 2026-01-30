import { useState, useEffect } from 'react';
import { useDeviceStore, useUIStore, useAuthStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { ConfirmModal } from '@/components/ConfirmModal';
import { Battery, BatteryLow, BatteryMedium, BatteryFull, Plug, Unplug, Info } from 'lucide-react';
import toast from 'react-hot-toast';
import { OrgSwitcher } from './OrgSwitcher';

const TriggerIndicator = ({ isDown }: { isDown: boolean }) => {
  return (
    <div className="flex items-center">
      <div className="w-5 h-5 relative">
        <div className={`absolute inset-0 rounded-full ${isDown ? 'bg-green-500' : 'bg-gray-400'}`}></div>

        <div
          className={`absolute left-0 top-0 bottom-0 ${isDown ? 'bg-red-500' : 'bg-white'}`}
          style={{
            width: '50%',
            height: '100%',
            borderTopLeftRadius: '9999px',
            borderBottomLeftRadius: '9999px'
          }}
        ></div>

        <div
          className={`absolute rounded-full ${isDown ? 'bg-green-500' : 'bg-gray-400'}`}
          style={{
            width: '40%',
            height: '100%',
            left: isDown ? '35%' : '45%',
            top: '0'
          }}
        ></div>
      </div>
    </div>
  );
};

const getBatteryColor = (percentage: number | null): string => {
  if (percentage === null) return 'text-gray-400';
  if (percentage >= 70) return 'text-green-500';
  if (percentage >= 30) return 'text-yellow-500';
  return 'text-red-500';
};

const getBatteryIcon = (percentage: number | null) => {
  if (percentage === null) return Battery;
  if (percentage >= 70) return BatteryFull;
  if (percentage >= 30) return BatteryMedium;
  return BatteryLow;
};

interface HeaderProps {
  onMenuToggle?: () => void;
  isMobileMenuOpen?: boolean;
}

export default function Header({ onMenuToggle, isMobileMenuOpen = false }: HeaderProps = {}) {
  const readerState = useDeviceStore((state) => state.readerState);
  const batteryPercentage = useDeviceStore((state) => state.batteryPercentage);
  const triggerState = useDeviceStore((state) => state.triggerState);
  const connect = useDeviceStore((state) => state.connect);
  const disconnect = useDeviceStore((state) => state.disconnect);
  const activeTab = useUIStore((state) => state.activeTab);
  const { isAuthenticated, user } = useAuthStore();

  const MOCK_TESTING = false;
  const mockBatteryPercentage = MOCK_TESTING ? 75 : batteryPercentage;
  const mockReaderState = MOCK_TESTING ? ReaderState.CONNECTED : readerState;

  const pageTitles = {
    home: { title: "Dashboard", subtitle: "Choose your scanning mode to get started" },
    inventory: { title: "Inventory", subtitle: "View and manage your scanned items" },
    locate: { title: "Locate", subtitle: "Search for a specific item" },
    barcode: { title: "Barcode Scanner", subtitle: "Scan barcodes to identify items" },
    settings: { title: "Device Setup", subtitle: "Configure your RFID reader" },
    help: { title: "Help", subtitle: "Quick answers to get you started" },
    assets: { title: "Assets", subtitle: "Manage your organization's assets" },
    locations: { title: "Locations", subtitle: "Manage your organization's locations" },
    reports: { title: "Reports", subtitle: "View asset locations and movement history" }
  };

  const [isBrowserSupported, setIsBrowserSupported] = useState(true);
  const [isDebounced, setIsDebounced] = useState(false);
  const [showDisconnectModal, setShowDisconnectModal] = useState(false);
  const [isBlinking, setIsBlinking] = useState(false);
  const [showInfoTooltip, setShowInfoTooltip] = useState(false);

  const handleLogout = () => {
    useAuthStore.getState().logout();
    useUIStore.getState().setActiveTab('home');
  };

  const handleConnectClick = async () => {
    if (isDebounced || !isBrowserSupported) return;

    setIsDebounced(true);
    setTimeout(() => setIsDebounced(false), 500);

    try {
      if (readerState === ReaderState.DISCONNECTED) {
        await connect();
      } else if (readerState === ReaderState.CONNECTED) {
        setShowDisconnectModal(true);
      }
    } catch (error) {
      const errorMessage = error instanceof Error ? error.message : '';
      if (errorMessage.includes('timeout')) {
        toast.error('Connection timed out. Please try again.');
      } else if (errorMessage.includes('disconnected')) {
        toast.error('Reader disconnected unexpectedly');
      } else {
        toast.error('Failed to connect to reader');
      }
      console.error('Connection error:', error);
    }
  };

  // Handle disconnect confirmation
  const handleDisconnectConfirm = async () => {
    setShowDisconnectModal(false);
    try {
      await disconnect();
    } catch (error) {
      toast.error('Failed to disconnect reader');
      console.error('Disconnect error:', error);
    }
  };

  useEffect(() => {
    const checkSupport = () => {
      const hasBluetoothAPI = typeof navigator !== 'undefined' && !!navigator.bluetooth;
      const isMocked = typeof window !== 'undefined' && !!window.__webBluetoothBridged;
      setIsBrowserSupported(hasBluetoothAPI || isMocked);
    };

    checkSupport();

    const handleMockReady = () => checkSupport();
    window.addEventListener('webBluetoothMockReady', handleMockReady);

    return () => {
      window.removeEventListener('webBluetoothMockReady', handleMockReady);
    };
  }, []);

  useEffect(() => {
    if (readerState === ReaderState.DISCONNECTED && isBrowserSupported) {
      const interval = setInterval(() => {
        setIsBlinking(prev => !prev);
      }, 1000);
      return () => clearInterval(interval);
    } else {
      setIsBlinking(false);
    }
  }, [readerState, isBrowserSupported]);

  const currentPage = pageTitles[activeTab as keyof typeof pageTitles] || pageTitles.inventory;

  const shouldShowConnectButton = activeTab !== 'home' && activeTab !== 'help';

  return (
    <>
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-2 md:px-2 lg:px-2 py-1 md:py-2 sticky top-0 z-50">
        <div className="flex justify-between items-center">
          {/* Breadcrumb: Org / Page Title */}
          <div className="flex items-center gap-2">
            {/* Mobile/Tablet Menu Toggle Button - Show on screens up to 1280px */}
            {onMenuToggle && (
              <button
                onClick={onMenuToggle}
                className="xl:hidden p-1.5 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
                aria-label="Toggle menu"
                data-testid="hamburger-button"
              >
                <div className="w-5 h-5 flex flex-col justify-center space-y-1">
                  <span className={`block h-0.5 w-5 bg-gray-600 dark:bg-gray-400 transform transition duration-300 ${isMobileMenuOpen ? 'rotate-45 translate-y-1.5' : ''}`}></span>
                  <span className={`block h-0.5 w-5 bg-gray-600 dark:bg-gray-400 transition duration-300 ${isMobileMenuOpen ? 'opacity-0' : ''}`}></span>
                  <span className={`block h-0.5 w-5 bg-gray-600 dark:bg-gray-400 transform transition duration-300 ${isMobileMenuOpen ? '-rotate-45 -translate-y-1.5' : ''}`}></span>
                </div>
              </button>
            )}

            <div className="flex items-center gap-1.5">
              <h1 className="text-base md:text-xl font-semibold text-gray-900 dark:text-gray-100">{currentPage.title}</h1>
              <div className="relative flex items-center">
                <button
                  onClick={() => setShowInfoTooltip(!showInfoTooltip)}
                  className="hover:bg-gray-100 dark:hover:bg-gray-700 rounded-full transition-colors p-0.5"
                  aria-label="Show page info"
                >
                  <Info className="w-4 h-4 text-gray-400 dark:text-gray-500 block translate-y-px" />
                </button>
                {showInfoTooltip && (
                  <>
                    <div
                      className="fixed inset-0 z-10"
                      onClick={() => setShowInfoTooltip(false)}
                    />
                    <div className="absolute left-0 top-full mt-2 z-20 bg-gray-900 dark:bg-gray-700 text-white text-sm rounded-lg px-3 py-2 shadow-lg whitespace-nowrap">
                      {currentPage.subtitle}
                      <div className="absolute left-4 -top-1 w-2 h-2 bg-gray-900 dark:bg-gray-700 transform rotate-45"></div>
                    </div>
                  </>
                )}
              </div>
            </div>
          </div>

          {/* Button Controls */}
          <div className="flex items-center gap-2">
            <button
              onClick={handleConnectClick}
              data-testid={(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.DISCONNECTED ? 'connect-button' : 'disconnect-button'}
              disabled={
                !MOCK_TESTING && (
                  isDebounced ||
                  readerState === ReaderState.CONNECTING ||
                  (readerState === ReaderState.DISCONNECTED && !isBrowserSupported)
                )
              }
              className={`
                  flex items-center gap-1.5 px-2.5 py-1.5 rounded-md font-medium text-white
                  transition-all duration-200 text-sm
                  ${(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.DISCONNECTED ?
                  (isBlinking ? 'bg-blue-700' : 'bg-blue-600') + ' hover:bg-blue-700' : ''}
                  ${((MOCK_TESTING ? mockReaderState : readerState) === ReaderState.CONNECTING ||
                  (MOCK_TESTING ? mockReaderState : readerState) === ReaderState.CONFIGURING ||
                  (MOCK_TESTING ? mockReaderState : readerState) === ReaderState.BUSY) ?
                  'bg-yellow-600 hover:bg-yellow-700' : ''}
                  ${(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.SCANNING ?
                  'bg-purple-600 hover:bg-purple-700' : ''}
                  ${(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.CONNECTED ?
                  'bg-green-600 hover:bg-green-700' : ''}
                  ${(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.ERROR ?
                  'bg-red-600 hover:bg-red-700' : ''}
                  ${!isBrowserSupported ? 'opacity-50 cursor-not-allowed' : ''}
                `}
              aria-label={(MOCK_TESTING ? mockReaderState : readerState)}
            >
              {(MOCK_TESTING ? mockReaderState : readerState) === ReaderState.DISCONNECTED ? (
                <Plug className="w-4 h-4" />
              ) : (
                <Unplug className="w-4 h-4" />
              )}
              <span className="hidden sm:inline">{MOCK_TESTING ? mockReaderState : readerState}</span>
            </button>

            {/* Auth UI - Always visible */}
            {isAuthenticated && user ? (
              <OrgSwitcher user={user} onLogout={handleLogout} />
            ) : (
              <button
                onClick={() => useUIStore.getState().setActiveTab('login')}
                className="px-3 py-1.5 bg-blue-600 hover:bg-blue-700 text-white rounded-md font-medium text-sm transition-colors"
              >
                Log In
              </button>
            )}

            {/* Battery & Trigger - Conditionally shown */}
            {shouldShowConnectButton && (
              <>
                {(MOCK_TESTING || readerState !== ReaderState.DISCONNECTED) && (MOCK_TESTING ? mockBatteryPercentage : batteryPercentage) !== null && (
                  <div
                    className="flex items-center gap-1 px-1.5"
                    title={`Battery: ${MOCK_TESTING ? mockBatteryPercentage : batteryPercentage}%`}
                  >
                    <div className="relative">
                      {(() => {
                        const percentage = MOCK_TESTING ? mockBatteryPercentage : batteryPercentage;
                        const BatteryIcon = getBatteryIcon(percentage);
                        const color = getBatteryColor(percentage);

                        return (
                          <>
                            <BatteryIcon className={`w-5 h-5 ${color}`} />

                            {percentage !== null && (
                              <div
                                className="absolute inset-0 flex items-center"
                                style={{ paddingLeft: '2px', paddingRight: '5px' }}
                              >
                                <div
                                  className={`h-2 rounded-sm transition-all duration-300 ${percentage >= 70 ? 'bg-green-500' :
                                      percentage >= 30 ? 'bg-yellow-500' :
                                        'bg-red-500'
                                    }`}
                                  style={{
                                    width: `${Math.max(0, Math.min(100, percentage))}%`,
                                    opacity: 0.7
                                  }}
                                />
                              </div>
                            )}
                          </>
                        );
                      })()}
                    </div>

                    <span className={`text-xs font-semibold ${getBatteryColor(MOCK_TESTING ? mockBatteryPercentage : batteryPercentage)}`}>
                      {MOCK_TESTING ? mockBatteryPercentage : batteryPercentage}%
                    </span>
                  </div>
                )}

                {(MOCK_TESTING || readerState !== ReaderState.DISCONNECTED) && (
                  <div
                    className="flex items-center"
                    title={`Trigger ${triggerState ? 'Pressed' : 'Released'}`}
                  >
                    <TriggerIndicator isDown={triggerState} />
                  </div>
                )}
              </>
            )}
          </div>
        </div>
      </header>

      <ConfirmModal
        isOpen={showDisconnectModal}
        onConfirm={handleDisconnectConfirm}
        onCancel={() => setShowDisconnectModal(false)}
        title="Disconnect Reader"
        message="Are you sure you want to disconnect the reader?"
      />
    </>
  );
}