-- TRA-906 — fire path transport for alarm_devices. HTTP (local edge) stays the
-- default; MQTT is the firewall-friendly path (Shelly subscribes outbound to the
-- broker, backend publishes to its command topic). command_topic is the Shelly's
-- MQTT topic prefix; the backend publishes to <command_topic>/command/switch:<switch_id>.
SET search_path = trakrf, public;

CREATE TYPE alarm_transport AS ENUM ('http', 'mqtt');

ALTER TABLE alarm_devices
    ADD COLUMN transport     alarm_transport NOT NULL DEFAULT 'http',
    ADD COLUMN command_topic VARCHAR(255);

COMMENT ON COLUMN alarm_devices.transport IS 'TRA-906: http = local edge HTTP RPC; mqtt = publish to the shared broker (firewall-friendly).';
COMMENT ON COLUMN alarm_devices.command_topic IS 'TRA-906: Shelly MQTT topic prefix (mqtt transport); backend publishes to <command_topic>/command/switch:<switch_id>.';
