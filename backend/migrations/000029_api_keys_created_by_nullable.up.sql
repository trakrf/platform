SET search_path=trakrf,public;

ALTER TABLE api_keys ALTER COLUMN created_by DROP NOT NULL;

ALTER TABLE api_keys
    ADD COLUMN created_by_key_id INT REFERENCES api_keys(id);

ALTER TABLE api_keys
    ADD CONSTRAINT api_keys_creator_exactly_one
    CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL));

COMMENT ON COLUMN api_keys.created_by IS
    'User who minted this key via session auth. Mutually exclusive with created_by_key_id.';
COMMENT ON COLUMN api_keys.created_by_key_id IS
    'Parent API key that minted this key via keys:admin scope. Mutually exclusive with created_by.';

-- Refresh stale scope enumeration (existing comment predates scans:write and keys:admin).
COMMENT ON COLUMN api_keys.scopes IS
    'Subset of ValidScopes in models/apikey: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
