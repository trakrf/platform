import React from 'react';
import { useUIStore, useDeviceStore } from '@/stores';
import type { TabType } from '@/stores';
import { ReaderState } from '@/worker/types/reader';
import { Package2, Search, Settings, ScanLine, HelpCircle, Home, Package, MapPinned, BarChart3 } from 'lucide-react';
import { version } from '../../package.json';

interface NavItemProps {
  id: TabType;
  label: string;
  icon: React.ReactNode;
  isActive: boolean;
  onClick: () => void;
  tooltip: string;
}

const NavItem: React.FC<NavItemProps> = ({ id, label, icon, isActive, onClick, tooltip }) => {
  const [showTooltip, setShowTooltip] = React.useState(false);
  
  return (
    <button
      onClick={onClick}
      onMouseEnter={() => setShowTooltip(true)}
      onMouseLeave={() => setShowTooltip(false)}
      title={tooltip}
      data-testid={`menu-item-${id}`}
      className={`relative flex items-center w-full px-3 py-2 text-left rounded-lg text-sm font-medium transition-colors ${
        isActive 
          ? 'bg-blue-600 text-white' 
          : 'text-gray-700 dark:text-gray-300 hover:bg-gray-100 dark:hover:bg-gray-700'
      }`}
    >
      <div className="flex items-center">
        <div className="w-5 h-5 mr-3">
          {icon}
        </div>
        {label}
      </div>
      {showTooltip && (
        <div className="absolute left-full top-1/2 -translate-y-1/2 ml-2 w-64 p-3 bg-gray-900 text-white text-sm rounded-lg shadow-lg z-50 pointer-events-none">
          <div className="relative">
            <div className="absolute top-1/2 -left-2 -translate-y-1/2 w-0 h-0 border-t-[6px] border-t-transparent border-b-[6px] border-b-transparent border-r-[6px] border-r-gray-900"></div>
            {tooltip}
          </div>
        </div>
      )}
    </button>
  );
};

