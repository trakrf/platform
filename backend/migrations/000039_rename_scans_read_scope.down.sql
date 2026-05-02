SET search_path=trakrf,public;

UPDATE api_keys
   SET scopes = array_replace(scopes, 'history:read', 'scans:read')
 WHERE 'history:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
