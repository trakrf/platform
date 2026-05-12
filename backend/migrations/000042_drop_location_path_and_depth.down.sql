SET search_path = trakrf,public;

-- TRA-684 down: restore locations.path (ltree), locations.depth (generated),
-- triggers, and indexes. The recompute helper is recreated so callers can
-- rebuild paths after the column comes back. Mirrors migrations 000018 +
-- 000036 + 000038 as a single restore step.

CREATE EXTENSION IF NOT EXISTS ltree;

ALTER TABLE trakrf.locations ADD COLUMN path ltree;

-- Restore canonical paths (lower(replace(external_key, '-', '_'))) by
-- recursing from roots. Mirrors recompute_location_paths()'s behaviour
-- before the function itself is recreated.
WITH RECURSIVE canonical AS (
    SELECT id, parent_location_id,
           text2ltree(replace(lower(external_key), '-', '_')) AS new_path
    FROM trakrf.locations
    WHERE parent_location_id IS NULL

    UNION ALL

    SELECT l.id, l.parent_location_id,
           c.new_path || text2ltree(replace(lower(l.external_key), '-', '_'))
    FROM trakrf.locations l
    JOIN canonical c ON l.parent_location_id = c.id
)
UPDATE trakrf.locations l
SET path = c.new_path
FROM canonical c
WHERE l.id = c.id;

ALTER TABLE trakrf.locations ALTER COLUMN path SET NOT NULL;
ALTER TABLE trakrf.locations ADD COLUMN depth INT GENERATED ALWAYS AS (nlevel(path)) STORED;

CREATE INDEX locations_path_gist_idx ON trakrf.locations USING GIST (path);
CREATE INDEX locations_depth_idx ON trakrf.locations(depth);

CREATE OR REPLACE FUNCTION trakrf.update_location_path()
RETURNS TRIGGER AS $$
DECLARE
    parent_path ltree;
    sanitized_external_key text;
BEGIN
    sanitized_external_key := REPLACE(LOWER(NEW.external_key), '-', '_');

    IF NEW.parent_location_id IS NULL THEN
        NEW.path = text2ltree(sanitized_external_key);
    ELSE
        SELECT path INTO parent_path
        FROM trakrf.locations
        WHERE id = NEW.parent_location_id;

        IF parent_path IS NULL THEN
            RAISE EXCEPTION 'Parent location % has no path', NEW.parent_location_id;
        END IF;

        NEW.path = parent_path || text2ltree(sanitized_external_key);
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER maintain_location_path
    BEFORE INSERT OR UPDATE OF parent_location_id, external_key
    ON trakrf.locations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_location_path();

CREATE OR REPLACE FUNCTION trakrf.cascade_location_path() RETURNS TRIGGER AS $$
BEGIN
    UPDATE trakrf.locations
    SET path = NEW.path || subpath(path, nlevel(OLD.path))
    WHERE path <@ OLD.path
      AND id != NEW.id;
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER cascade_location_path_change
    AFTER UPDATE OF parent_location_id, external_key
    ON trakrf.locations
    FOR EACH ROW
    WHEN (OLD.path IS DISTINCT FROM NEW.path)
    EXECUTE FUNCTION trakrf.cascade_location_path();

CREATE OR REPLACE FUNCTION trakrf.recompute_location_paths() RETURNS INT AS $$
DECLARE
    rows_updated INT;
BEGIN
    WITH RECURSIVE canonical AS (
        SELECT id, parent_location_id,
               text2ltree(replace(lower(external_key), '-', '_')) AS new_path
        FROM trakrf.locations
        WHERE parent_location_id IS NULL

        UNION ALL

        SELECT l.id, l.parent_location_id,
               c.new_path || text2ltree(replace(lower(l.external_key), '-', '_'))
        FROM trakrf.locations l
        JOIN canonical c ON l.parent_location_id = c.id
    )
    UPDATE trakrf.locations l
    SET path = c.new_path
    FROM canonical c
    WHERE l.id = c.id
      AND l.path IS DISTINCT FROM c.new_path;

    GET DIAGNOSTICS rows_updated = ROW_COUNT;
    RETURN rows_updated;
END;
$$ LANGUAGE plpgsql;
