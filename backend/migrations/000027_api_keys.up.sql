SET search_path=trakrf,public;

-- Sequence for permuted ID generation
CREATE SEQUENCE api_key_seq;

CREATE TABLE api_keys (
    id           INT PRIMARY KEY,
    jti          UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    org_id       INT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name         VARCHAR(255) NOT NULL,
    scopes       TEXT[] NOT NULL,
    created_by   INT NOT NULL REFERENCES users(id),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at   TIMESTAMPTZ,
    last_used_at TIMESTAMPTZ,
    revoked_at   TIMESTAMPTZ
);

-- Permuted id trigger (mirrors assets / locations convention)
CREATE TRIGGER generate_api_key_id_trigger
    BEFORE INSERT ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('api_key_seq');

-- Partial index for the dominant UI query ("active keys for this org")
CREATE INDEX idx_api_keys_active_by_org
    ON api_keys(org_id)
    WHERE revoked_at IS NULL;

-- Lookup by jti (UNIQUE constraint already creates an index; this is explicit)
CREATE INDEX idx_api_keys_jti ON api_keys(jti);

-- No RLS on api_keys. See migration 000020 (users / org_users) for precedent:
-- the middleware must read this table BEFORE app.current_org_id is set, and
-- our DB user lacks BYPASSRLS on TimescaleDB Cloud. Org isolation is enforced
-- at the application layer — every storage method in storage/apikeys.go takes
-- orgID explicitly and WHERE-clauses on it.

COMMENT ON TABLE  api_keys IS 'API keys for public API authentication (TRA-393)';
COMMENT ON COLUMN api_keys.jti IS 'JWT ID — revocation handle referenced by api_key JWTs';
COMMENT ON COLUMN api_keys.scopes IS 'Subset of: assets:read, assets:write, locations:read, locations:write, scans:read';
COMMENT ON COLUMN api_keys.expires_at IS 'NULL means never expires';
COMMENT ON COLUMN api_keys.revoked_at IS 'NULL means active';
