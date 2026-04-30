SET search_path=trakrf,public;

-- Recreate create_asset_with_tags without the p_type parameter. The Go caller
-- (storage/assets.go) invokes positionally via SELECT *, so dropping a
-- parameter is a coordinated change — see the matching code edit in this PR.
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, VARCHAR, TEXT, INT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE OR REPLACE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_current_location_id INT,
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
        org_id, identifier, name, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
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

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;
ALTER TABLE assets DROP COLUMN type;
