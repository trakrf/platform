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

  // Is a search running right now? Driven from the reader's state and the
  // trigger edge by device-manager — see setSearchActive for why it is not
  // derived from staleness (TRA-1171).
  searchActive: boolean;

  // When the running search stopped, so the held readouts can be judged as of
  // the release rather than as of now (TRA-1171). 0 means no search has ended
  // yet in this session.
  searchEndedAt: number;

  // Statistics (calculated from buffer)
  currentRSSI: number;          // Most recent RSSI value
  averageRSSI: number;          // Average over last second
  peakRSSI: number;             // Peak value in buffer
  updateRate: number;           // Updates per second
  
  // Actions
  addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number, workerTimestamp?: number, epc?: string) => void;
  setStatusMessage: (message: string) => void;
  setTarget: (epc: string) => void;
  setSearchActive: (active: boolean) => void;
  clearBuffer: () => void;

  // Getters
  getRecentReadings: (duration?: number) => RssiDataPoint[];  // Get readings from last N ms
  getFilteredRSSI: () => number;  // Get time-weighted filtered RSSI
  getStatistics: () => LocateStatistics;  // Statistics with staleness applied
  isHearingTag: () => boolean;    // Is the target audible right now?
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

// Had the signal already gone at the moment the operator is reading?
//
// While a search runs, that moment is now. Once it has stopped it is the
// release — so a held result is the one that was actually on screen when the
// trigger came up. Judging a held value against `now` instead would let a
// search that ended hearing nothing revive its last reading the instant the
// staleness rule stopped applying, and the gauge would climb off zero after
// the operator let go (TRA-1171).
//
// One helper rather than two copies, for the reason STALE_THRESHOLD_MS is one
// value: the gauge and the Statistics panel must not disagree about whether
// the tag was being heard.
function signalWasStale(state: Pick<LocateState, 'searchActive' | 'searchEndedAt' | 'lastUpdateTime'>): boolean {
  const readAt = state.searchActive ? Date.now() : state.searchEndedAt;
  return readAt - state.lastUpdateTime > STALE_THRESHOLD_MS;
}

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
    searchActive: false,
    searchEndedAt: 0,

    // Statistics
    currentRSSI: DEFAULT_RSSI,
    averageRSSI: DEFAULT_RSSI,
    peakRSSI: DEFAULT_RSSI,
    updateRate: 0,
    
    // Add new RSSI reading to ring buffer
    addRssiReading: (nb_rssi: number, wb_rssi?: number, phase?: number, workerTimestamp?: number, epc?: string) => {
      // The release gate (TRA-1171).
      //
      // Several tag packets per stop keep arriving after the ABORT — measured
      // on hardware, in every run. They are genuinely fresh, so every
      // staleness defence below passes them through by design, and they are
      // what keeps the gauge moving and the alarm sounding after the operator
      // has let go. Telling the UI about the release sooner shortens that
      // tail; only refusing to consume the reads ends it.
      if (!get().searchActive) {
        return;
      }

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

    // A search is running, or it is not. While it is not, reads are refused
    // and the staleness decay is suspended: the last value the operator saw is
    // the RESULT of the search they just ran, not a stale reading — they
    // released the trigger in order to read it.
    //
    // Zeroing it instead would be a false negative on the primary function of
    // this screen. On a tag finder, nothing on the gauge reads as "the item is
    // not here", which is the failure TRA-1080 and TRA-1123 both exist to
    // prevent, reached here from the opposite direction.
    //
    // This is deliberately NOT derived from staleness. The reads that cause
    // the defect arrive within milliseconds of the ABORT and are genuinely
    // fresh, so no staleness rule can tell them from a live search.
    setSearchActive: (active: boolean) => {
      // Stamp the moment the search stopped. The held readouts are judged for
      // staleness as of THIS instant rather than as of now — otherwise a
      // search that ended while hearing nothing would revive its last reading
      // the moment the staleness rule stopped applying, and the gauge would
      // climb off zero after the operator let go (TRA-1171).
      set(active ? { searchActive: true } : { searchActive: false, searchEndedAt: Date.now() });
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

      // Once the search has stopped these four are the result the operator is
      // reading — but only if there was one. A search that ended hearing
      // nothing decays here too, judged as of the release (TRA-1171).
      if (signalWasStale(state)) {
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

      // The whole calculation runs at ONE instant: now while the search is
      // running, and the release once it has stopped. Freezing the clock is
      // what freezes the gauge exactly where the operator left it — whether
      // that is -30, -70, or the bottom of the scale (TRA-1171).
      //
      // Freezing only the staleness verdict is not enough: the 500ms window
      // below would go on draining for half a second after the release and
      // the number would drift before settling.
      const readAt = state.searchActive ? Date.now() : state.searchEndedAt;

      // If no readings in the last second, there is no signal to show.
      if (signalWasStale(state)) {
        return DEFAULT_RSSI;
      }

      const window = 500; // 500ms window for filtering
      const cutoff = readAt - window;

      const recentReadings = state.rssiBuffer.filter(p => p.timestamp > cutoff);

      if (recentReadings.length === 0) {
        return state.currentRSSI;
      }

      // Time-weighted average (more recent = higher weight)
      let weightedSum = 0;
      let totalWeight = 0;

      recentReadings.forEach(point => {
        const age = readAt - point.timestamp;
        const weight = 1 - (age / window); // Linear decay
        weightedSum += point.nb_rssi * weight;
        totalWeight += weight;
      });

      return totalWeight > 0 ? Math.round(weightedSum / totalWeight) : state.currentRSSI;
    },

    // "Are we hearing the target tag right now?" — a different question from
    // "what should the gauge show", which may be a held result from a search
    // that is over.
    //
    // These were one signal until TRA-1171, and fusing them was safe only
    // while the display always decayed on its own. Once it holds, a single
    // signal keeps the beeper running forever on a number nobody is still
    // listening to.
    isHearingTag: () => {
      const state = get();
      return state.searchActive && Date.now() - state.lastUpdateTime <= STALE_THRESHOLD_MS;
    }
  }))
);