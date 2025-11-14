SET search_path=trakrf,public;

CREATE EXTENSION IF NOT EXISTS ltree;

ALTER TABLE locations ADD COLUMN path ltree;

-- Backfill existing locations before adding NOT NULL constraint
-- Root locations (no parent)
UPDATE locations
SET path = text2ltree(REPLACE(LOWER(identifier), '-', '_'))
WHERE parent_location_id IS NULL AND path IS NULL;

-- Child locations (recursive update by depth)
WITH RECURSIVE location_hierarchy AS (
    -- Root locations already have paths
    SELECT id, identifier, parent_location_id, path, 1 as level
    FROM locations
    WHERE parent_location_id IS NULL

    UNION ALL

    -- Child locations
    SELECT l.id, l.identifier, l.parent_location_id,
           lh.path || text2ltree(REPLACE(LOWER(l.identifier), '-', '_')) as path,
           lh.level + 1
    FROM locations l
    INNER JOIN location_hierarchy lh ON l.parent_location_id = lh.id
    WHERE l.path IS NULL
)
UPDATE locations l
SET path = lh.path
FROM location_hierarchy lh
WHERE l.id = lh.id AND l.path IS NULL;

ALTER TABLE locations ADD COLUMN depth INT GENERATED ALWAYS AS (nlevel(path)) STORED;

CREATE OR REPLACE FUNCTION update_location_path()
RETURNS TRIGGER AS $$
DECLARE
    parent_path ltree;
    sanitized_identifier text;
BEGIN
    sanitized_identifier := REPLACE(LOWER(NEW.identifier), '-', '_');

    IF NEW.parent_location_id IS NULL THEN
        NEW.path = text2ltree(sanitized_identifier);
    ELSE
        SELECT path INTO parent_path
        FROM locations
        WHERE id = NEW.parent_location_id;

        IF parent_path IS NULL THEN
            RAISE EXCEPTION 'Parent location % has no path', NEW.parent_location_id;
        END IF;

        NEW.path = parent_path || text2ltree(sanitized_identifier);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER maintain_location_path
    BEFORE INSERT OR UPDATE OF parent_location_id, identifier
    ON locations
    FOR EACH ROW
    EXECUTE FUNCTION update_location_path();

ALTER TABLE locations ALTER COLUMN path SET NOT NULL;

ALTER TABLE locations ADD CONSTRAINT no_self_reference
    CHECK (id != parent_location_id);

CREATE INDEX locations_path_gist_idx ON locations USING GIST (path);

CREATE INDEX locations_depth_idx ON locations(depth);
