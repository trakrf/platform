-- TRA-846 — enable the OAuth2 client_credentials + refresh_token grant on the
-- TRA-843 refresh_tokens table. An API integration is not a user, so api-type
-- rows carry api_key_id and no user_id. Relax user_id to nullable and tighten
-- the type-consistency CHECK so session rows require user_id (no api_key_id)
-- and api rows require api_key_id (no user_id).

SET search_path = trakrf, public;

ALTER TABLE refresh_tokens ALTER COLUMN user_id DROP NOT NULL;

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_type_consistent;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_type_consistent CHECK (
    (token_type = 'session' AND user_id IS NOT NULL AND api_key_id IS NULL) OR
    (token_type = 'api'     AND user_id IS NULL     AND api_key_id IS NOT NULL)
);

COMMENT ON COLUMN refresh_tokens.user_id IS 'Owning user for session tokens. NULL for token_type=api (TRA-846): an integration is authenticated by its api_keys row, not a user.';
