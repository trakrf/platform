SET search_path=trakrf,public;

-- TRA-682: rename history:read → tracking:read. The scope gates both
-- /assets/{asset_id}/history (time-series) AND /reports/asset-locations
-- (current-state snapshot). The previous name implied "historical data
-- only" and misdirected integrators scoping a key for live tracking.
-- Pre-launch hard cut; existing preview keys are rewritten in place.
UPDATE api_keys
   SET scopes = array_replace(scopes, 'history:read', 'tracking:read')
 WHERE 'history:read' = ANY(scopes);

COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, tracking:read, scans:write, keys:admin';
