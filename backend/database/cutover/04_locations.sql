-- TRA-810 — pull locations.
-- Natural key: external_key (unique per org for live rows).
-- Two-phase for parent_location_id self-FK.
\set ON_ERROR_STOP on

-- Phase 1: insert all live locations with parent_location_id = NULL.
INSERT INTO trakrf.locations
    (org_id, external_key, name, description, parent_location_id,
     valid_from, valid_to, is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.external_key, s.name, s.description, NULL,
    s.valid_from, s.valid_to, s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.locations s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

-- Phase 2: set parent_location_id for rows whose parent is also live.
UPDATE trakrf.locations t_child
SET parent_location_id = t_parent.id
FROM cloud_src.locations s_child
JOIN cloud_src.organizations src_org ON src_org.id = s_child.org_id
JOIN cloud_src.locations s_parent ON s_parent.id = s_child.parent_location_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
JOIN trakrf.locations t_parent
       ON t_parent.org_id = t_org.id AND t_parent.external_key = s_parent.external_key
WHERE t_child.org_id = t_org.id
  AND t_child.external_key = s_child.external_key
  AND s_child.deleted_at IS NULL
  AND s_parent.deleted_at IS NULL
  AND src_org.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT; src_parents INT; tgt_parents INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.locations;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'locations count mismatch: src=% tgt=%', src_n, tgt_n; END IF;

    -- Parent linkage: count source live rows whose parent is also live.
    SELECT count(*) INTO src_parents FROM cloud_src.locations s
        JOIN cloud_src.locations p ON p.id = s.parent_location_id AND p.deleted_at IS NULL
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_parents FROM trakrf.locations WHERE parent_location_id IS NOT NULL;
    IF src_parents <> tgt_parents THEN
        RAISE EXCEPTION 'locations parent_location_id link mismatch: src=% tgt=%', src_parents, tgt_parents;
    END IF;
    RAISE NOTICE 'locations OK: % rows (% with live parent)', tgt_n, tgt_parents;
END $$;
