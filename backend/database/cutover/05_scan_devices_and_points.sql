-- TRA-810 — pull scan_devices then scan_points.
-- scan_devices natural key: (org_id, external_key). TRA-899 renamed the target
--   column identifier -> external_key; the cloud source still exposes `identifier`.
-- scan_points natural key: (org_id, external_key); FKs via device.external_key + location.external_key.
\set ON_ERROR_STOP on

INSERT INTO trakrf.scan_devices
    (org_id, external_key, name, type, serial_number, model, description,
     valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.identifier, s.name, s.type, s.serial_number, s.model, s.description,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.scan_devices s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.scan_devices s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.scan_devices;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'scan_devices mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'scan_devices OK: % rows', tgt_n;
END $$;

INSERT INTO trakrf.scan_points
    (org_id, scan_device_id, location_id, external_key, name, antenna_port,
     description, valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, t_dev.id, t_loc.id, s.identifier, s.name, s.antenna_port,
    s.description, s.valid_from, s.valid_to, s.is_active, s.metadata,
    s.created_at, s.updated_at
FROM cloud_src.scan_points s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
JOIN cloud_src.scan_devices src_dev    ON src_dev.id = s.scan_device_id
LEFT JOIN cloud_src.locations src_loc  ON src_loc.id = s.location_id
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
JOIN trakrf.scan_devices t_dev
        ON t_dev.org_id = t_org.id AND t_dev.external_key = src_dev.identifier
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND src_dev.deleted_at IS NULL
  AND (src_loc.id IS NULL OR src_loc.deleted_at IS NULL);

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.scan_points s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.scan_devices d  ON d.id = s.scan_device_id AND d.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (s.location_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.locations l
                            WHERE l.id = s.location_id AND l.deleted_at IS NULL));
    SELECT count(*) INTO tgt_n FROM trakrf.scan_points;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'scan_points mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'scan_points OK: % rows', tgt_n;
END $$;
