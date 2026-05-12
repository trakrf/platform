SET search_path=trakrf,public;

UPDATE api_keys
   SET scopes = array_replace(scopes, 'tracking:read', 'history:read')
 WHERE 'tracking:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, history:read, scans:write, keys:admin';
