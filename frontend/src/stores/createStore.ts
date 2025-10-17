/**
 * Helper to create Zustand stores with OpenReplay tracking
 */
import { StateCreator } from 'zustand';
import { getZustandPlugin } from '@/lib/openreplay';

/**
 * Creates a Zustand store with OpenReplay tracking if available
 * @param createState - The state creator function
 * @param storeName - Optional name for the store (used in OpenReplay)
 * @returns The wrapped state creator or original if OpenReplay is disabled
 */
export function createStoreWithTracking<T>(
  createState: StateCreator<T>,
  storeName?: string
): StateCreator<T> {
  const zustandPlugin = getZustandPlugin();
  
  if (zustandPlugin && typeof zustandPlugin === 'function') {
    // Wrap the store with OpenReplay tracking
    console.log(`Creating store with OpenReplay tracking: ${storeName || 'unnamed'}`);
    // Type assertion needed because zustandPlugin type is not exported from library
    return (zustandPlugin as (createState: StateCreator<T>, storeName?: string) => StateCreator<T>)(createState, storeName);
  }
  
  // Return the original state creator if plugin is not available
  return createState;
}