import React, { useState } from 'react';
import { useDeviceStore, useUIStore, useAuthStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { ConfirmModal } from '@/components/ConfirmModal';
import { Battery, BatteryLow, BatteryMedium, BatteryFull, Plug, Unplug, Info } from 'lucide-react';
import toast from 'react-hot-toast';
import { UserMenu } from './UserMenu';

const TriggerIndicator = ({ isDown }: { isDown: boolean }) => {
  return (
    <div className="flex items-center mx-1">
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
  
  // ============= MOCK DATA FOR TESTING =============
  // To test the battery indicator appearance:
  // 1. Set MOCK_TESTING to true
  // 2. Change mockBatteryPercentage to test different colors:
  //    - 75 = Green (good battery)
  //    - 45 = Yellow (medium battery)
  //    - 25 = Red (low battery)
  // 3. Navigate to any screen except Home to see the indicator
  // 4. Set MOCK_TESTING to false when done testing
  const MOCK_TESTING = false; // Set to false to disable mock
  const mockBatteryPercentage = MOCK_TESTING ? 75 : batteryPercentage; // Try: 75 (green), 45 (yellow), 25 (red)
  const mockReaderState = MOCK_TESTING ? ReaderState.CONNECTED : readerState; // Simulate connected state
  // ==================================================
  
  
  
  const pageTitles = {
    home: { title: "RFID Dashboard", subtitle: "Choose your scanning mode to get started" },
    inventory: { title: "My Items", subtitle: "View and manage your scanned items" },
    locate: { title: "Find Item", subtitle: "Search for a specific item" },
    barcode: { title: "Barcode Scanner", subtitle: "Scan barcodes to identify items" },
    settings: { title: "Device Setup", subtitle: "Configure your RFID reader" },
    help: { title: "Help", subtitle: "Quick answers to get you started" }
  };
  
  const [isBrowserSupported, setIsBrowserSupported] = React.useState(true);

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
        // Show modal instead of direct disconnect
        setShowDisconnectModal(true);
      }
      // Remove cancel behavior - button is now disabled during CONFIGURING/BUSY states
    } catch (error) {
      // Error handling with toasts
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
  
  // Move browser support check to useEffect to prevent hydration mismatch
  React.useEffect(() => {
    // Only run this check on the client side
    // Check browser support using Web Bluetooth API directly
    const checkSupport = () => {
      const hasBluetoothAPI = typeof navigator !== 'undefined' && !!navigator.bluetooth;
      const isMocked = typeof window !== 'undefined' && !!window.__webBluetoothMocked;
      setIsBrowserSupported(hasBluetoothAPI || isMocked);
    };
    
    checkSupport();
    
    // Listen for mock ready event
    const handleMockReady = () => checkSupport();
    window.addEventListener('webBluetoothMockReady', handleMockReady);
    
    return () => {
      window.removeEventListener('webBluetoothMockReady', handleMockReady);
    };
  }, []);
  
  // Blinking effect for Connect Device button
  React.useEffect(() => {
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
      <header className="bg-white dark:bg-gray-800 border-b border-gray-200 dark:border-gray-700 px-4 md:px-6 lg:px-8 py-3 md:py-4">
        <div className="flex justify-between items-center">
          {/* Page Title with Info Button */}
          <div className="flex items-center gap-2">
            {/* Mobile/Tablet Menu Toggle Button - Show on screens up to 1280px */}
            {onMenuToggle && (
              <button
                onClick={onMenuToggle}
                className="xl:hidden p-2 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-lg transition-colors"
                aria-label="Toggle menu"
                data-testid="hamburger-button"
              >
                <div className="w-6 h-6 flex flex-col justify-center space-y-1">
                  <span className={`block h-0.5 w-6 bg-gray-600 dark:bg-gray-400 transform transition duration-300 ${isMobileMenuOpen ? 'rotate-45 translate-y-1.5' : ''}`}></span>
                  <span className={`block h-0.5 w-6 bg-gray-600 dark:bg-gray-400 transition duration-300 ${isMobileMenuOpen ? 'opacity-0' : ''}`}></span>
                  <span className={`block h-0.5 w-6 bg-gray-600 dark:bg-gray-400 transform transition duration-300 ${isMobileMenuOpen ? '-rotate-45 -translate-y-1.5' : ''}`}></span>
                </div>
              </button>
            )}
            <h1 className="text-lg md:text-2xl font-bold text-gray-900 dark:text-gray-100">{currentPage.title}</h1>
            <div className="relative">
              <button
                onClick={() => setShowInfoTooltip(!showInfoTooltip)}
                className="p-1 hover:bg-gray-100 dark:hover:bg-gray-700 rounded-full transition-colors"
                aria-label="Show page info"
              >
                <Info className="w-4 h-4 md:w-5 md:h-5 text-gray-500 dark:text-gray-400" />
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

          {/* Button Controls */}
          <div className="flex items-center gap-2 md:gap-3">
            {/* Auth UI - Always visible */}
            {isAuthenticated && user ? (
              <UserMenu user={user} onLogout={handleLogout} />
            ) : (
              <button
                onClick={() => useUIStore.getState().setActiveTab('login')}
                className="px-3 md:px-4 py-2 bg-blue-600 hover:bg-blue-700 text-white rounded-lg font-semibold text-sm md:text-base transition-colors"
              >
                Log In
              </button>
            )}

            {/* Connect Device Button - Conditionally shown */}
            {shouldShowConnectButton && (
              <>
            {(MOCK_TESTING || readerState !== ReaderState.DISCONNECTED) && (MOCK_TESTING ? mockBatteryPercentage : batteryPercentage) !== null && (
                <div
                  className={`flex items-center gap-1 md:gap-1.5 px-1.5 md:px-2`}
                  title={`Battery: ${MOCK_TESTING ? mockBatteryPercentage : batteryPercentage}%`}
                >
                  <div className="relative">
                    {(() => {
                      const percentage = MOCK_TESTING ? mockBatteryPercentage : batteryPercentage;
                      const BatteryIcon = getBatteryIcon(percentage);
                      const color = getBatteryColor(percentage);

                      return (
                        <>
                          <BatteryIcon className={`w-4 h-4 md:w-5 md:h-5 lg:w-6 lg:h-6 ${color}`} />

                          {percentage !== null && (
                            <div 
                              className="absolute inset-0 flex items-center"
                              style={{ paddingLeft: '2px', paddingRight: '5px' }}
                            >
                              <div 
                                className={`h-2 md:h-2.5 lg:h-3 rounded-sm transition-all duration-300 ${
                                  percentage >= 70 ? 'bg-green-500' :
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
                  
                  <span className={`text-xs md:text-sm font-semibold ${getBatteryColor(MOCK_TESTING ? mockBatteryPercentage : batteryPercentage)}`}>
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
                  flex items-center px-3 md:px-6 py-2 md:py-3 rounded-lg font-semibold text-white
                  transition-all duration-200 text-sm md:text-base
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
                  <Plug className="w-5 h-5 mr-1" />
                ) : (
                  <Unplug className="w-5 h-5 mr-1" />
                )}
                {MOCK_TESTING ? mockReaderState : readerState}
              </button>
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