import React, { useState, useEffect } from 'react';
import { useDeviceStore, useSettingsStore, useTagStore, useUIStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { Bluetooth, Zap, Settings2, Info, RefreshCw, ChevronDown, ChevronUp, Smartphone, WifiOff, Battery, Bug } from 'lucide-react';
import { ConnectIcon } from '@/components/icons/ConnectIcon';
import toast from 'react-hot-toast';
import { version } from '../../package.json';

export default function SettingsScreen() {
  // Set active tab when component mounts - standard React pattern
  React.useEffect(() => {
    useUIStore.getState().setActiveTab('settings');
  }, []);

  // Local state to track Zustand store values
  const [readerState, setLocalReaderState] = useState(useDeviceStore.getState().readerState);
  const [batteryPercentage, setBatteryPercentage] = useState(useDeviceStore.getState().batteryPercentage);
  const [deviceName, setDeviceName] = useState(useDeviceStore.getState().deviceName);
  
  const [rfPower, setLocalRfPower] = useState(useSettingsStore.getState().rfid?.transmitPower ?? 30);
  const [isAdvancedExpanded, setIsAdvancedExpanded] = useState(false);
  const [isBrowserSupported, setIsBrowserSupported] = useState(true);
  const [isDebounced, setIsDebounced] = useState(false);
  const [isBlinking, setIsBlinking] = useState(false);
  const [selectedSession, setSelectedSession] = useState('S1');
  const [isDebugPanelVisible, setIsDebugPanelVisible] = useState(false);
  const [debugData, setDebugData] = useState<Record<string, unknown> | null>(null);

  // New settings state
  const [workerLogLevel, setLocalWorkerLogLevel] = useState(useSettingsStore.getState().system?.workerLogLevel || 'info');
  const [batteryCheckInterval, setLocalBatteryCheckInterval] = useState(useSettingsStore.getState().system?.batteryCheckInterval || 60);

  // Get setter functions from stores
  const { setTransmitPower, setWorkerLogLevel, setBatteryCheckInterval } = useSettingsStore.getState();
  const { connect, disconnect } = useDeviceStore.getState();
  // Removed inventoryRunning - using readerState === ReaderState.SCANNING instead
  
  // Subscribe to store changes
  useEffect(() => {
    // Subscribe to device store changes
    const unsubDeviceStore = useDeviceStore.subscribe((state) => {
      setLocalReaderState(state.readerState);
      setBatteryPercentage(state.batteryPercentage);
      setDeviceName(state.deviceName);
    });
    
    // Subscribe to settings store changes
    const unsubSettingsStore = useSettingsStore.subscribe((state) => {
      setLocalRfPower(state.rfid?.transmitPower ?? 30);
      setLocalWorkerLogLevel(state.system?.workerLogLevel || 'info');
      setLocalBatteryCheckInterval(state.system?.batteryCheckInterval || 60);
    });
    
    // Cleanup subscriptions
    return () => {
      unsubDeviceStore();
      unsubSettingsStore();
    };
  }, []);
  
  // Check browser support
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
  
  // Blinking effect for Connect Device button
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
  
  const handlePowerChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const value = parseFloat(e.target.value);
    setTransmitPower(value);
    setLocalRfPower(value);
  };
  
  const handleConnectClick = async () => {
    if (isDebounced || !isBrowserSupported) return;
    
    setIsDebounced(true);
    setTimeout(() => setIsDebounced(false), 500);
    
    try {
      if (readerState === ReaderState.DISCONNECTED) {
        await connect();
      } else if (readerState === ReaderState.CONNECTED) {
        await disconnect();
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
  
  const getBatteryPercentage = () => {
    return batteryPercentage;
  };
  
  // Get power level label and color
  const getPowerLevelInfo = () => {
    if (rfPower <= 15) return { label: 'Low', color: 'text-blue-600' };
    if (rfPower <= 22) return { label: 'Medium', color: 'text-blue-600' };
    return { label: 'High', color: 'text-blue-600' };
  };
  
  const powerInfo = getPowerLevelInfo();
  
  return (
    <div className="max-w-7xl mx-auto space-y-2 md:space-y-6">
      {/* Device Connection Section */}
      <div className={`border rounded-lg p-6 ${
        readerState === ReaderState.DISCONNECTED ? 'bg-red-50 dark:bg-red-900/20 border-red-200 dark:border-red-800' : 'bg-green-50 dark:bg-green-900/20 border-green-200 dark:border-green-800'
      }`}>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center">
            {readerState === ReaderState.DISCONNECTED ? (
              <WifiOff className="w-5 h-5 text-red-600 mr-2" />
            ) : (
              <Bluetooth className="w-5 h-5 text-green-600 mr-2" />
            )}
            <h2 className={`text-lg font-semibold ${
              readerState === ReaderState.DISCONNECTED ? 'text-red-800 dark:text-red-200' : 'text-green-800 dark:text-green-200'
            }`}>Device Connection</h2>
          </div>
          <div className="flex items-center space-x-3">
            {readerState !== ReaderState.DISCONNECTED && getBatteryPercentage() !== null && (
              <div className="flex items-center text-gray-600">
                <Battery className="w-4 h-4 mr-1" />
                <span className="text-sm font-medium text-gray-600 dark:text-gray-400">{getBatteryPercentage()}%</span>
              </div>
            )}
            <span className={`px-3 py-1 text-sm font-medium rounded-full ${
              readerState === ReaderState.DISCONNECTED ? 'bg-red-100 dark:bg-red-800 text-red-800 dark:text-red-100' : 'bg-green-100 dark:bg-green-800 text-green-800 dark:text-green-100'
            }`}>
              {readerState === ReaderState.DISCONNECTED ? 'Disconnected' : 'Connected'}
            </span>
          </div>
        </div>
        
        <div className="mb-4">
          <div className="flex items-center mb-2">
            <Smartphone className="w-5 h-5 text-gray-600 mr-2" />
            <h3 className="font-semibold text-gray-900 dark:text-gray-100">
              {deviceName || 'TrakRF Handheld Reader'}
            </h3>
          </div>
          <p className={`text-sm ${
            readerState === ReaderState.DISCONNECTED ? 'text-red-700 dark:text-red-300' : 'text-green-700 dark:text-green-300'
          }`}>
            {readerState === ReaderState.DISCONNECTED ?
              'Connect your device to start scanning' : 
              'Device is connected and ready to scan'
            }
          </p>
        </div>
        
        <button
          onClick={handleConnectClick}
          disabled={
            isDebounced || 
            readerState === ReaderState.CONNECTING ||
            readerState === ReaderState.SCANNING ||
            (readerState === ReaderState.DISCONNECTED && !isBrowserSupported)
          }
          className={`w-full px-4 py-3 text-white rounded-lg font-medium flex items-center justify-center transition-all duration-200 ${
            readerState === ReaderState.DISCONNECTED ? 
              (isBlinking ? 'bg-blue-700' : 'bg-blue-600') + ' hover:bg-blue-700' : 
              'bg-green-600 hover:bg-green-700'
          } ${
            (!isBrowserSupported || readerState === ReaderState.SCANNING) ? 'opacity-50 cursor-not-allowed' : ''
          }`}
        >
          <ConnectIcon className="w-6 h-6 mr-2" />
          {readerState === ReaderState.DISCONNECTED ? 'Connect Device' :
           readerState === ReaderState.CONNECTING ? 'Connecting...' :
           'Disconnect'
          }
        </button>
      </div>

      {/* Basic Settings Section */}
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg p-6">
        <div className="flex items-center mb-6">
          <Zap className="w-5 h-5 text-blue-600 mr-2" />
          <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Basic Settings</h2>
        </div>
        
        {/* Signal Power */}
        <div className="mb-8">
          <div className="flex items-center justify-between mb-4">
            <label className="text-sm font-medium text-gray-700 dark:text-gray-300 flex items-center">
              Signal Power
              <Info className="w-4 h-4 text-gray-400 ml-2" />
            </label>
            <div className="flex items-center">
              <span className={`text-sm font-medium ${powerInfo.color} mr-3`}>
                {powerInfo.label}
              </span>
              <span className="text-sm font-bold text-gray-900 dark:text-gray-100">
                {rfPower.toFixed(0)} dBm
              </span>
            </div>
          </div>
          
          <div className="relative">
            <input 
              type="range" 
              min="10" 
              max="30" 
              step="1" 
              value={rfPower}
              onChange={handlePowerChange}
              className="w-full h-2 bg-gray-200 dark:bg-gray-600 rounded-lg appearance-none cursor-pointer slider"
              style={{
                background: `linear-gradient(to right, #3b82f6 0%, #3b82f6 ${((rfPower - 10) / 20) * 100}%, ${isBrowserSupported ? '#4b5563' : '#e5e7eb'} ${((rfPower - 10) / 20) * 100}%, ${isBrowserSupported ? '#4b5563' : '#e5e7eb'} 100%)`
              }}
            />
            <div className="flex justify-between text-xs text-gray-500 dark:text-gray-400 mt-2">
              <span>Low</span>
              <span>Medium</span>
              <span>High</span>
            </div>
          </div>
        </div>
        
      </div>

      {/* Advanced Settings Section */}
      <div className="bg-white dark:bg-gray-800 border border-gray-200 dark:border-gray-700 rounded-lg">
        <button
          onClick={() => setIsAdvancedExpanded(!isAdvancedExpanded)}
          className="w-full px-6 py-4 flex items-center justify-between hover:bg-gray-50 dark:hover:bg-gray-700 transition-colors"
        >
          <div className="flex items-center">
            <Settings2 className="w-5 h-5 text-gray-600 mr-2" />
            <h2 className="text-lg font-semibold text-gray-900 dark:text-gray-100">Advanced Settings</h2>
          </div>
          {isAdvancedExpanded ? 
            <ChevronUp className="w-5 h-5 text-gray-600 dark:text-gray-400" /> : 
            <ChevronDown className="w-5 h-5 text-gray-600 dark:text-gray-400" />
          }
        </button>
        
        {isAdvancedExpanded && (
          <div className="border-t border-gray-200 dark:border-gray-700 p-6 space-y-6">
            {/* Session Selection */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">Session</h3>
              </div>
              <select
                value={selectedSession}
                onChange={(e) => setSelectedSession(e.target.value)}
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              >
                <option value="S0">S0 - No persistence</option>
                <option value="S1">S1 - Short memory</option>
                <option value="S2">S2 - Medium persistence</option>
                <option value="S3">S3 - High persistence</option>
              </select>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                Session determines how tags remember being read. S0 has no persistence, while S1-S3 have increasing levels of persistence.
              </p>
            </div>

            {/* Worker Log Level */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">Worker Log Level</h3>
              </div>
              <select
                value={workerLogLevel}
                onChange={(e) => {
                  const newLevel = e.target.value as 'error' | 'warn' | 'info' | 'debug';
                  setLocalWorkerLogLevel(newLevel);
                  setWorkerLogLevel(newLevel);
                }}
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              >
                <option value="error">Error</option>
                <option value="warn">Warn</option>
                <option value="info">Info</option>
                <option value="debug">Debug</option>
              </select>
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                Controls the verbosity of worker thread logging. Debug level provides the most detailed information.
              </p>
            </div>

            {/* Battery Check Interval */}
            <div>
              <div className="flex items-center justify-between mb-3">
                <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">Battery Check Interval (seconds)</h3>
              </div>
              <input
                type="number"
                min="0"
                max="300"
                step="10"
                value={batteryCheckInterval}
                onChange={(e) => {
                  const newInterval = parseInt(e.target.value, 10);
                  if (!isNaN(newInterval) && newInterval >= 0 && newInterval <= 300) {
                    setLocalBatteryCheckInterval(newInterval);
                    setBatteryCheckInterval(newInterval);
                  }
                }}
                className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
              />
              <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                How often to check battery level when idle. Set to 0 to disable. Frequency doubles when battery is below 20%.
              </p>
            </div>

            {/* RF Power Guidelines */}
            <div className="bg-blue-50 dark:bg-blue-900/20 border border-blue-200 dark:border-blue-800 rounded-lg p-4">
              <div className="flex items-center mb-3">
                <Info className="w-4 h-4 text-blue-600 dark:text-blue-400 mr-2" />
                <h3 className="font-medium text-blue-900 dark:text-blue-100">RF Power Guidelines</h3>
              </div>
              
              <div className="space-y-2 text-sm">
                <div className="flex justify-between">
                  <span className="text-blue-700 dark:text-blue-300">10-15 dBm (Low):</span>
                  <span className="text-blue-600 dark:text-blue-400">1-2 meters range, best battery life</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-blue-700 dark:text-blue-300">16-22 dBm (Medium):</span>
                  <span className="text-blue-600 dark:text-blue-400">3-5 meters range, balanced performance</span>
                </div>
                <div className="flex justify-between">
                  <span className="text-blue-700 dark:text-blue-300">23-30 dBm (High):</span>
                  <span className="text-blue-600 dark:text-blue-400">6+ meters range, higher battery usage</span>
                </div>
              </div>
            </div>
            
            {/* Device Information */}
            <div>
              <div className="flex items-center mb-4">
                <Info className="w-4 h-4 text-gray-500 dark:text-gray-400 mr-2" />
                <h3 className="font-medium text-gray-900 dark:text-gray-100">Device Information</h3>
              </div>
              
              <div className="grid grid-cols-2 gap-6">
                <div>
                  <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">App Version</h4>
                  <p className="text-sm text-gray-600 dark:text-gray-400">TrakRF Web v{version}</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Compatible Devices</h4>
                  <p className="text-sm text-gray-600 dark:text-gray-400">CS108 RFID Readers</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Connection Type</h4>
                  <p className="text-sm text-gray-600 dark:text-gray-400">Web Bluetooth API</p>
                </div>
                <div>
                  <h4 className="text-sm font-medium text-gray-700 dark:text-gray-300 mb-1">Frequency Range</h4>
                  <p className="text-sm text-gray-600 dark:text-gray-400">902-928 MHz (US)</p>
                </div>
              </div>
            </div>
            
            {/* Debug Tools Section */}
            <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
              <div className="flex items-center justify-between mb-4">
                <div className="flex items-center">
                  <Bug className="w-4 h-4 text-gray-500 dark:text-gray-400 mr-2" />
                  <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100">Debug Tools</h3>
                </div>
                <button
                  onClick={() => setIsDebugPanelVisible(!isDebugPanelVisible)}
                  className="text-sm text-blue-600 hover:text-blue-700 font-medium"
                >
                  {isDebugPanelVisible ? 'Hide' : 'Show'} Debug Panel
                </button>
              </div>
              
              {isDebugPanelVisible && (
                <>
                  <div className="flex flex-wrap gap-3 mt-4">
                    <button 
                      onClick={() => {
                        const connectionInfo = {
                          readerState,
                          deviceName,
                          batteryPercentage: getBatteryPercentage(),
                          browserSupported: isBrowserSupported,
                          inventoryRunning: readerState === ReaderState.SCANNING
                        };
                        setDebugData(connectionInfo);
                      }}
                      className="px-4 py-2 bg-blue-100 dark:bg-blue-900 text-blue-700 dark:text-blue-300 rounded-lg text-sm font-medium hover:bg-blue-200 dark:hover:bg-blue-800"
                    >
                      Connection Info
                    </button>
                    <button 
                      onClick={() => {
                        const deviceState = {
                          ...useDeviceStore.getState(),
                          tagStore: useTagStore.getState(),
                          settingsStore: useSettingsStore.getState()
                        };
                        setDebugData(deviceState);
                      }}
                      className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg text-sm font-medium hover:bg-gray-200 dark:hover:bg-gray-600"
                    >
                      Show Device State
                    </button>
                    <button 
                      onClick={() => {
                        setDebugData({ message: 'Command state reset (simulated)' });
                      }}
                      className="px-4 py-2 bg-gray-100 dark:bg-gray-700 text-gray-700 dark:text-gray-300 rounded-lg text-sm font-medium hover:bg-gray-200 dark:hover:bg-gray-600"
                    >
                      Reset Command State
                    </button>
                  </div>
                  
                  {debugData && (
                    <div className="mt-4 p-4 bg-gray-900 rounded-lg overflow-auto max-h-96">
                      <pre className="text-xs text-green-400 font-mono">
                        {JSON.stringify(debugData, null, 2)}
                      </pre>
                    </div>
                  )}
                </>
              )}
            </div>
            
            {/* About Section */}
            <div className="border-t border-gray-200 dark:border-gray-700 pt-6">
              <h3 className="text-sm font-medium text-gray-900 dark:text-gray-100 mb-4">About</h3>
              <div className="space-y-2 text-sm text-gray-600 dark:text-gray-400">
                <p>TrakRF Web v{version}</p>
                <p>A web application for CS108 RFID readers using Web Bluetooth technology.</p>
                <p className="text-xs text-gray-500 dark:text-gray-500">© 2025 TrakRF</p>
              </div>
            </div>
            
            {/* Refresh Device Status Button */}
            <button className="flex items-center text-gray-600 dark:text-gray-400 hover:text-gray-900 dark:hover:text-gray-100 mt-4">
              <RefreshCw className="w-4 h-4 mr-2" />
              <span className="text-sm font-medium">Refresh Device Status</span>
            </button>
          </div>
        )}
      </div>
    </div>
  );
}