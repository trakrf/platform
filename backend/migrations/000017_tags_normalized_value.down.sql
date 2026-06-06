SET search_path = trakrf, public;

DROP INDEX IF EXISTS idx_tags_normalized_value;
-- Drop the column before the function it depends on.
ALTER TABLE tags DROP COLUMN IF EXISTS normalized_value;
DROP FUNCTION IF EXISTS normalize_tag_value(text);
