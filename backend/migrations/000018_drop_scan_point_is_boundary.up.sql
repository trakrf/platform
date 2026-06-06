-- TRA-943: the is_boundary gate is removed. After output devices were mapped to
-- location (not scan point), a per-scan-point boundary flag no longer maps to a
-- clean per-portal concept; all rule config now lives visibly on the output
-- device. Greenfield (unreleased) — safe to drop.
ALTER TABLE trakrf.scan_points DROP COLUMN is_boundary;
