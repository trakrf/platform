/**
 * Locate Store - Simplified ring buffer for real-time RSSI tracking
 * 
 * This store maintains a 10-second ring buffer of RSSI readings for locate mode.
 * Data flows directly from packet parsing to this store, no intermediate processing.
 */

import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';

// RSSI data point in the ring buffer
export interface RssiDataPoint {
  timestamp: number;      // Unix timestamp in ms
  nb_rssi: number;        // Narrowband RSSI value (filtered by CS108)
  wb_rssi?: number;       // Wideband RSSI value (if available)
  phase?: number;         // Phase angle (if available)
}

// Store state
interface LocateState {
  // Ring buffer configuration
  bufferSize: number;           // Max number of data points (default 100 for 10 seconds at 10Hz)
  bufferDuration: number;       // Duration in ms to keep data (default 10000ms = 10 seconds)
  
  // Ring buffer data
  rssiBuffer: RssiDataPoint[];  // Circular buffer of RSSI readings
  bufferIndex: number;          // Current write position in buffer
  
  // Current state
  lastUpdateTime: number;       // Timestamp of last RSSI update
  statusMessage: string;        // UI status message
  
  // Statistics (calculated from buffer)
  currentRSSI: number;          // Most recent RSSI value
  averageRSSI: number;          // Average over last second
  peakRSSI: number;             // Peak value in buffer
  updateRate: number;           // Updates per second
  
  // Actions
  addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number) => void;
  setStatusMessage: (message: string) => void;
  clearBuffer: () => void;
  
  // Getters
  getRecentReadings: (duration?: number) => RssiDataPoint[];  // Get readings from last N ms
  getFilteredRSSI: () => number;  // Get time-weighted filtered RSSI
}

// Default RSSI value when no signal
const DEFAULT_RSSI = -120;

export const useLocateStore = create<LocateState>()(
  subscribeWithSelector((set, get) => ({
    // Configuration
    bufferSize: 100,
    bufferDuration: 10000,
    
    // Initialize empty ring buffer
    rssiBuffer: [],
    bufferIndex: 0,
    
    // State
    lastUpdateTime: 0,
    statusMessage: 'Ready to locate',
    
    // Statistics
    currentRSSI: DEFAULT_RSSI,
    averageRSSI: DEFAULT_RSSI,
    peakRSSI: DEFAULT_RSSI,
    updateRate: 0,
    
    // Add new RSSI reading to ring buffer
    addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number) => {
      const now = Date.now();
      const state = get();
      
      // Create new data point
      const dataPoint: RssiDataPoint = {
        timestamp: now,
        nb_rssi,
        wb_rssi,
        phase
      };
      
      // Update ring buffer
      const newBuffer = [...state.rssiBuffer];
      
      // If buffer is full, overwrite oldest entry
      if (newBuffer.length >= state.bufferSize) {
        newBuffer[state.bufferIndex] = dataPoint;
      } else {
        newBuffer.push(dataPoint);
      }
      
      // Clean out old entries (older than bufferDuration)
      const cutoffTime = now - state.bufferDuration;
      const filteredBuffer = newBuffer.filter(point => point.timestamp > cutoffTime);
      
      // Calculate statistics from buffer
      const currentRSSI = nb_rssi;
      
      // Calculate average over last second
      const oneSecondAgo = now - 1000;
      const recentReadings = filteredBuffer.filter(p => p.timestamp > oneSecondAgo);
      const averageRSSI = recentReadings.length > 0
        ? recentReadings.reduce((sum, p) => sum + p.nb_rssi, 0) / recentReadings.length
        : nb_rssi;
      
      // Find peak in entire buffer
      const peakRSSI = filteredBuffer.length > 0
        ? Math.max(...filteredBuffer.map(p => p.nb_rssi))
        : nb_rssi;
      
      // Calculate update rate (updates per second over last 2 seconds)
      const twoSecondsAgo = now - 2000;
      const recentUpdates = filteredBuffer.filter(p => p.timestamp > twoSecondsAgo);
      const updateRate = recentUpdates.length / 2; // Updates per second
      
      // Update state
      set({
        rssiBuffer: filteredBuffer,
        bufferIndex: (state.bufferIndex + 1) % state.bufferSize,
        lastUpdateTime: now,
        currentRSSI,
        averageRSSI: Math.round(averageRSSI),
        peakRSSI,
        updateRate: Math.round(updateRate * 10) / 10 // Round to 1 decimal
      });
    },

    // Set status message
    setStatusMessage: (message: string) => {
      set({ statusMessage: message });
    },
    
    // Clear buffer
    clearBuffer: () => {
      set({
        rssiBuffer: [],
        bufferIndex: 0,
        currentRSSI: DEFAULT_RSSI,
        averageRSSI: DEFAULT_RSSI,
        peakRSSI: DEFAULT_RSSI,
        updateRate: 0,
        lastUpdateTime: 0
      });
    },
    
    // Get recent readings from buffer
    getRecentReadings: (duration: number = 1000) => {
      const state = get();
      const cutoff = Date.now() - duration;
      return state.rssiBuffer.filter(p => p.timestamp > cutoff);
    },
    
    // Get time-weighted filtered RSSI (for smooth gauge display)
    getFilteredRSSI: () => {
      const state = get();
      const now = Date.now();

      // If no readings in the last 1 second, return default (no signal)
      const staleThreshold = 1000;
      if (now - state.lastUpdateTime > staleThreshold) {
        return DEFAULT_RSSI;
      }

      const window = 500; // 500ms window for filtering
      const cutoff = now - window;

      const recentReadings = state.rssiBuffer.filter(p => p.timestamp > cutoff);

      if (recentReadings.length === 0) {
        return state.currentRSSI;
      }

      // Time-weighted average (more recent = higher weight)
      let weightedSum = 0;
      let totalWeight = 0;

      recentReadings.forEach(point => {
        const age = now - point.timestamp;
        const weight = 1 - (age / window); // Linear decay
        weightedSum += point.nb_rssi * weight;
        totalWeight += weight;
      });

      return totalWeight > 0 ? Math.round(weightedSum / totalWeight) : state.currentRSSI;
    }
  }))
);