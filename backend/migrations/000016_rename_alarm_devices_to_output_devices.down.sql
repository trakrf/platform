-- TRA-929 down: rename output_devices back to alarm_devices. Exact reverse of
-- the up migration; all data and associations are preserved.
SET search_path = trakrf, public;

ALTER POLICY org_isolation_output_devices ON output_devices RENAME TO org_isolation_alarm_devices;

ALTER TRIGGER update_output_devices_updated_at  ON output_devices RENAME TO update_alarm_devices_updated_at;
ALTER TRIGGER generate_output_device_id_trigger ON output_devices RENAME TO generate_alarm_device_id_trigger;

ALTER INDEX idx_output_devices_location RENAME TO idx_alarm_devices_location;
ALTER INDEX idx_output_devices_org      RENAME TO idx_alarm_devices_org;

ALTER TABLE output_devices RENAME CONSTRAINT output_devices_location_id_fkey TO alarm_devices_location_id_fkey;
ALTER TABLE output_devices RENAME CONSTRAINT output_devices_org_id_fkey      TO alarm_devices_org_id_fkey;
ALTER TABLE output_devices RENAME CONSTRAINT output_devices_pkey             TO alarm_devices_pkey;

ALTER TABLE output_devices RENAME TO alarm_devices;

ALTER TYPE output_transport   RENAME TO alarm_transport;
ALTER TYPE output_device_type RENAME TO alarm_device_type;
