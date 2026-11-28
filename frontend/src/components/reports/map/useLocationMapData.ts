/**
 * Data for the location map tab (TRA-1402): the org's maps, the selected
 * map and its image, and live presence polled every POLL_MS.
 */

import { useCallback, useEffect, useState } from 'react';
import { locationMapsApi } from '@/lib/api/locationMaps';
import type { LocationMap, LocationMapSummary, MapPresence } from '@/types/locationMaps';

export const POLL_MS = 5000;

export function useLocationMapData(selectedId: number | null, seenWithinSeconds: number, paused: boolean) {
  const [maps, setMaps] = useState<LocationMapSummary[]>([]);
  const [mapsLoaded, setMapsLoaded] = useState(false);
  const [map, setMap] = useState<LocationMap | null>(null);
  const [imageUrl, setImageUrl] = useState<string | null>(null);
  const [presence, setPresence] = useState<MapPresence | null>(null);
  const [presenceError, setPresenceError] = useState(false);

  const reloadMaps = useCallback(async () => {
    try {
      const { data } = await locationMapsApi.list();
      setMaps(data.data);
    } catch {
      setMaps([]);
    } finally {
      setMapsLoaded(true);
    }
  }, []);

  useEffect(() => {
    void reloadMaps();
  }, [reloadMaps]);

  // Selected map metadata.
  const reloadMap = useCallback(async () => {
    if (selectedId == null) {
      setMap(null);
      return;
    }
    try {
      const { data } = await locationMapsApi.get(selectedId);
      setMap(data.data);
    } catch {
      setMap(null);
    }
  }, [selectedId]);

  useEffect(() => {
    setPresence(null);
    void reloadMap();
  }, [reloadMap]);

  // Image: refetch only when the image itself changes (sha256).
  const mapId = map?.location_id ?? null;
  const sha = map?.sha256 ?? null;
  useEffect(() => {
    if (mapId == null || !sha) {
      setImageUrl(null);
      return;
    }
    let url: string | null = null;
    let cancelled = false;
    (async () => {
      try {
        const { data } = await locationMapsApi.getImageBlob(mapId);
        if (cancelled) return;
        url = URL.createObjectURL(data);
        setImageUrl(url);
      } catch {
        if (!cancelled) setImageUrl(null);
      }
    })();
    return () => {
      cancelled = true;
      if (url) URL.revokeObjectURL(url);
    };
  }, [mapId, sha]);

  // Live presence.
  useEffect(() => {
    if (mapId == null || paused) return;
    let cancelled = false;
    const tick = async () => {
      if (typeof document !== 'undefined' && document.hidden) return;
      try {
        const { data } = await locationMapsApi.presence(mapId, seenWithinSeconds);
        if (!cancelled) {
          setPresence(data.data);
          setPresenceError(false);
        }
      } catch {
        if (!cancelled) setPresenceError(true);
      }
    };
    void tick();
    const timer = setInterval(tick, POLL_MS);
    return () => {
      cancelled = true;
      clearInterval(timer);
    };
  }, [mapId, seenWithinSeconds, paused, map?.pins]);

  return { maps, mapsLoaded, reloadMaps, map, setMap, reloadMap, imageUrl, presence, presenceError };
}
