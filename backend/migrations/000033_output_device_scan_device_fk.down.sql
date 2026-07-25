-- TRA-1028 down: drop scan_device_id (comment drops with the column).
SET search_path = trakrf, public;

ALTER TABLE output_devices DROP COLUMN IF EXISTS scan_device_id;
