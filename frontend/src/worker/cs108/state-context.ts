/**
 * State Context Interface
 *
 * Provides access to reader state management for CommandManager
 * to handle state transitions during command execution.
 */

import type { ReaderStateType } from '../types/reader.js';

/**
 * State context passed to CommandManager for managing reader state transitions
 */
export interface StateContext {
  /**
   * Get current reader state
   */
  getReaderState: () => ReaderStateType;

  /**
   * Set reader state
   * @param state New reader state
   */
  setReaderState: (state: ReaderStateType) => void;
}