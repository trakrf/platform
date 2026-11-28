/**
 * Location maps (TRA-1402): a floor plan or site image on a parent location,
 * its descendant locations pinned on it, and who is in each pinned zone now.
 * Mirrors backend/internal/models/locationmap.
 */

/** A location's position on a map, as a percentage (0–100) of the image. */
export interface MapPin {
  location_id: number;
  x_pct: number;
  y_pct: number;
}

export interface LocationMap {
  location_id: number;
  location_name: string;
  content_type: string;
  byte_size: number;
  sha256: string;
  updated_at: string;
  pins: MapPin[];
}

export interface LocationMapSummary {
  location_id: number;
  location_name: string;
  pin_count: number;
  updated_at: string;
}

/** A person or asset inside the map's subtree. location_id is where it
 *  actually is, which may be below the pin it is drawn at. */
export interface MapOccupant {
  asset_id: number;
  name: string;
  external_key: string;
  is_person: boolean;
  location_id: number;
  location_name: string;
  last_seen: string;
}

export interface MapZoneOccupancy {
  location_id: number;
  name: string;
  x_pct: number;
  y_pct: number;
  occupants: MapOccupant[];
}

export interface MapPresence {
  map_location_id: number;
  generated_at: string;
  zones: MapZoneOccupancy[];
  /** In the map's subtree but under no pin. */
  unplaced: MapOccupant[];
}
