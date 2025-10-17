SET search_path=trakrf,public;

-- Drop hypertable (automatically drops chunks)
DROP TABLE IF EXISTS events CASCADE;
