-- TRA-1028 down: rebuild output_device_type without 'csl_cs463_gpo'.
-- PG has no DROP VALUE. If any row still uses the value the USING cast below
-- errors, which is the intended assertion: migrate the rows first.
SET search_path = trakrf, public;

ALTER TABLE output_devices ALTER COLUMN type DROP DEFAULT;
ALTER TYPE output_device_type RENAME TO output_device_type_old;
CREATE TYPE output_device_type AS ENUM ('shelly_gen4');
ALTER TABLE output_devices
    ALTER COLUMN type TYPE output_device_type USING type::text::output_device_type;
ALTER TABLE output_devices ALTER COLUMN type SET DEFAULT 'shelly_gen4';
DROP TYPE output_device_type_old;

COMMENT ON COLUMN output_devices.switch_id IS NULL;
COMMENT ON COLUMN output_devices.command_topic IS
    'TRA-906: Shelly MQTT topic prefix (mqtt transport); backend publishes to <command_topic>/command/switch:<switch_id>.';
