SET search_path=trakrf,public;

UPDATE locations
SET identifier = REPLACE(identifier, '-', '_')
WHERE identifier LIKE '%-%';
