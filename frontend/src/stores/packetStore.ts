/**
 * Packet Store - Manages debug packet logging and display
 */
import { create } from 'zustand';
import { createStoreWithTracking } from './createStore';

// Packet log type
interface PacketLogEntry {
  type: 'command' | 'response' | 'notification';
  data: string;
  description?: string;
  length?: number | null;
  timestamp?: number;
}

// Packet Store interface for debug info
interface PacketState {
  showDebugPanel: boolean;
  lastPacket: string | null;
  lastCommandCode: string | null; // Track the last command code for routing responses
  packetLog: PacketLogEntry[];
  
  // Actions
  toggleDebugPanel: () => void;
  setLastPacket: (packet: string | null) => void;
  setLastCommandCode: (code: string | null) => void;
  clearPacketLog: () => void;
  addPacketLog: (packet: PacketLogEntry) => void;
}

export const usePacketStore = create<PacketState>(createStoreWithTracking((set) => ({
  // Initial state
  showDebugPanel: false,
  lastPacket: null,
  lastCommandCode: null,
  packetLog: [],
  
  // Actions
  toggleDebugPanel: () => set((state) => ({ showDebugPanel: !state.showDebugPanel })),
  setLastPacket: (packet) => set({ lastPacket: packet }),
  setLastCommandCode: (code) => set({ lastCommandCode: code }),
  clearPacketLog: () => set({ packetLog: [] }),
  addPacketLog: (packet) => set((state) => {
    // Limit packet log to prevent memory leaks (keep last 100 entries)
    const MAX_PACKET_LOG_SIZE = 100;
    const newLog = [...state.packetLog, { ...packet, timestamp: Date.now() }];
    
    // If we exceed the limit, remove oldest entries
    if (newLog.length > MAX_PACKET_LOG_SIZE) {
      return { packetLog: newLog.slice(-MAX_PACKET_LOG_SIZE) };
    }
    
    return { packetLog: newLog };
  }),
}), 'PacketStore'));