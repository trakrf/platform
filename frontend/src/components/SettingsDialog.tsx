import React, { useEffect } from 'react';
import { useUIStore } from '@/stores';

export default function SettingsDialog(): React.ReactNode {
  const { 
    showSettingsDialog, 
    setShowSettingsDialog,
    setActiveTab
  } = useUIStore.getState();
  
  // When the settings dialog is shown, redirect to settings tab instead
  useEffect(() => {
    if (showSettingsDialog) {
      setActiveTab('settings');
      setShowSettingsDialog(false);
    }
  }, [showSettingsDialog, setActiveTab, setShowSettingsDialog]);
  
  // This component doesn't render anything visible itself
  return null;
}