export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'scans:read'
  | 'scans:write'
  | 'keys:admin';

export interface APIKey {
  surrogate_id: number;
  jti: string;
  name: string;
  scopes: Scope[];
  created_by: number | null;
  created_by_key_id: number | null;
  created_at: string;
  expires_at?: string | null; // omit-when-unset
  last_used_at: string | null;
}

export interface CreateAPIKeyRequest {
  name: string;
  scopes: Scope[];
  expires_at?: string | null;
}

export interface APIKeyCreateResponse {
  key: string; // full JWT — shown once
  surrogate_id: number;
  name: string;
  scopes: Scope[];
  created_at: string;
  expires_at?: string | null; // omit-when-unset
}

export const ALL_SCOPES: Scope[] = [
  'assets:read',
  'assets:write',
  'locations:read',
  'locations:write',
  'scans:read',
  'scans:write',
  'keys:admin',
];

export const ACTIVE_KEY_CAP = 10;
