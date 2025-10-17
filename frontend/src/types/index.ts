/**
 * Common type definitions for the TrakRF Handheld application
 */

// Re-export commonly used types from stores
export type { TagInfo } from '@/stores/tagStore';
export type { TabType } from '@/stores/uiStore';

// Re-export RFID constants and types  
export { ReaderState, type ReaderStateType } from '@/worker/types/reader';