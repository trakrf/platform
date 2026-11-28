/**
 * Location maps API client (TRA-1402).
 *
 * Internal, session-authenticated routes under /api/v1/location-maps (not in
 * the public OpenAPI). The image is fetched as a blob because an <img src>
 * cannot carry the bearer token.
 */

import { apiClient } from '../client';
import type {
  LocationMap,
  LocationMapSummary,
  MapPin,
  MapPresence,
} from '@/types/locationMaps';

interface Envelope<T> {
  data: T;
}

/** Map image upload limit; the backend enforces the same number. */
export const MAX_MAP_IMAGE_BYTES = 5 * 1024 * 1024;
export const MAP_IMAGE_TYPES = ['image/png', 'image/jpeg', 'image/webp'];

/** Strip a FileReader data URL down to its base64 payload. */
export function dataUrlToBase64(dataUrl: string): string {
  const comma = dataUrl.indexOf(',');
  return comma === -1 ? dataUrl : dataUrl.slice(comma + 1);
}

export function fileToBase64(file: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(dataUrlToBase64(String(reader.result)));
    reader.onerror = () => reject(reader.error ?? new Error('Could not read file'));
    reader.readAsDataURL(file);
  });
}

const base = (locationId: number) => `/location-maps/${locationId}`;

export const locationMapsApi = {
  list: () => apiClient.get<Envelope<LocationMapSummary[]>>('/location-maps'),

  get: (locationId: number) => apiClient.get<Envelope<LocationMap>>(base(locationId)),

  getImageBlob: (locationId: number) =>
    apiClient.get<Blob>(`${base(locationId)}/image`, { responseType: 'blob' }),

  putImage: async (locationId: number, file: Blob) =>
    apiClient.put<Envelope<LocationMap>>(`${base(locationId)}/image`, {
      data_base64: await fileToBase64(file),
    }),

  putPins: (locationId: number, pins: MapPin[]) =>
    apiClient.put<Envelope<LocationMap>>(`${base(locationId)}/pins`, { pins }),

  remove: (locationId: number) => apiClient.delete(base(locationId)),

  /** seenWithinSeconds: 0 = no limit. */
  presence: (locationId: number, seenWithinSeconds: number) =>
    apiClient.get<Envelope<MapPresence>>(`${base(locationId)}/presence`, {
      params: { seen_within: seenWithinSeconds },
    }),
};
