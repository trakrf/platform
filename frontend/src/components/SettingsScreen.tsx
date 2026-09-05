import React, { useState, useEffect } from 'react';
import { useDeviceStore, useSettingsStore, useTagStore, useUIStore } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { useBluetoothSupport } from '@/hooks/useBluetoothSupport';
import { connectErrorMessage } from '@/hooks/connectErrorMessage';
import { Bluetooth, Zap, Settings2, Info, RefreshCw, ChevronDown, ChevronUp, Smartphone, WifiOff, Battery, Bug } from 'lucide-react';
import { ConnectIcon } from '@/components/icons/ConnectIcon';
import toast from 'react-hot-toast';
import { appVersion } from '@/version';
import { ReaderDetailsPanel } from '@/components/ReaderDetailsPanel';

export default function SettingsScreen() {
  // Set active tab when component mounts - standard React pattern
  React.useEffect(() => {
    useUIStore.getState().setActiveTab('settings');
  }, []);

  // Local state to track Zustand store values
  const [readerState, setLocalReaderState] = useState(useDeviceStore.getState().readerState);
  const [batteryPercentage, setBatteryPercentage] = useState(useDeviceStore.getState().batteryPercentage);
  const [deviceName, setDeviceName] = useState(useDeviceStore.getState().deviceName);
  const [readerDetails, setReaderDetails] = useState(useDeviceStore.getState().readerDetails);
  
  const [rfPower, setLocalRfPower] = useState(useSettingsStore.getState().rfid?.transmitPower ?? 30);
  const [isAdvancedExpanded, setIsAdvancedExpanded] = useState(false);
  const { supported: isBrowserSupported, setupPrerequisite } = useBluetoothSupport();
  const [isDebounced, setIsDebounced] = useState(false);
  const [isBlinking, setIsBlinking] = useState(false);
  const [selectedSession, setSelectedSession] = useState('S1');
  const [isDebugPanelVisible, setIsDebugPanelVisible] = useState(false);
  const [debugData, setDebugData] = useState<Record<string, unknown> | null>(null);

  // New settings state
  const [workerLogLevel, setLocalWorkerLogLevel] = useState(useSettingsStore.getState().system?.workerLogLevel || 'info');
  const [batteryCheckInterval, setLocalBatteryCheckInterval] = useState(useSettingsStore.getState().system?.batteryCheckInterval || 60);

  // Tag data capture (TRA-1251). userWords uses ?? rather than || because 0 is
  // a meaningful value here — it means read TID only.
  const [captureAllTagData, setLocalCaptureAllTagData] = useState(useSettingsStore.getState().rfid?.captureAllTagData ?? false);
  const [tidWords, setLocalTidWords] = useState(useSettingsStore.getState().rfid?.tidWords ?? 6);
  const [userOffset, setLocalUserOffset] = useState(useSettingsStore.getState().rfid?.userOffset ?? 0);
  const [userWords, setLocalUserWords] = useState(useSettingsStore.getState().rfid?.userWords ?? 4);

  // Get setter functions from stores
  const {
    setTransmitPower,
    setWorkerLogLevel,
    setBatteryCheckInterval,
    setCaptureAllTagData,
    setTidWords,
    setUserOffset,
    setUserWords
  } = useSettingsStore.getState();
  const { connect, disconnect } = useDeviceStore.getState();
  // Removed inventoryRunning - using readerState === ReaderState.SCANNING instead
  
  // Subscribe to store changes
  useEffect(() => {
    // Subscribe to device store changes
    const unsubDeviceStore = useDeviceStore.subscribe((state) => {
      setLocalReaderState(state.readerState);
      setBatteryPercentage(state.batteryPercentage);
      setDeviceName(state.deviceName);
      setReaderDetails(state.readerDetails);
    });
    
    // Subscribe to settings store changes
    const unsubSettingsStore = useSettingsStore.subscribe((state) => {
      setLocalRfPower(state.rfid?.transmitPower ?? 30);
      setLocalWorkerLogLevel(state.system?.workerLogLevel || 'info');
      setLocalBatteryCheckInterval(state.system?.batteryCheckInterval || 60);
      setLocalCaptureAllTagData(state.rfid?.captureAllTagData ?? false);
      setLocalTidWords(state.rfid?.tidWords ?? 6);
      setLocalUserOffset(state.rfid?.userOffset ?? 0);
      setLocalUserWords(state.rfid?.userWords ?? 4);
    });
    
    // Cleanup subscriptions
    return () => {
      unsubDeviceStore();
      unsubSettingsStore();
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
      toast.error(connectErrorMessage(error, setupPrerequisite?.connectHint));
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

          {/*
            Here rather than buried in Advanced Settings. This is the answer to
            "what is this reader" — the question a support conversation opens
            with and the one every capture we have ever taken cannot answer.
            The Device Information block below is about the APP; this is about
            the hardware in the operator's hand. TRA-1232.
          */}
          <ReaderDetailsPanel details={readerDetails} />
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
                data-testid="worker-log-level"
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

            {/* Capture all tag data (TRA-1251) */}
            <div>
              <label className="flex items-start cursor-pointer">
                <input
                  type="checkbox"
                  checked={captureAllTagData}
                  onChange={(e) => setCaptureAllTagData(e.target.checked)}
                  className="mt-1 w-4 h-4 text-blue-600 border-gray-300 dark:border-gray-600 rounded focus:ring-blue-500"
                />
                <span className="ml-3">
                  <span className="block text-sm font-medium text-gray-900 dark:text-gray-100">
                    Capture all tag data
                  </span>
                  <span className="block text-xs text-gray-500 dark:text-gray-400 mt-1">
                    Also read each tag&apos;s TID and USER memory during a scan, and include
                    them in the export. Measured against a dense stack, this finds roughly
                    60% as many tags in the same time — nothing is lost, it just accumulates
                    slower, so hold the trigger longer or make several passes. Leave it off
                    for ordinary inventory.
                  </span>
                </span>
              </label>

              {captureAllTagData && (
                <div className="mt-4 pl-7 space-y-4">
                  <div>
                    <label
                      htmlFor="tid-words"
                      className="block text-sm font-medium text-gray-900 dark:text-gray-100 mb-2"
                    >
                      TID words
                    </label>
                    <input
                      id="tid-words"
                      type="number"
                      min="1"
                      max="255"
                      value={tidWords}
                      onChange={(e) => setTidWords(parseInt(e.target.value, 10))}
                      className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                      16-bit words of TID to read. 6 covers a chip serial; reduce it to 2 if
                      reads come back empty, since not every chip carries more than that.
                    </p>
                  </div>

                  <div>
                    <label
                      htmlFor="user-offset"
                      className="block text-sm font-medium text-gray-900 dark:text-gray-100 mb-2"
                    >
                      USER offset
                    </label>
                    <input
                      id="user-offset"
                      type="number"
                      min="0"
                      max="65535"
                      value={userOffset}
                      onChange={(e) => setUserOffset(parseInt(e.target.value, 10))}
                      className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                      Word offset to start reading USER memory from.
                    </p>
                  </div>

                  <div>
                    <label
                      htmlFor="user-words"
                      className="block text-sm font-medium text-gray-900 dark:text-gray-100 mb-2"
                    >
                      USER words
                    </label>
                    <input
                      id="user-words"
                      type="number"
                      min="0"
                      max="255"
                      value={userWords}
                      onChange={(e) => setUserWords(parseInt(e.target.value, 10))}
                      className="w-full px-4 py-2 border border-gray-300 dark:border-gray-600 bg-white dark:bg-gray-700 text-gray-900 dark:text-gray-100 rounded-lg focus:ring-2 focus:ring-blue-500 focus:border-blue-500"
                    />
                    <p className="text-xs text-gray-500 dark:text-gray-400 mt-2">
                      Words of USER memory to read. <strong>Set this to 0 to read TID
                      only</strong> — a two-bank read fails as a unit against a chip that
                      has no USER memory, and this is the way past that.
                    </p>
                  </div>
                </div>
              )}
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
                  <p className="text-sm text-gray-600 dark:text-gray-400">TrakRF Web {appVersion}</p>
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
                          // What the reader IS, alongside what it is doing.
                          // Whatever a diagnostic carries has to say which
                          // firmware it was taken on, or it cannot be
                          // attributed later — and flashing destroys the
                          // attribution permanently. TRA-1232.
                          readerDetails,
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
                <p>TrakRF Web {appVersion}</p>
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