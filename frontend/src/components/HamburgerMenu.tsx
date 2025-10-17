import React, { useState, useEffect, useRef } from 'react';
import { useUIStore, TabType } from '@/stores';

interface MenuItemProps {
  id: TabType;
  label: string;
  icon: React.ReactNode;
  isActive: boolean;
  onClick: () => void;
}

const MenuItem: React.FC<MenuItemProps> = ({ id, label, icon, isActive, onClick }) => {
  return (
    <button
      onClick={onClick}
      className={`flex items-center w-full p-4 min-h-[48px] mb-2 rounded-xl transition-all duration-200 ${
        isActive 
          ? 'bg-gradient-to-r from-blue-500 to-blue-600 text-white shadow-md' 
          : 'text-slate-700 dark:text-slate-200 hover:bg-slate-100 dark:hover:bg-slate-700'
      }`}
      data-testid={`menu-item-${id}`}
    >
      <div className={`w-6 h-6 mr-3 flex-shrink-0 transition-transform duration-200 ${
        isActive ? 'scale-110' : ''
      }`}>{icon}</div>
      <span className="font-semibold">{label}</span>
      {isActive && (
        <div className="ml-auto w-2 h-2 bg-white rounded-full"></div>
      )}
    </button>
  );
};

export default function HamburgerMenu() {
  // Local state to track Zustand store values
  const [activeTab, setLocalActiveTab] = useState(useUIStore.getState().activeTab);
  const [isMenuOpen, setIsMenuOpen] = useState(false);
  const { setActiveTab } = useUIStore.getState();
  const menuRef = useRef<HTMLDivElement>(null);
  
  // Subscribe to store changes
  useEffect(() => {
    const unsubUIStore = useUIStore.subscribe(
      (state) => {
        setLocalActiveTab(state.activeTab);
      }
    );
    
    return () => {
      unsubUIStore();
    };
  }, []);
  
  // Close menu when clicking outside
  useEffect(() => {
    const handleClickOutside = (event: MouseEvent) => {
      if (menuRef.current && !menuRef.current.contains(event.target as Node)) {
        setIsMenuOpen(false);
      }
    };

    document.addEventListener('mousedown', handleClickOutside);
    return () => {
      document.removeEventListener('mousedown', handleClickOutside);
    };
  }, []);
  
  const handleTabClick = (tab: TabType) => {
    setActiveTab(tab);
    setIsMenuOpen(false);
  };
  
  return (
    <div className="relative" ref={menuRef} data-testid="hamburger-menu">
      <button 
        onClick={() => setIsMenuOpen(!isMenuOpen)}
        className="p-3 text-slate-600 dark:text-slate-300 hover:text-slate-900 dark:hover:text-slate-100 hover:bg-slate-100 dark:hover:bg-slate-700 rounded-lg transition-colors duration-200 focus:outline-none focus:ring-2 focus:ring-blue-500"
        aria-label="Menu"
        data-testid="hamburger-button"
      >
        <svg 
          className="h-6 w-6" 
          fill="none" 
          viewBox="0 0 24 24" 
          stroke="currentColor"
        >
          <path 
            strokeLinecap="round" 
            strokeLinejoin="round" 
            strokeWidth={2} 
            d="M4 6h16M4 12h16M4 18h16" 
          />
        </svg>
      </button>
      
      {isMenuOpen && (
        <div className="absolute right-0 top-full mt-3 w-72 rounded-2xl shadow-xl bg-white dark:bg-slate-800 border border-slate-200 dark:border-slate-700 z-50" data-testid="hamburger-dropdown">
          <div className="p-4">
            <MenuItem
              id="inventory"
              label="Inventory"
              isActive={activeTab === 'inventory'}
              onClick={() => handleTabClick('inventory')}
              icon={
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z" />
                </svg>
              }
            />
            
            <MenuItem
              id="locate"
              label="Locate"
              isActive={activeTab === 'locate'}
              onClick={() => handleTabClick('locate')}
              icon={
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M21 21l-6-6m2-5a7 7 0 11-14 0 7 7 0 0114 0z" />
                </svg>
              }
            />
            
            {/* Temporarily hidden - barcode functionality not working
            <MenuItem
              id="barcode"
              label="Barcode"
              isActive={activeTab === 'barcode'}
              onClick={() => handleTabClick('barcode')}
              icon={
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M12 4v1m6 11h2m-6 0h-2v4m0-11v3m0 0h.01M12 12h4.01M16 20h4M4 12h4m12 0h.01M5 8h2a1 1 0 001-1V5a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1zm12 0h2a1 1 0 001-1V5a1 1 0 00-1-1h-2a1 1 0 00-1 1v2a1 1 0 001 1zM5 20h2a1 1 0 001-1v-2a1 1 0 00-1-1H5a1 1 0 00-1 1v2a1 1 0 001 1z" />
                </svg>
              }
            />
            */}
            
            <MenuItem
              id="settings"
              label="Settings"
              isActive={activeTab === 'settings'}
              onClick={() => handleTabClick('settings')}
              icon={
                <svg xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24" stroke="currentColor">
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M10.325 4.317c.426-1.756 2.924-1.756 3.35 0a1.724 1.724 0 002.573 1.066c1.543-.94 3.31.826 2.37 2.37a1.724 1.724 0 001.065 2.572c1.756.426 1.756 2.924 0 3.35a1.724 1.724 0 00-1.066 2.573c.94 1.543-.826 3.31-2.37 2.37a1.724 1.724 0 00-2.572 1.065c-.426 1.756-2.924 1.756-3.35 0a1.724 1.724 0 00-2.573-1.066c-1.543.94-3.31-.826-2.37-2.37a1.724 1.724 0 00-1.065-2.572c-1.756-.426-1.756-2.924 0-3.35a1.724 1.724 0 001.066-2.573c-.94-1.543.826-3.31 2.37-2.37.996.608 2.296.07 2.572-1.065z" />
                  <path strokeLinecap="round" strokeLinejoin="round" strokeWidth={2} d="M15 12a3 3 0 11-6 0 3 3 0 016 0z" />
                </svg>
              }
            />
          </div>
        </div>
      )}
    </div>
  );
}