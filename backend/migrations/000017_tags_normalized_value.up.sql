-- TRA-944 — leading-zero / case-insensitive hex normalization for tag->asset
-- resolution during ingestion. Mirrors the handheld getMatchingKey
-- (frontend/src/utils/reconciliationUtils.ts): uppercase, strip non-hex
-- [^0-9A-F], then strip leading zeros keeping >=1 digit (^0+(?=[0-9])). A reader
-- emits the full-width EPC (000000000000000000010023) while tags are often
-- registered by the short barcode value (10023); without this they never match
-- and the geofence/alarm never fires. Domain is all-hex (UHF EPC + BLE MAC), so
-- a leading-zero trim is safe across the board.
SET search_path = trakrf, public;

-- Single source of truth for the normalization, shared by the generated column
-- below and the ingest membership query (storage/ingest.go), so the two can
-- never drift. SQL (not plpgsql) + IMMUTABLE so it's usable in a generated
-- column and inlinable by the planner on the per-read query path.
CREATE FUNCTION normalize_tag_value(v text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT regexp_replace(regexp_replace(upper(v), '[^0-9A-F]', '', 'g'), '^0+(?=[0-9])', '')
    $$;

COMMENT ON FUNCTION normalize_tag_value(text) IS 'TRA-944: hex tag-value match key (uppercase, strip non-hex, strip leading zeros keeping >=1 digit); mirrors handheld getMatchingKey. Shared by tags.normalized_value and the ingest query.';

ALTER TABLE tags ADD COLUMN normalized_value text
    GENERATED ALWAYS AS (normalize_tag_value(value)) STORED;

-- Match path is (org_id, normalized_value) restricted to live, asset-linked tags.
CREATE INDEX idx_tags_normalized_value ON tags (org_id, normalized_value)
    WHERE asset_id IS NOT NULL AND deleted_at IS NULL;

COMMENT ON COLUMN tags.normalized_value IS 'TRA-944: leading-zero/case-insensitive hex key for ingestion tag->asset matching; mirrors handheld getMatchingKey. Generated, do not write directly.';
