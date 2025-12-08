SET search_path=trakrf,public;

DROP INDEX IF EXISTS locations_depth_idx;
DROP INDEX IF EXISTS locations_path_gist_idx;

ALTER TABLE locations DROP CONSTRAINT IF EXISTS no_self_reference;

DROP TRIGGER IF EXISTS maintain_location_path ON locations;
DROP FUNCTION IF EXISTS update_location_path();

ALTER TABLE locations DROP COLUMN IF EXISTS depth;
ALTER TABLE locations DROP COLUMN IF EXISTS path;

DROP EXTENSION IF EXISTS ltree CASCADE;
