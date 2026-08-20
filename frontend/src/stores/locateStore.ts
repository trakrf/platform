/**
 * Locate Store - Simplified ring buffer for real-time RSSI tracking
 * 
 * This store maintains a 10-second ring buffer of RSSI readings for locate mode.
 * Data flows directly from packet parsing to this store, no intermediate processing.
 */

import { create } from 'zustand';
import { subscribeWithSelector } from 'zustand/middleware';
import { recordStoreUpdateStart } from '../lib/perf/locate-metrics';
import { removeLeadingZeros } from '../utils/reconciliationUtils';

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
  targetEPC: string;            // The target the buffered readings were read for
  
  // Statistics (calculated from buffer)
  currentRSSI: number;          // Most recent RSSI value
  averageRSSI: number;          // Average over last second
  peakRSSI: number;             // Peak value in buffer
  updateRate: number;           // Updates per second
  
  // Actions
  addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number, workerTimestamp?: number, epc?: string) => void;
  setStatusMessage: (message: string) => void;
  setTarget: (epc: string) => void;
  clearBuffer: () => void;
  
  // Getters
  getRecentReadings: (duration?: number) => RssiDataPoint[];  // Get readings from last N ms
  getFilteredRSSI: () => number;  // Get time-weighted filtered RSSI
  getStatistics: () => LocateStatistics;  // Statistics with staleness applied
}

// The four numbers the Statistics panel renders, as the operator should read
// them right now
export interface LocateStatistics {
  currentRSSI: number;
  averageRSSI: number;
  peakRSSI: number;
  updateRate: number;
}

// Default RSSI value when no signal
export const DEFAULT_RSSI = -120;

// Does a read belong to the tag we are looking for?
//
// The operator may type a leading-zero-stripped EPC while the tag reports its
// full width, which is the equivalence TRA-1108/TRA-1120 already build into the
// hardware mask by OR-ing a 96- and a 128-bit descriptor. This filter has to
// use the same one: comparing raw strings would drop every legitimate read for
// a stripped target and re-break locate for 128-bit EPCs.
//
// An empty target means nothing has been selected yet, and a reading with no
// EPC gives no basis to reject it. Both admit.
export function isReadingForTarget(readEPC: string | undefined, targetEPC: string): boolean {
  if (!targetEPC || !readEPC) return true;
  return removeLeadingZeros(readEPC.toUpperCase()) === removeLeadingZeros(targetEPC.toUpperCase());
}

// How long after the last read the signal counts as gone. One value, because
// the gauge, the Status row and the Statistics panel must agree about whether
// the tag is being heard (TRA-1089).
export const STALE_THRESHOLD_MS = 1000;

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
    targetEPC: '',
    
    // Statistics
    currentRSSI: DEFAULT_RSSI,
    averageRSSI: DEFAULT_RSSI,
    peakRSSI: DEFAULT_RSSI,
    updateRate: 0,
    
    // Add new RSSI reading to ring buffer
    addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number, workerTimestamp?: number, epc?: string) => {
      // Reject reads from tags that are not the target.
      //
      // handler.ts has always said "the application layer (locateStore) will
      // filter for the target EPC" — it never did, so the hardware Gen2 Select
      // was the only filter, and it is demonstrably imperfect: measured on the
      // turntable bench, ~0.03% of reads come from a tag at the edge of the
      // field that mis-decoded the Select and asserted SL. One stray sample is
      // enough to turn an honest "no signal" into a plausible reading, which is
      // this ticket's symptom by a different route than the stale buffer.
      //
      // This also drops the read still in flight from the old mask when the
      // target changes mid-search, instead of leaving it on screen until
      // staleness catches it a second later.
      if (!isReadingForTarget(epc, get().targetEPC)) {
        return;
      }

      // Start metrics recording (returns completion callback)
      const completeMetrics = recordStoreUpdateStart(workerTimestamp);

      const now = Date.now();

      // Debug logging - enable with: window.__LOCATE_DEBUG = true
      if ((window as unknown as Record<string, unknown>).__LOCATE_DEBUG) {
        console.log(`[RSSI] ${now} | ${nb_rssi} dBm | wb: ${wb_rssi ?? 'n/a'}`);
      }
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

      // Complete metrics recording
      completeMetrics();
    },

    // Set status message
    setStatusMessage: (message: string) => {
      set({ statusMessage: message });
    },

    // Point the buffer at a target. A reading only says anything about the tag
    // it was read for, so retargeting throws the readings away rather than
    // showing the previous tag's signal for a search that is returning nothing
    // (TRA-1123). Re-asserting the same target is a no-op, because the screen
    // re-syncs on every mount and every settings change.
    setTarget: (epc: string) => {
      if (get().targetEPC === epc) return;
      get().clearBuffer();
      set({ targetEPC: epc });
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
    
    // Statistics as the operator should read them.
    //
    // currentRSSI/averageRSSI/peakRSSI/updateRate are recomputed only inside
    // addRssiReading(), so once reads stop they freeze on the last value
    // indefinitely. On a screen whose whole job is proximity feedback, a frozen
    // number from a search that is over is a wrong answer, not an old one: it
    // reads as "the tag is right here" for a search that has returned nothing
    // (TRA-1123). Decay them on the same staleness signal getFilteredRSSI()
    // already uses, so the gauge, the Status row and this panel cannot
    // disagree.
    getStatistics: () => {
      const state = get();

      if (Date.now() - state.lastUpdateTime > STALE_THRESHOLD_MS) {
        return {
          currentRSSI: DEFAULT_RSSI,
          averageRSSI: DEFAULT_RSSI,
          peakRSSI: DEFAULT_RSSI,
          updateRate: 0
        };
      }

      return {
        currentRSSI: state.currentRSSI,
        averageRSSI: state.averageRSSI,
        peakRSSI: state.peakRSSI,
        updateRate: state.updateRate
      };
    },

    // Get time-weighted filtered RSSI (for smooth gauge display)
    getFilteredRSSI: () => {
      const state = get();
      const now = Date.now();

      // If no readings in the last 1 second, return default (no signal)
      if (now - state.lastUpdateTime > STALE_THRESHOLD_MS) {
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