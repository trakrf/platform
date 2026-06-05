-- TRA-929 — rename the alarm_device entity to output_device end to end. The
-- Shelly relay is a general actuator (lift gates, roller doors, shuttle-lot
-- exits, strobes), not specifically an alarm. Pure rename, no behavior change:
-- ALTER ... RENAME keeps all rows and the 1:1 location association intact. The
-- entity is brand new (TRA-903/906) and in zero external integrations, so this
-- window is the cheapest the rename will ever be.
SET search_path = trakrf, public;

-- Enum types
ALTER TYPE alarm_device_type RENAME TO output_device_type;
ALTER TYPE alarm_transport   RENAME TO output_transport;

-- Table
ALTER TABLE alarm_devices RENAME TO output_devices;

-- Primary key + foreign key constraints. Postgres keeps the old auto-generated
-- constraint names on a table rename, so rename them explicitly to match.
ALTER TABLE output_devices RENAME CONSTRAINT alarm_devices_pkey             TO output_devices_pkey;
ALTER TABLE output_devices RENAME CONSTRAINT alarm_devices_org_id_fkey      TO output_devices_org_id_fkey;
ALTER TABLE output_devices RENAME CONSTRAINT alarm_devices_location_id_fkey TO output_devices_location_id_fkey;

-- Indexes
ALTER INDEX idx_alarm_devices_org      RENAME TO idx_output_devices_org;
ALTER INDEX idx_alarm_devices_location RENAME TO idx_output_devices_location;

-- Triggers (named per the device entity)
ALTER TRIGGER generate_alarm_device_id_trigger ON output_devices RENAME TO generate_output_device_id_trigger;
ALTER TRIGGER update_alarm_devices_updated_at  ON output_devices RENAME TO update_output_devices_updated_at;

-- RLS policy
ALTER POLICY org_isolation_alarm_devices ON output_devices RENAME TO org_isolation_output_devices;

-- Refresh comments to the new entity name.
COMMENT ON TABLE output_devices IS 'TRA-929 (was TRA-903 alarm_devices): output/actuator devices (demo: Shelly Gen4 relay). Internal-only CRUD; fired by the geofence engine via alarm.Firer.';
COMMENT ON COLUMN output_devices.base_url IS 'Local HTTP base URL of the device, e.g. http://192.168.50.66 (Shelly Gen2+ RPC).';
COMMENT ON COLUMN output_devices.switch_id IS 'Shelly relay channel passed as Switch.Set id=.';
COMMENT ON COLUMN output_devices.location_id IS 'Optional binding: fires when the geofence engine trips at this logical location (any reader/antenna mapped to it).';
COMMENT ON COLUMN output_devices.transport IS 'TRA-906: http = local edge HTTP RPC; mqtt = publish to the shared broker (firewall-friendly).';
COMMENT ON COLUMN output_devices.command_topic IS 'TRA-906: Shelly MQTT topic prefix (mqtt transport); backend publishes to <command_topic>/command/switch:<switch_id>.';
