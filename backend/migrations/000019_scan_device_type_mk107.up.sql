-- TRA-961 — register the MOKO MK107 Pro BLE gateway as a scan device type.
-- Without it, the topic->device resolver yields type 'moko_mk107' which has no
-- enum slot, and even once parsed (parser_mk107.go) the device can't be stored.
-- Analogous to gl_s10 (TRA-925); placed after gl_s10 to keep the BLE-gateway
-- types grouped. Single statement / schema-qualified so it never runs inside a
-- multi-statement transaction block (ADD VALUE restriction).
ALTER TYPE trakrf.scan_device_type ADD VALUE IF NOT EXISTS 'moko_mk107' AFTER 'gl_s10';
