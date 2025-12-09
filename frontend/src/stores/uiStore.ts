/**
 * UI Store - Manages UI state including active tab and dialog visibility
 */
import { create } from 'zustand';

// Define tab types
export type TabType = 'home' | 'inventory' | 'barcode' | 'settings' | 'locate' | 'help' | 'assets' | 'locations' | 'login' | 'signup' | 'forgot-password' | 'reset-password' | 'create-org' | 'org-members' | 'org-settings' | 'accept-invite';
export type TabId = TabType;

// Notification interface
interface Notification {
  id?: string;
  message: string;
  type?: 'info' | 'error' | 'success' | 'warning';
}

// UI Store interface
interface UIState {
  activeTab: TabId;
  sidebarOpen: boolean;
  settingsOpen: boolean;
  contextMenuOpen: string | null;
  notifications: Notification[];
  showSettingsDialog: boolean;
}

interface UIActions {
  setActiveTab: (tab: TabId) => void;
  setSidebarOpen: (open: boolean) => void;
  setSettingsOpen: (open: boolean) => void;
  setContextMenuOpen: (id: string | null) => void;
  addNotification: (notification: Omit<Notification, 'id'>) => void;
  removeNotification: (id: string) => void;
  setShowSettingsDialog: (show: boolean) => void;
}

export const useUIStore = create<UIState & UIActions>((set) => ({
  activeTab: 'home',
  sidebarOpen: false,
  settingsOpen: false,
  contextMenuOpen: null,
  notifications: [],
  showSettingsDialog: false,

  setActiveTab: (tab) => set({ activeTab: tab }),
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