-- Drop location maps. Policies, triggers and indexes drop with the tables.
SET search_path = trakrf, public;

DROP TABLE IF EXISTS location_map_pins;
DROP TABLE IF EXISTS location_maps;
