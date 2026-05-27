-- TRA-843 — refresh tokens for active-session continuity.
-- Pairs short-lived access JWTs (15 min target) with long-lived rotating
-- refresh tokens (30d). Single-use: each refresh issues a new row and marks
-- the previous one used (replaced_by). Replay of a used token revokes the
-- whole chain — see service-layer refresh logic.

SET search_path = trakrf, public;

-- token_type and api_key_id are baked in at table-creation for forward-compat
-- with the OAuth2 client_credentials + refresh_token grant flow planned for
-- the public API (epic following TRA-843). Session tokens use
-- token_type='session', api_key_id NULL. API tokens (when that flow lands)
-- use token_type='api' with api_key_id pointing at the parent integration.
-- Scopes are NOT stored here — they live on api_keys and are joined at mint
-- time, single source of truth.
CREATE TYPE refresh_token_type AS ENUM ('session', 'api');

CREATE TABLE refresh_tokens (
    id           BIGSERIAL PRIMARY KEY,
    token_type   refresh_token_type NOT NULL DEFAULT 'session',
    user_id      BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
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
    -- Type discriminator invariants: session tokens require user_id and have
    -- no api_key_id; api tokens require api_key_id. Enforced at the DB to
    -- prevent application bugs from inserting mixed rows.
    CONSTRAINT refresh_tokens_type_consistent CHECK (
        (token_type = 'session' AND api_key_id IS NULL) OR
        (token_type = 'api'     AND api_key_id IS NOT NULL)
    )
);

CREATE UNIQUE INDEX idx_refresh_tokens_token_hash ON refresh_tokens(token_hash);
CREATE INDEX idx_refresh_tokens_user_expires ON refresh_tokens(user_id, expires_at);
CREATE INDEX idx_refresh_tokens_api_key ON refresh_tokens(api_key_id) WHERE api_key_id IS NOT NULL;

COMMENT ON TABLE refresh_tokens IS 'Long-lived rotating refresh tokens paired with short-lived access JWTs (TRA-843). token_type discriminates session refresh from the planned OAuth2 API-token grant flow (follow-up epic).';
COMMENT ON COLUMN refresh_tokens.token_hash IS 'SHA-256 hex of the opaque secret returned to the client';
COMMENT ON COLUMN refresh_tokens.used_at IS 'Set when this token is exchanged. Presenting a used token again indicates replay → revoke the whole chain.';
COMMENT ON COLUMN refresh_tokens.replaced_by IS 'Chain pointer to the next token in the rotation lineage. Used to walk a chain on replay-detection.';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Set on logout or chain-revocation. Revoked tokens always reject.';
COMMENT ON COLUMN refresh_tokens.token_type IS 'session = user login session refresh; api = integration access (OAuth2 client_credentials + refresh) — follow-up epic.';
COMMENT ON COLUMN refresh_tokens.api_key_id IS 'For token_type=api: the api_keys row this refresh was minted under. NULL for session tokens.';
