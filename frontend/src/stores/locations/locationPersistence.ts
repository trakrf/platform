import type { StateCreator } from 'zustand';
import type { LocationStore } from './locationStore';

const STORAGE_KEY = 'location-metadata';

interface LocationMetadata {
  allIdentifiers: string[];
  lastFetched: number;
}

export function createLocationPersistence(
  stateCreator: StateCreator<LocationStore>
) {
  return (set: any, get: any, api: any) => {
    let initialIdentifiers: string[] = [];

    if (typeof window !== 'undefined' && window.localStorage) {
      try {
        const stored = localStorage.getItem(STORAGE_KEY);
        if (stored) {
          const metadata: LocationMetadata = JSON.parse(stored);
          initialIdentifiers = metadata.allIdentifiers || [];
        }
      } catch (error) {
        console.error('[LocationStore] Failed to load metadata:', error);
      }
    }

    const baseState = stateCreator(set, get, api);

    return {
      ...baseState,
      cache: {
        ...baseState.cache,
        allIdentifiers: initialIdentifiers,
      },
    };
  };
}
