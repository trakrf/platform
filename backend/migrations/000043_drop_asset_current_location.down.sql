SET search_path = trakrf,public;

-- Re-add the denormalized column, index, and comment. Data is NOT restored:
-- the up-migration's backfilled asset_scans rows remain (removing them is
-- unsafe — indistinguishable from genuine device reads). current_location_id
-- comes back NULL for all rows.
ALTER TABLE assets ADD COLUMN current_location_id INT REFERENCES locations(id);
CREATE INDEX idx_assets_current_location ON assets(current_location_id);
COMMENT ON COLUMN assets.current_location_id IS 'Denormalized current location for query performance';

-- Restore create_asset_with_tags() with the p_current_location_id parameter.
DROP FUNCTION IF EXISTS create_asset_with_tags(
    INT, VARCHAR, VARCHAR, TEXT,
    TIMESTAMPTZ, TIMESTAMPTZ, BOOLEAN, JSONB, JSONB
);

CREATE FUNCTION create_asset_with_tags(
    p_org_id INT,
    p_external_key VARCHAR(255),
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
        org_id, external_key, name, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
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
