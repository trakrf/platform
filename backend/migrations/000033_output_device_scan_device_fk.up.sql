-- TRA-1028 — address a GPO output device by its reader instead of a free-text
-- command_topic. scan_device_id points at the reader whose GPO the output
-- drives; the RPC base topic is derived from that reader's publish_topic at
-- fire time (readers/output_devices.go), not stored redundantly here.
SET search_path = trakrf, public;

ALTER TABLE output_devices ADD COLUMN scan_device_id BIGINT REFERENCES scan_devices(id);

COMMENT ON COLUMN output_devices.scan_device_id IS
    'TRA-1028: for csl_cs463_gpo, the reader (scan_devices row) whose GPO is driven. The RPC base topic is derived from that reader''s publish_topic at fire time, not persisted here.';
