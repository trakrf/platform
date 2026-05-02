SET search_path=trakrf,public;

-- TRA-578: rename scans:read → history:read so the scope vocabulary aligns
-- with /assets/{id}/history and /locations/current rather than a non-existent
-- /scans resource. Hard cut: pre-launch, no production keys exist.
UPDATE api_keys
   SET scopes = array_replace(scopes, 'scans:read', 'history:read')
 WHERE 'scans:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, history:read, scans:write, keys:admin';
