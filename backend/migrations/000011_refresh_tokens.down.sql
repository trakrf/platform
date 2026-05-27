-- TRA-843 — drop refresh_tokens.

SET search_path = trakrf, public;

DROP TABLE IF EXISTS refresh_tokens;
DROP TYPE  IF EXISTS refresh_token_type;