export default function TabNavigation() {
  // Read directly from store to avoid race conditions
  const activeTab = useUIStore((state) => state.activeTab);
  const readerState = useDeviceStore((state) => state.readerState);
  const { setActiveTab } = useUIStore.getState();
  
  // Handle browser back button navigation
  React.useEffect(() => {
    // Handle browser back button
    const handlePopState = (event: PopStateEvent) => {
      if (event.state?.tab) {
        setActiveTab(event.state.tab);
      }
    };
    
    // Add event listener
    window.addEventListener('popstate', handlePopState);
    
    // Set initial state
    const initialTab = window.location.hash.slice(1) || activeTab;
    window.history.replaceState({ tab: initialTab }, '', `#${initialTab}`);
    
    return () => {
      window.removeEventListener('popstate', handlePopState);
    };
  }, [activeTab, setActiveTab]);
  
  const handleTabClick = (tab: TabType) => {
    setActiveTab(tab);
    // Push new state to browser history
    window.history.pushState({ tab }, '', `#${tab}`);
  };

  // Get device status display
  const getDeviceStatus = () => {
    switch (readerState) {
      case ReaderState.DISCONNECTED:
        return { text: 'Disconnected', color: 'text-red-600 dark:text-red-400', bgColor: 'bg-red-100 dark:bg-red-900' };
      case ReaderState.CONNECTING:
        return { text: 'Connecting', color: 'text-yellow-600 dark:text-yellow-400', bgColor: 'bg-yellow-100 dark:bg-yellow-900' };
      case ReaderState.CONNECTED:
        return { text: 'Connected', color: 'text-green-600 dark:text-green-400', bgColor: 'bg-green-100 dark:bg-green-900' };
      case ReaderState.CONFIGURING:
        return { text: 'Configuring', color: 'text-blue-600 dark:text-blue-400', bgColor: 'bg-blue-100 dark:bg-blue-900' };
      case ReaderState.BUSY:
        return { text: 'Scanning', color: 'text-purple-600 dark:text-purple-400', bgColor: 'bg-purple-100 dark:bg-purple-900' };
      case ReaderState.SCANNING:
        return { text: 'Scanning', color: 'text-purple-600 dark:text-purple-400', bgColor: 'bg-purple-100 dark:bg-purple-900' };
      case ReaderState.ERROR:
        return { text: 'Error', color: 'text-red-600 dark:text-red-400', bgColor: 'bg-red-100 dark:bg-red-900' };
      default:
        return { text: 'Unknown', color: 'text-gray-600 dark:text-gray-400', bgColor: 'bg-gray-100 dark:bg-gray-800' };
    }
  };

  const deviceStatus = getDeviceStatus();
  
  return (
    <div className="flex flex-col h-screen">
      {/* TrakRF Logo and Title */}
      <div className="p-6 border-b border-gray-200 dark:border-gray-700">
        <div className="relative group">
          <div className="flex items-center">
            <img src="/logo.png" alt="TrakRF Logo" className="w-8 h-8 mr-3" />
            <div>
              <h1 className="text-lg font-bold text-gray-900 dark:text-gray-100">TrakRF</h1>
              <p className="text-sm text-gray-500 dark:text-gray-400">Handheld Tag Reader</p>
              <p className="text-xs text-gray-400 dark:text-gray-500 mt-0.5">v{version}</p>
            </div>
          </div>

          {/* Attribution Tooltip */}
          <div className="absolute left-0 top-full mt-2 w-80 p-4 bg-gray-900 text-white text-xs rounded-lg shadow-xl z-50 opacity-0 invisible group-hover:opacity-100 group-hover:visible transition-all duration-200 pointer-events-none">
            <div className="space-y-2">
              <div>
                <div className="font-semibold mb-1">Built with:</div>
                <div className="text-gray-300">React, TypeScript, Vite, Tailwind CSS, Zustand</div>
              </div>
              <div className="border-t border-gray-700 pt-2">
                <div className="font-semibold mb-1">Sound Effects:</div>
                <div className="text-gray-300">
                  CC BY 3.0 US from{' '}
                  <span className="text-blue-400">rcptones.com/dev_tones/</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Device Status */}
      <div className="px-6 py-4 border-b border-gray-200 dark:border-gray-700">
        <div className="text-sm text-gray-600 dark:text-gray-400 mb-2">Device Status</div>
        <div className="flex items-center">
          <div className={`w-2 h-2 rounded-full mr-2 ${deviceStatus.bgColor.replace('bg-', 'bg-').replace('-100', '-600')} ${(readerState === ReaderState.SCANNING || readerState === ReaderState.BUSY) ? 'animate-pulse' : ''}`}></div>
          <span className={`text-sm font-medium ${deviceStatus.color}`}>
            {deviceStatus.text}
          </span>
        </div>
      </div>

      {/* Navigation Items */}
      <div className="flex-1 px-6 py-4">
        <nav className="space-y-2">
          <NavItem
            id="home"
            label="Home"
            isActive={activeTab === 'home'}
            onClick={() => handleTabClick('home')}
            icon={<Home className="w-5 h-5" />}
            tooltip="Go to the main dashboard with quick access to all features"
          />
          
          <NavItem
            id="inventory"
            label="Inventory"
            isActive={activeTab === 'inventory'}
            onClick={() => handleTabClick('inventory')}
            icon={<Package2 className="w-5 h-5" />}
            tooltip="View scanned items and check what's missing from your list"
          />
          
          <NavItem
            id="locate"
            label="Locate"
            isActive={activeTab === 'locate'}
            onClick={() => handleTabClick('locate')}
            icon={<Search className="w-5 h-5" />}
            tooltip="Find a specific item by walking around with the scanner"
          />
          
          <NavItem
            id="barcode"
            label="Barcode"
            isActive={activeTab === 'barcode'}
            onClick={() => handleTabClick('barcode')}
            icon={<ScanLine className="w-5 h-5" />}
            tooltip="Use your phone camera to scan regular barcodes"
          />

          <NavItem
            id="assets"
            label="Assets"
            isActive={activeTab === 'assets'}
            onClick={() => handleTabClick('assets')}
            icon={<Package className="w-5 h-5" />}
            tooltip="Manage your assets - create, view, and track asset information"
          />

          <NavItem
            id="locations"
            label="Locations"
            isActive={activeTab === 'locations'}
            onClick={() => handleTabClick('locations')}
            icon={<MapPinned className="w-5 h-5" />}
            tooltip="Manage your locations - create, view, and organize location data"
          />

          <NavItem
            id="reports"
            label="Reports"
            isActive={activeTab === 'reports' || activeTab === 'reports-history'}
            onClick={() => handleTabClick('reports')}
            icon={<BarChart3 className="w-5 h-5" />}
            tooltip="View asset location reports and movement history"
          />

          <NavItem
            id="settings"
            label="Settings"
            isActive={activeTab === 'settings'}
            onClick={() => handleTabClick('settings')}
            icon={<Settings className="w-5 h-5" />}
            tooltip="Configure device and application settings"
          />
          
          <NavItem
            id="help"
            label="Help"
            isActive={activeTab === 'help'}
            onClick={() => handleTabClick('help')}
            icon={<HelpCircle className="w-5 h-5" />}
            tooltip="Quick answers to common questions"
          />
        </nav>
      </div>
    </div>
  );
}