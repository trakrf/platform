-- TRA-961 down — intentionally a no-op.
--
-- PostgreSQL cannot DROP a value from an enum in place; reversing this would
-- require rebuilding scan_device_type from scratch and rewriting every
-- dependent object (scan_devices.type and the process_tag_scans return type,
-- migration 000012). An unused enum value is harmless, so we leave 'moko_mk107'
-- in place rather than carry that risk. Re-applying the up migration is a no-op
-- thanks to ADD VALUE IF NOT EXISTS.
SELECT 1;
