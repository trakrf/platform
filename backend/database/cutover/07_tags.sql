-- TRA-810 — pull tags. Natural key: (org_id, type, value).
-- FKs: asset_id via assets.external_key OR location_id via locations.external_key (mutually exclusive).
\set ON_ERROR_STOP on

INSERT INTO trakrf.tags
    (org_id, type, value, asset_id, location_id, valid_from, valid_to,
     is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.type, s.value, t_asset.id, t_loc.id,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.tags s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
LEFT JOIN cloud_src.assets src_asset   ON src_asset.id = s.asset_id
LEFT JOIN cloud_src.locations src_loc  ON src_loc.id = s.location_id
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
LEFT JOIN trakrf.assets t_asset
        ON t_asset.org_id = t_org.id AND t_asset.external_key = src_asset.external_key
LEFT JOIN trakrf.locations t_loc
        ON t_loc.org_id = t_org.id AND t_loc.external_key = src_loc.external_key
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND (src_asset.id IS NULL OR src_asset.deleted_at IS NULL)
  AND (src_loc.id   IS NULL OR src_loc.deleted_at IS NULL)
  -- Skip tags whose only FK target was soft-deleted (would now violate tag_target CHECK).
  AND (
        (s.asset_id IS NOT NULL AND t_asset.id IS NOT NULL)
     OR (s.location_id IS NOT NULL AND t_loc.id IS NOT NULL)
  );

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    -- Source count must exclude tags whose targeted asset/location was soft-deleted.
    SELECT count(*) INTO src_n FROM cloud_src.tags s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (
                (s.asset_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.assets a
                      WHERE a.id = s.asset_id AND a.deleted_at IS NULL))
             OR (s.location_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.locations l
                      WHERE l.id = s.location_id AND l.deleted_at IS NULL))
          );
    SELECT count(*) INTO tgt_n FROM trakrf.tags;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'tags mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'tags OK: % rows', tgt_n;
END $$;
