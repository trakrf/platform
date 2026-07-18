export type Scope =
  | 'assets:read'
  | 'assets:write'
  | 'locations:read'
  | 'locations:write'
  | 'tracking:read'
  | 'keys:admin';

export interface APIKey {
  id: number;
  jti: string;
  name: string;
  scopes: Scope[];
  created_by: number | null;
  created_by_key_id: number | null;
  created_at: string;
  expires_at: string | null;
  last_used_at: string | null;
}

export interface CreateAPIKeyRequest {
  name: string;
  scopes: Scope[];
  expires_at?: string | null;
}

export interface APIKeyCreateResponse {
  // OAuth2 client_credentials pair, returned once at creation (TRA-210).
  // `client_id` is the row's jti; `client_secret` is the opaque secret shown
  // exactly once. Both are exchanged at POST /oauth/token for an access token.
  client_id: string;
  client_secret: string;
  id: number;
  name: string;
  scopes: Scope[];
  created_at: string;
  expires_at: string | null;
}

export const ALL_SCOPES: Scope[] = [
  'assets:read',
  'assets:write',
  'locations:read',
  'locations:write',
  'tracking:read',
  'keys:admin',
];

export const ACTIVE_KEY_CAP = 10;
