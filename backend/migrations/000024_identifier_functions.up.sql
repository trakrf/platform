SET search_path=trakrf,public;

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
    p_identifiers JSONB
) RETURNS TABLE (asset_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_asset_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.assets (
        org_id, identifier, name, type, description,
        current_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_type, p_description,
        p_current_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_asset_id;

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, asset_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
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
    p_identifiers JSONB
) RETURNS TABLE (location_id INT, identifier_ids INT[]) AS $$
DECLARE
    v_location_id INT;
    v_identifier_ids INT[] := '{}';
    v_identifier JSONB;
    v_new_id INT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, identifier, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_identifier, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_identifiers IS NOT NULL AND jsonb_array_length(p_identifiers) > 0 THEN
        FOR v_identifier IN SELECT * FROM jsonb_array_elements(p_identifiers)
        LOOP
            INSERT INTO trakrf.identifiers (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_identifier->>'type', 'rfid'),
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
