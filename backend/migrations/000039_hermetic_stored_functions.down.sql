-- TRA-1076 down — restore the pre-000039 function definitions verbatim.
--
-- Each body below is the definition this migration replaced: 000002 for the id
-- generator and updated_at trigger, 000010 for the two stored procedures, 000017
-- for normalize_tag_value. The only difference from the .up.sql versions is the
-- absent SET clause (and, for normalize_tag_value, the unqualified built-ins).
--
-- Rolling back reinstates the dependency on the caller's search_path: with
-- trakrf off the path, every INSERT in the schema fails again at hmac(). That is
-- the point of a faithful down migration, not a defect in it.

SET search_path = trakrf, public;

-- ---- 000002 -----------------------------------------------------------------

CREATE OR REPLACE FUNCTION trakrf._feistel_encrypt(seq_value BIGINT) RETURNS BIGINT
LANGUAGE plpgsql STABLE AS $$
DECLARE
    master_key BYTEA;
    L BIGINT;
    R BIGINT;
    L_new BIGINT;
    round_idx INT;
    round_key BYTEA;
    f_out BIGINT;
    MASK26 CONSTANT BIGINT := (1::bigint << 26) - 1;
BEGIN
    IF seq_value >= (1::bigint << 52) THEN
        RAISE EXCEPTION 'Feistel input overflow: % >= 2^52', seq_value;
    END IF;

    -- Two-arg current_setting returns NULL on missing instead of erroring;
    -- explicit empty-string check guards against silent corruption (decode('','hex')
    -- yields a zero-length bytea, and hmac() accepts it, producing deterministic-
    -- but-wrong outputs).
    DECLARE
        key_hex TEXT := current_setting('app.obfuscation_key', true);
    BEGIN
        IF key_hex IS NULL OR key_hex = '' THEN
            RAISE EXCEPTION 'app.obfuscation_key is not set on this database. Run: ALTER DATABASE <db> SET app.obfuscation_key = ''<64-hex-char-secret>''';
        END IF;
        master_key := decode(key_hex, 'hex');
    END;

    L := (seq_value >> 26) & MASK26;
    R := seq_value & MASK26;

    FOR round_idx IN 1..6 LOOP
        round_key := hmac(('round-' || round_idx)::bytea, master_key, 'sha256');
        -- Take first 4 bytes of HMAC(int8send(R), round_key), interpret as
        -- big-endian uint32, mask to 26 bits.
        f_out := ('x' || encode(substring(
                    hmac(int8send(R), round_key, 'sha256')
                    FROM 1 FOR 4), 'hex'))::bit(32)::bigint & MASK26;
        L_new := R;
        R := L # f_out;
        L := L_new;
    END LOOP;

    -- Pure Feistel output in [0, 2^52). Probability of NEW.id = 0 is 1/2^52 ≈ 2e-16; not handled, see TRA-720 design.
    RETURN (L << 26) | R;
END;
$$;

CREATE OR REPLACE FUNCTION trakrf.generate_obfuscated_id() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    seq_value BIGINT;
BEGIN
    -- Single shared sequence for all surrogate ids (TRA-886). The sequence name
    -- is hardcoded and the function takes no trigger argument, so no trigger can
    -- redirect minting to a per-table sequence and reintroduce cross-type id
    -- equality.
    seq_value := nextval('trakrf.id_seq');
    NEW.id := trakrf._feistel_encrypt(seq_value);
    RETURN NEW;
END;
$$;

CREATE OR REPLACE FUNCTION trakrf.update_updated_at_column() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

-- ---- 000010 -----------------------------------------------------------------

CREATE OR REPLACE FUNCTION trakrf.create_asset_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (asset_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_asset_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
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
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
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

CREATE OR REPLACE FUNCTION trakrf.create_location_with_tags(
    p_org_id BIGINT,
    p_external_key VARCHAR(255),
    p_name VARCHAR(255),
    p_description TEXT,
    p_parent_location_id BIGINT,
    p_valid_from TIMESTAMPTZ,
    p_valid_to TIMESTAMPTZ,
    p_is_active BOOLEAN,
    p_metadata JSONB,
    p_tags JSONB
) RETURNS TABLE (location_id BIGINT, tag_ids BIGINT[]) AS $$
DECLARE
    v_location_id BIGINT;
    v_tag_ids BIGINT[] := '{}';
    v_tag JSONB;
    v_new_id BIGINT;
BEGIN
    INSERT INTO trakrf.locations (
        org_id, external_key, name, description,
        parent_location_id, valid_from, valid_to, is_active, metadata
    ) VALUES (
        p_org_id, p_external_key, p_name, p_description,
        p_parent_location_id, p_valid_from, p_valid_to, p_is_active, p_metadata
    )
    RETURNING id INTO v_location_id;

    IF p_tags IS NOT NULL AND jsonb_array_length(p_tags) > 0 THEN
        FOR v_tag IN SELECT * FROM jsonb_array_elements(p_tags) LOOP
            INSERT INTO trakrf.tags (org_id, type, value, location_id, is_active)
            VALUES (
                p_org_id,
                COALESCE(v_tag->>'type', 'rfid'),
                v_tag->>'value',
                v_location_id,
                TRUE
            )
            RETURNING id INTO v_new_id;
            v_tag_ids := array_append(v_tag_ids, v_new_id);
        END LOOP;
    END IF;

    RETURN QUERY SELECT v_location_id, v_tag_ids;
END;
$$ LANGUAGE plpgsql;

-- ---- 000017 -----------------------------------------------------------------
-- Unqualified built-ins, as originally written. Equivalent output, so the STORED
-- generated column is unaffected either way.

CREATE OR REPLACE FUNCTION trakrf.normalize_tag_value(v text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT regexp_replace(regexp_replace(upper(v), '[^0-9A-F]', '', 'g'), '^0+(?=[0-9])', '')
    $$;

COMMENT ON FUNCTION trakrf.normalize_tag_value(text) IS 'TRA-944: hex tag-value match key (uppercase, strip non-hex, strip leading zeros keeping >=1 digit); mirrors handheld getMatchingKey. Shared by tags.normalized_value and the ingest query.';
