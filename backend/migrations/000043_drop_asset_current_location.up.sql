SET search_path = trakrf,public;

-- TRA-799: current_location becomes purely derived from the latest asset_scans
-- row. Drop the denormalized assets.current_location_id column. Reverses the
-- writable field from TRA-477 while still pre-launch and free to change.

-- 1. Backfill — preserve legacy create-time locations as scan rows before the
--    column is dropped. Idempotent via NOT EXISTS: no-ops on prod/preview
--    (already applied by hand), runs correctly on fresh/restored databases.
--    identifier_scan_id and scan_point_id are nullable — only these four
--    columns are supplied.
INSERT INTO asset_scans (timestamp, org_id, asset_id, location_id)
SELECT a.updated_at, a.org_id, a.id, a.current_location_id
FROM assets a
WHERE a.current_location_id IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM asset_scans s WHERE s.asset_id = a.id);

-- 2. Redefine create_asset_with_tags() without p_current_location_id — asset
--    location is scan/operational data, never set on create (TRA-734). Drop
--    the old 10-arg signature first so the overload does not linger.
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id INT, tag_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_tag_ids INT[] := '{}';
    v_tag JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, external_key, name, description,
        valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags)
        LOOP
            INSERT INTO trakrf.tags (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

-- 3. Drop the denormalized column and its index. Location derives solely from
--    the latest asset_scans row.
DROP INDEX IF EXISTS idx_assets_current_location;
ALTER TABLE assets DROP COLUMN current_location_id;
