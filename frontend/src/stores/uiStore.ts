/**
 * UI Store - Manages UI state including active tab and dialog visibility
 */
import { create } from 'zustand';

// Define tab types
export type TabType = 'scan' | 'settings' | 'locate' | 'help' | 'assets' | 'locations' | 'scan-devices' | 'output-devices' | 'live-reads' | 'reports' | 'reports-history' | 'mustering' | 'login' | 'signup' | 'forgot-password' | 'reset-password' | 'create-org' | 'org-members' | 'org-settings' | 'org-geofence-defaults' | 'accept-invite' | 'api-keys' | 'admin-orgs';
export type TabId = TabType;

// Scan tab read mode (TRA-1031). Session-local, never persisted — always
// boots as 'rfid'; barcode is the secondary use case.
export type ScanTabMode = 'rfid' | 'barcode';

// Notification interface
interface Notification {
  id?: string;
  message: string;
  type?: 'info' | 'error' | 'success' | 'warning';
}

// UI Store interface
interface UIState {
  activeTab: TabId;
  scanTabMode: ScanTabMode;
  sidebarOpen: boolean;
  settingsOpen: boolean;
  contextMenuOpen: string | null;
  notifications: Notification[];
  showSettingsDialog: boolean;
}

interface UIActions {
  setActiveTab: (tab: TabId) => void;
  setScanTabMode: (mode: ScanTabMode) => void;
  setSidebarOpen: (open: boolean) => void;
  setSettingsOpen: (open: boolean) => void;
  setContextMenuOpen: (id: string | null) => void;
  addNotification: (notification: Omit<Notification, 'id'>) => void;
  removeNotification: (id: string) => void;
  setShowSettingsDialog: (show: boolean) => void;
}

export const useUIStore = create<UIState & UIActions>((set) => ({
  activeTab: 'scan',
  scanTabMode: 'rfid',
  sidebarOpen: false,
  settingsOpen: false,
  contextMenuOpen: null,
  notifications: [],
  showSettingsDialog: false,

  setActiveTab: (tab) => set({ activeTab: tab }),
  setScanTabMode: (mode) => set({ scanTabMode: mode }),
  setSidebarOpen: (open) => set({ sidebarOpen: open }),
  setSettingsOpen: (open) => set({ settingsOpen: open }),
  setContextMenuOpen: (id) => set({ contextMenuOpen: id }),
  addNotification: (notification) => set((state) => ({
    notifications: [...state.notifications, { ...notification, id: Math.random().toString(36) }]
  })),
  removeNotification: (id) => set((state) => ({
    notifications: state.notifications.filter((n) => n.id !== id)
  })),
  setShowSettingsDialog: (show) => set({ showSettingsDialog: show }),
}));