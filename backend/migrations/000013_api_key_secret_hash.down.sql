SET search_path = trakrf, public;

ALTER TABLE api_keys
    DROP COLUMN secret_hash;
