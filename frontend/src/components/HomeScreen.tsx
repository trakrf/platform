import React from 'react';
import { useUIStore, TabType } from '@/stores';
import { Package2, Search, ScanLine, Play } from 'lucide-react';

interface TabCardProps {
  id: TabType;
  label: string;
  icon: React.ReactNode;
  color: string;
  iconColor: string;
  isActive: boolean;
  onClick: () => void;
}

const TabCard: React.FC<TabCardProps> = ({ label, icon, iconColor, onClick }) => {
  return (
    <button
      onClick={onClick}
      className={`
        group relative w-full h-full
        bg-white dark:bg-gray-800 rounded-xl shadow-lg border-2 
        transition-all duration-200 ease-out
        hover:shadow-xl hover:scale-[1.02] active:scale-[0.98]
        border-gray-200 dark:border-gray-700 hover:border-gray-300 dark:hover:border-gray-600
      `}
    >
      <div className="flex flex-col items-center justify-center h-full p-4 sm:p-3">
        <div className={`
          w-10 h-10 sm:w-12 sm:h-12 lg:w-14 lg:h-14 mb-2 transition-transform duration-200
          ${iconColor} group-hover:scale-110
        `}>
          {icon}
        </div>
        <span className="text-sm sm:text-base font-semibold text-gray-900 dark:text-gray-100 transition-colors duration-200">
          {label}
        </span>
      </div>
    </button>
  );
};

export default function HomeScreen() {
  const { setActiveTab } = useUIStore();

  const handleTabClick = (tab: TabType) => {
    setActiveTab(tab);
    // Push new state to browser history
    window.history.pushState({ tab }, '', `#${tab}`);
  };

  const tabs = [
    {
      id: 'inventory' as TabType,
      label: 'Inventory',
      icon: <Package2 className="w-full h-full" />,
      color: 'bg-blue-600 hover:bg-blue-700',
      iconColor: 'text-blue-600'
    },
    {
      id: 'locate' as TabType,
      label: 'Locate',
      icon: <Search className="w-full h-full" />,
      color: 'bg-green-600 hover:bg-green-700',
      iconColor: 'text-green-600'
    },
    {
      id: 'barcode' as TabType,
      label: 'Barcode',
      icon: <ScanLine className="w-full h-full" />,
      color: 'bg-purple-600 hover:bg-purple-700',
      iconColor: 'text-purple-600'
    }
  ];

  return (
    <div className="h-full overflow-auto flex flex-col w-full">
      {/* Tab Grid - Responsive height */}
      <div className="flex items-center justify-center mb-2 md:mb-4">
        <div className="grid grid-cols-1 sm:grid-cols-3 gap-4 w-full max-w-5xl">
          {tabs.map((tab) => (
            <div key={tab.id} className="h-32 sm:h-40 md:h-48">
              <TabCard
                id={tab.id}
                label={tab.label}
                icon={tab.icon}
                color={tab.color}
                iconColor={tab.iconColor}
                isActive={false}
                onClick={() => handleTabClick(tab.id)}
              />
            </div>
          ))}
        </div>
      </div>

      {/* Video Section - Responsive height */}
      <div className="flex items-center justify-center flex-1 min-h-[250px] md:min-h-[300px]">
        <div className="w-full max-w-5xl">
          <div className="relative bg-gray-900 dark:bg-gray-800 rounded-xl overflow-hidden shadow-lg aspect-video lg:aspect-[16/7.2]">
            <div className="h-full bg-gradient-to-br from-gray-800 to-gray-900 flex items-center justify-center p-8">
              <div className="text-center text-white">
                <div className="group flex items-center justify-center w-16 h-16 bg-white bg-opacity-20 hover:bg-opacity-30 rounded-full transition-all duration-200 hover:scale-110 cursor-pointer mx-auto mb-3">
                  <Play className="w-6 h-6 text-white ml-1" />
                </div>
                <h3 className="text-xl font-semibold mb-2">Watch Demo</h3>
                <p className="text-gray-300 text-sm">Learn how to use the RFID scanner</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </div>
  );
}