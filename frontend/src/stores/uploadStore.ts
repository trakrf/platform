import { create } from 'zustand';

/**
 * Global upload store
 *
 * Tracks the active bulk upload job ID for global progress monitoring.
 * When a CSV upload starts, the job ID is stored here and the GlobalUploadAlert
 * component polls the backend for status updates.
 */

interface UploadStore {
  /**
   * Active job ID being tracked (null if no active job)
   */
  activeJobId: string | null;

  /**
   * Set the active job ID to start tracking
   */
  setActiveJobId: (jobId: string | null) => void;

  /**
   * Clear the active job ID (job completed or dismissed)
   */
  clearActiveJobId: () => void;
}

export const useUploadStore = create<UploadStore>((set) => ({
  activeJobId: null,

  setActiveJobId: (jobId) =>
    set({
      activeJobId: jobId,
    }),

  clearActiveJobId: () =>
    set({
      activeJobId: null,
    }),
}));
