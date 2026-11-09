-- Drop the output drive log. Policy and indexes drop with the table.
SET search_path = trakrf, public;

DROP TABLE IF EXISTS output_drives;
