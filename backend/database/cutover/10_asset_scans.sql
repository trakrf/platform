-- TRA-810 — asset_scans hypertable. Expected empty pre-launch. If non-empty,
-- copy with org_id/asset_id remapped via natural keys.
\set ON_ERROR_STOP on

DO $$
DECLARE n BIGINT;
BEGIN
    SELECT count(*) INTO n FROM cloud_src.asset_scans;
    IF n = 0 THEN
        RAISE NOTICE 'asset_scans OK: 0 rows on source — skipping';
        RETURN;
    END IF;
    RAISE NOTICE 'asset_scans: % rows on source — performing remapped copy', n;
END $$;

-- This INSERT is a no-op when source is empty. When non-empty it remaps.
-- tag_scan_id intentionally NOT carried: tag_scans is skipped from pull,
-- so the source IDs would not point to anything; leave NULL.
INSERT INTO trakrf.asset_scans
    (timestamp, org_id, asset_id, location_id, scan_point_id, tag_scan_id, created_at)
SELECT
    s.timestamp, t_org.id, t_asset.id, t_loc.id, t_sp.id, NULL, s.created_at
FROM cloud_src.asset_scans s
JOIN cloud_src.organizations src_org       ON src_org.id = s.org_id
JOIN cloud_src.assets src_asset            ON src_asset.id = s.asset_id
LEFT JOIN cloud_src.locations src_loc      ON src_loc.id = s.location_id
LEFT JOIN cloud_src.scan_points src_sp     ON src_sp.id = s.scan_point_id
JOIN trakrf.organizations t_org            ON t_org.identifier = src_org.identifier
JOIN trakrf.assets t_asset
        ON t_asset.org_id = t_org.id AND t_asset.external_key = src_asset.external_key
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
LEFT JOIN trakrf.scan_points t_sp
        ON t_sp.org_id = t_org.id AND t_sp.identifier = src_sp.identifier
WHERE src_org.deleted_at   IS NULL
  AND src_asset.deleted_at IS NULL
  AND (src_loc.id IS NULL OR src_loc.deleted_at IS NULL)
  AND (src_sp.id  IS NULL OR src_sp.deleted_at IS NULL);

-- Expected-row count must apply the same filter as the INSERT above:
-- a source scan is migratable only if its asset (required FK) is still live
-- on target. Scans whose source asset was soft-deleted have nowhere to go on
-- the new schema and are dropped by design (consistent with the tombstone
-- filter applied to entity tables).
DO $$ DECLARE src_n BIGINT; tgt_n BIGINT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.asset_scans s
        JOIN cloud_src.organizations o ON o.id = s.org_id   AND o.deleted_at IS NULL
        JOIN cloud_src.assets        a ON a.id = s.asset_id AND a.deleted_at IS NULL
        WHERE (s.location_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.locations l
                            WHERE l.id = s.location_id AND l.deleted_at IS NULL))
          AND (s.scan_point_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.scan_points sp
                            WHERE sp.id = s.scan_point_id AND sp.deleted_at IS NULL));
    SELECT count(*) INTO tgt_n FROM trakrf.asset_scans;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'asset_scans mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'asset_scans OK: % rows', tgt_n;
END $$;
