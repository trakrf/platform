-- Reverts TRA-846. Restoring NOT NULL only succeeds if no api rows exist
-- (api rows have NULL user_id); intended for dev rollback before any api
-- tokens are minted.

SET search_path = trakrf, public;

ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_type_consistent;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_type_consistent CHECK (
    (token_type = 'session' AND api_key_id IS NULL) OR
    (token_type = 'api'     AND api_key_id IS NOT NULL)
);

ALTER TABLE refresh_tokens ALTER COLUMN user_id SET NOT NULL;
