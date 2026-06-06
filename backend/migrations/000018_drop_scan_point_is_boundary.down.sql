ALTER TABLE trakrf.scan_points
    ADD COLUMN is_boundary BOOLEAN NOT NULL DEFAULT false;

COMMENT ON COLUMN trakrf.scan_points.is_boundary IS
    'Geofence boundary marker (TRA-901): true = portal/exit capture point.';
