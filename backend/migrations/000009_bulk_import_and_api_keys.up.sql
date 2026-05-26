-- TRA-720 — bulk_import_jobs + api_keys.
-- bulk_import_jobs: tags_created counter (post-000025), corrected valid_row_counts
-- (post-000026). api_keys: self-FK created_by_key_id (post-000029). RLS on
-- bulk_import_jobs only — api_keys is app-layer enforced (auth reads before
-- session GUC is set; per legacy 000027 comment).

SET search_path = trakrf, public;

-- ============================================================================
-- bulk_import_jobs
-- ============================================================================
CREATE SEQUENCE bulk_import_job_seq AS BIGINT;

CREATE TABLE bulk_import_jobs (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    status          TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows      INT NOT NULL DEFAULT 0,
    processed_rows  INT NOT NULL DEFAULT 0,
    failed_rows     INT NOT NULL DEFAULT 0,
    tags_created    INT NOT NULL DEFAULT 0,
    errors          JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at    TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (
        processed_rows >= 0 AND failed_rows >= 0
        AND processed_rows + failed_rows <= total_rows
    )
);

CREATE INDEX idx_bulk_import_jobs_org_id ON bulk_import_jobs(org_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);

CREATE TRIGGER generate_bulk_import_job_id_trigger
    BEFORE INSERT ON bulk_import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('bulk_import_job_seq');

ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.tags_created IS 'Number of tag rows created by this job (post-000025)';

-- ============================================================================
-- api_keys
-- ============================================================================
CREATE SEQUENCE api_key_seq AS BIGINT;

CREATE TABLE api_keys (
    id                  BIGINT PRIMARY KEY,
    jti                 UUID NOT NULL UNIQUE DEFAULT gen_random_uuid(),
    org_id              BIGINT NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name                VARCHAR(255) NOT NULL,
    scopes              TEXT[] NOT NULL,
    created_by          BIGINT REFERENCES users(id),
    created_by_key_id   BIGINT REFERENCES api_keys(id),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    expires_at          TIMESTAMPTZ,
    last_used_at        TIMESTAMPTZ,
    revoked_at          TIMESTAMPTZ,
    CONSTRAINT api_keys_creator_exactly_one
        CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL))
);

CREATE TRIGGER generate_api_key_id_trigger
    BEFORE INSERT ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('api_key_seq');

CREATE INDEX idx_api_keys_active_by_org
    ON api_keys(org_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_jti ON api_keys(jti);

COMMENT ON TABLE api_keys IS 'API keys for public API authentication';
COMMENT ON COLUMN api_keys.jti IS 'JWT ID — revocation handle referenced by api_key JWTs';
COMMENT ON COLUMN api_keys.created_by IS 'User who minted this key via session auth. Mutually exclusive with created_by_key_id.';
COMMENT ON COLUMN api_keys.created_by_key_id IS 'Parent API key that minted this key via keys:admin scope. Mutually exclusive with created_by.';
COMMENT ON COLUMN api_keys.scopes IS 'Subset of: assets:read, assets:write, locations:read, locations:write, scans:read, scans:write, keys:admin';
