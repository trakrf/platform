SET search_path=trakrf,public;

ALTER TABLE locations DISABLE TRIGGER maintain_location_path;

UPDATE locations
SET identifier = REPLACE(identifier, '-', '_')
WHERE identifier LIKE '%-%';

UPDATE locations
SET path = text2ltree(REPLACE(LOWER(identifier), '-', '_'))
WHERE parent_location_id IS NULL AND (path IS NULL OR path::text = '');

WITH RECURSIVE location_hierarchy AS (
    SELECT id, identifier, parent_location_id, path, 1 as level
    FROM locations
    WHERE parent_location_id IS NULL AND path IS NOT NULL

    UNION ALL

    SELECT l.id, l.identifier, l.parent_location_id,
           lh.path || text2ltree(REPLACE(LOWER(l.identifier), '-', '_')) as path,
           lh.level + 1
    FROM locations l
    INNER JOIN location_hierarchy lh ON l.parent_location_id = lh.id
    WHERE l.path IS NULL OR l.path::text = ''
)
UPDATE locations l
SET path = lh.path
FROM location_hierarchy lh
WHERE l.id = lh.id;

ALTER TABLE locations ENABLE TRIGGER maintain_location_path;
