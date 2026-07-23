-- TRA-1028 — CS463 GPO as an alarm output. Two axes: transport is how we reach
-- the device (unchanged: http | mqtt), type is what frame it speaks. A GPO
-- output rides mqtt transport and speaks Gpo.Set instead of Switch.Set.
SET search_path = trakrf, public;

-- NOTE: the new value is added but deliberately NOT used in this migration.
-- PG requires that an enum value added inside a transaction not be referenced
-- until the transaction commits.
ALTER TYPE output_device_type ADD VALUE IF NOT EXISTS 'csl_cs463_gpo';

COMMENT ON COLUMN output_devices.type IS
    'shelly_gen4 = Shelly Gen4 relay (Switch.Set); csl_cs463_gpo = CS463 general purpose output (Gpo.Set, TRA-1028).';
COMMENT ON COLUMN output_devices.switch_id IS
    'Output-channel index; base and range are type-specific. shelly_gen4: 0-based relay channel. csl_cs463_gpo: 1-based GPO port, 1-4.';
COMMENT ON COLUMN output_devices.command_topic IS
    'MQTT topic the device is addressed on (mqtt transport). shelly_gen4: Shelly topic prefix. csl_cs463_gpo: reader RPC base topic, e.g. trakrf.id/cs463-212 (frame goes to <base>/rpc).';
