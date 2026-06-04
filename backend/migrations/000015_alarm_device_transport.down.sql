-- TRA-906 down: drop the transport columns and enum.
SET search_path = trakrf, public;

ALTER TABLE alarm_devices
    DROP COLUMN IF EXISTS command_topic,
    DROP COLUMN IF EXISTS transport;

DROP TYPE IF EXISTS alarm_transport;
