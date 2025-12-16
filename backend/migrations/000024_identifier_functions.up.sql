SET search_path=trakrf,public;

-- Function to create asset with identifiers atomically
-- Returns: asset_id and array of identifier_ids
-- Runs within caller's transaction - exceptions cause full rollback
CREATE OR REPLACE FUNCTION create_asset_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_type VARCHAR(50),
    p_description TEXT,
    p_current_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB  -- array of {type, value}
) RETURNS TABLE (
    asset_id INT,
    identifier_ids INT[]
) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    -- Insert asset (trigger generates permuted ID)
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    -- Insert each identifier (trigger generates permuted ID)
    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (
                org_id, type, value, asset_id, is_active
            ) VALUES (
                p_org_id,
                v_identifier->>'type',
                v_identifier->>'value',
                v_asset_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_asset_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- Function to create location with identifiers atomically
CREATE OR REPLACE FUNCTION create_location_with_identifiers(
    p_org_id INT,
    p_identifier VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id INT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_identifiers JSONB  -- array of {type, value}
) RETURNS TABLE (
    location_id INT,
    identifier_ids INT[]
) AS $$
DECLARE
    v_location_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    -- Insert location (trigger generates permuted ID)
    INSERT INTO trakrf.locations (
        org_id, identifier, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    -- Insert each identifier (trigger generates permuted ID)
    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (
                org_id, type, value, location_id, is_active
            ) VALUES (
                p_org_id,
                v_identifier->>'type',
                v_identifier->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;

            v_identifier_ids := array_append(v_identifier_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_identifier_ids;
END;
$$ LANGUAGE plpgsql;

-- Comments for documentation
COMMENT ON FUNCTION create_asset_with_identifiers IS 'Atomically creates an asset with its tag identifiers. Any failure rolls back entire operation.';
COMMENT ON FUNCTION create_location_with_identifiers IS 'Atomically creates a location with its tag identifiers. Any failure rolls back entire operation.';
