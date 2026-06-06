SET search_path = trakrf, public;

DROP INDEX IF EXISTS idx_tags_normalized_value;
ALTER TABLE tags DROP COLUMN IF EXISTS normalized_value;
