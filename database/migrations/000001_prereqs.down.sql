-- Drop functions and schema in reverse order
DROP FUNCTION IF EXISTS trakrf.update_updated_at_column() CASCADE;
DROP FUNCTION IF EXISTS trakrf.generate_permuted_id() CASCADE;
DROP FUNCTION IF EXISTS generate_hashed_id() CASCADE;
DROP SCHEMA IF EXISTS trakrf CASCADE;
DROP EXTENSION IF EXISTS timescaledb CASCADE;
