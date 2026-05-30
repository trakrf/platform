-- TRA-720 — bulk_import_jobs + api_keys + refresh_tokens.
-- bulk_import_jobs: tags_created counter (post-000025), corrected valid_row_counts
-- (post-000026). api_keys: self-FK created_by_key_id (post-000029), secret_hash
-- (TRA-847). RLS on bulk_import_jobs only — api_keys is app-layer enforced (auth
-- reads before session GUC is set; per legacy 000027 comment).
-- refresh_tokens (TRA-843/846) lives here rather than with the other auth tables
-- in 000003 because it FKs api_keys.
-- Folded in (flatten of post-TRA-720 incrementals): former migrations 000011
-- (refresh_tokens), 000012 (refresh_tokens api grant), 000013 (api_keys
-- secret_hash). Their one-time DELETE data-cleanup steps are omitted — a
-- from-scratch build has no rows to clean.

SET search_path = trakrf, public;

-- ============================================================================
-- bulk_import_jobs
-- ============================================================================
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
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.tags_created IS 'Number of tag rows created by this job (post-000025)';

-- ============================================================================
-- api_keys
-- ============================================================================
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
    -- TRA-847 (folded in from migration 000013): api keys authenticate via an
    -- opaque client_secret (hashed), not a long-lived JWT.
    secret_hash         VARCHAR(64) NOT NULL,
    CONSTRAINT api_keys_creator_exactly_one
        CHECK ((created_by IS NOT NULL) <> (created_by_key_id IS NOT NULL))
);

CREATE TRIGGER generate_api_key_id_trigger
    BEFORE INSERT ON api_keys
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE INDEX idx_api_keys_active_by_org
    ON api_keys(org_id) WHERE revoked_at IS NULL;
CREATE INDEX idx_api_keys_jti ON api_keys(jti);

COMMENT ON TABLE api_keys IS 'API keys for public API authentication';
COMMENT ON COLUMN api_keys.jti IS 'JWT ID — revocation handle referenced by api_key JWTs';
COMMENT ON COLUMN api_keys.created_by IS 'User who minted this key via session auth. Mutually exclusive with created_by_key_id.';
COMMENT ON COLUMN api_keys.created_by_key_id IS 'Parent API key that minted this key via keys:admin scope. Mutually exclusive with created_by.';
COMMENT ON COLUMN api_keys.scopes IS 'Subset of: assets:read, assets:write, locations:read, locations:write, tracking:read, scans:write, keys:admin';
COMMENT ON COLUMN api_keys.secret_hash IS 'SHA-256 hex of the opaque client_secret shown once at creation (TRA-847). The plaintext secret is never stored.';

-- ============================================================================
-- refresh_tokens (folded in from migrations 000011/TRA-843 + 000012/TRA-846)
-- Long-lived rotating refresh tokens paired with short-lived access JWTs.
-- Single-use: each refresh issues a new row and marks the previous used
-- (replaced_by); replay of a used token revokes the whole chain. token_type
-- discriminates user-session refresh from the OAuth2 client_credentials +
-- refresh_token grant for API integrations. Defined here (not with the other
-- auth tables in 000003) because it FKs api_keys, created just above.
-- Reflects the post-000012 shape: user_id nullable, type-consistency CHECK
-- requires user_id for session rows and api_key_id for api rows.
-- ============================================================================
CREATE TYPE refresh_token_type AS ENUM ('session', 'api');

CREATE TABLE refresh_tokens (
    id           BIGSERIAL PRIMARY KEY,
    token_type   refresh_token_type NOT NULL DEFAULT 'session',
    user_id      BIGINT REFERENCES users(id) ON DELETE CASCADE,
    org_id       BIGINT REFERENCES organizations(id) ON DELETE CASCADE,
    api_key_id   BIGINT REFERENCES api_keys(id) ON DELETE CASCADE,
    token_hash   VARCHAR(64) NOT NULL,
    user_agent   TEXT,
    ip           INET,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ NOT NULL,
    used_at      TIMESTAMPTZ,
    replaced_by  BIGINT REFERENCES refresh_tokens(id) ON DELETE SET NULL,
    revoked_at   TIMESTAMPTZ,
    -- Type discriminator invariants (post-TRA-846): session tokens require
    -- user_id and have no api_key_id; api tokens require api_key_id and have no
    -- user_id. Enforced at the DB to prevent mixed rows.
    CONSTRAINT refresh_tokens_type_consistent CHECK (
        (token_type = 'session' AND user_id IS NOT NULL AND api_key_id IS NULL) OR
        (token_type = 'api'     AND user_id IS NULL     AND api_key_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_expires ON refresh_tokens(user_id, expires_at);
CREATE INDEX idx_refresh_tokens_api_key ON refresh_tokens(api_key_id) WHERE api_key_id IS NOT NULL;

COMMENT ON TABLE refresh_tokens IS 'Long-lived rotating refresh tokens paired with short-lived access JWTs (TRA-843). token_type discriminates session refresh from the OAuth2 API-token grant flow (TRA-846).';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hex of the opaque secret returned to the client';
COMMENT ON COLUMN refresh_tokens.used_at IS 'Set when this token is exchanged. Presenting a used token again indicates replay → revoke the whole chain.';
COMMENT ON COLUMN refresh_tokens.replaced_by IS 'Chain pointer to the next token in the rotation lineage. Used to walk a chain on replay-detection.';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Set on logout or chain-revocation. Revoked tokens always reject.';
COMMENT ON COLUMN refresh_tokens.token_type IS 'session = user login session refresh; api = integration access (OAuth2 client_credentials + refresh, TRA-846).';
COMMENT ON COLUMN refresh_tokens.user_id IS 'Owning user for session tokens. NULL for token_type=api (TRA-846): an integration is authenticated by its api_keys row, not a user.';
COMMENT ON COLUMN refresh_tokens.api_key_id IS 'For token_type=api: the api_keys row this refresh was minted under. NULL for session tokens.';
