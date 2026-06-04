-- TRA-903 down: drop alarm output devices. Policy, indexes and triggers drop
-- with the table; the enum is dropped after.
SET search_path = trakrf, public;

DROP TABLE IF EXISTS alarm_devices;
DROP TYPE IF EXISTS alarm_device_type;
