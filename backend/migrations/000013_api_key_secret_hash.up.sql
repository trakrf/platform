-- TRA-847: api keys now authenticate via an opaque client_secret (hashed),
-- not a long-lived JWT. Pre-existing rows have no recoverable secret and are
-- unusable under the new model, so they are removed (0 live prod keys per the
-- DB audit). Refresh tokens referencing them are removed first (FK).

SET search_path = trakrf, public;

DELETE FROM refresh_tokens
WHERE api_key_id IS NOT NULL;

DELETE FROM api_keys;

ALTER TABLE api_keys
    ADD COLUMN secret_hash VARCHAR(64) NOT NULL;

COMMENT ON COLUMN api_keys.secret_hash IS 'SHA-256 hex of the opaque client_secret shown once at creation (TRA-847). The plaintext secret is never stored.';
