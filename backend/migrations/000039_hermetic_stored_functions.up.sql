-- TRA-1076 — pin search_path on every stored function, so none of them depends
-- on the caller's session to resolve a name.
--
-- A function with no `SET search_path` of its own resolves unqualified names
-- through the *caller's* path. Everything works today only because every
-- connection carries search_path=trakrf,public, which makes a runtime connection
-- parameter load-bearing rather than a convenience — and is why TRA-1074 must
-- replace the DSN setting rather than simply delete it.
--
-- The break is transitive, and wider than the three functions TRA-1076 named.
-- _feistel_encrypt calls hmac() unqualified, so it resolves only while whichever
-- schema holds pgcrypto is on the path (see the placement table below — it is not
-- the same schema in every environment). _feistel_encrypt backs
-- generate_obfuscated_id, the BEFORE INSERT id trigger on every table in the
-- schema — so with the path narrowed, *every INSERT* fails, not just the stored
-- procedures. Verified locally: `SET search_path = public; SELECT
-- trakrf._feistel_encrypt(1);` → "function hmac(bytea, bytea, unknown) does not
-- exist".
--
-- Beyond tidiness: an unqualified reference inside a SECURITY DEFINER function is
-- a privilege-escalation vector, since a caller who can create objects earlier on
-- the path can shadow the intended table and have it execute as the definer.
-- (ADR 0003 names this as the reason a configurable schema name can never work.)
--
-- Pattern copied from trakrf.org_is_entitled (000022): pin the path *and*
-- schema-qualify the body, so correctness never rests on the attribute alone. A
-- qualified body is also what makes the pin safe without listing pg_temp — PG
-- searches pg_temp implicitly first for relations, but a schema-qualified
-- reference cannot be shadowed by a temp object.
--
-- hmac() is deliberately left to resolve through the pinned path rather than
-- hardcoded as trakrf.hmac(). pgcrypto's schema is deployment-dependent, and the
-- environments genuinely disagree — measured 2026-08-05:
--
--     local (docker)  pgcrypto in trakrf
--     preview (CNPG)  pgcrypto in public
--     prod    (CNPG)  pgcrypto in public
--
-- Local lands in trakrf because 000001 says `CREATE EXTENSION pgcrypto`
-- unqualified and the runner's search_path decides; the deployed pair predate
-- that and were installed into public. So `SET search_path = trakrf, public`
-- resolves hmac wherever it actually is, while a hardcoded trakrf.hmac() would
-- fail on both preview and prod. Do not "tidy" this into a qualified call.
--
-- Already correct, and untouched here: org_is_entitled (000022),
-- org_capability_set (000036), resolve_scan_topic (000020),
-- list_active_scan_topics (000021).
--
-- process_tag_scans (000010) is NOT restored: 000012 dropped it when the Go MQTT
-- subscriber replaced the PG-trigger fan-out (TRA-900). Making a dropped function
-- hermetic would resurrect it.
--
-- Bodies below are unchanged from their current definitions except where noted;
-- the only edit is the added SET clause.

SET search_path = trakrf, public;

-- ============================================================================
-- 000002 — the id generator and updated_at trigger functions
-- ============================================================================

-- The root of the transitive break: the unqualified hmac() call.
CREATE OR REPLACE FUNCTION trakrf._feistel_encrypt(seq_value BIGINT) RETURNS BIGINT
LANGUAGE plpgsql STABLE
SET search_path = trakrf, public
AS $$
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
LANGUAGE plpgsql
SET search_path = trakrf, public
AS $$
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
LANGUAGE plpgsql
SET search_path = trakrf, public
AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;

-- ============================================================================
-- 000010 — the stored procedures TRA-1076 named
-- ============================================================================
-- Both bodies were already schema-qualified (the ticket's description predates
-- the 000001-000010 re-baseline). They still failed under a hostile search_path,
-- because their INSERTs fire the id trigger and land in _feistel_encrypt.

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
) RETURNS TABLE (asset_id BIGINT, tag_ids BIGINT[])
LANGUAGE plpgsql
SET search_path = trakrf, public
AS $$
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
$$;

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
) RETURNS TABLE (location_id BIGINT, tag_ids BIGINT[])
LANGUAGE plpgsql
SET search_path = trakrf, public
AS $$
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
$$;

-- ============================================================================
-- 000017 — normalize_tag_value: qualified, NOT pinned
-- ============================================================================
-- This one deliberately does not get a SET clause. proconfig makes a SQL function
-- un-inlinable (verified against this PG: an identical function with a SET clause
-- stays as a function call in EXPLAIN VERBOSE where the unpinned one is folded to
-- its body), and 000017 chose SQL-over-plpgsql + IMMUTABLE precisely so it could
-- inline — it backs the tags.normalized_value generated column and the per-read
-- ingest membership query in storage/ingest.go.
--
-- It needs no pin: the body touches no tables, and its built-ins are now written
-- as pg_catalog.* so they cannot be resolved through the path at all. That is
-- strictly stronger than a pin, and free.
--
-- Also restates the schema in the name. 000017 wrote `CREATE FUNCTION
-- normalize_tag_value`, unqualified, so its placement was decided by that
-- migration's own search_path; it landed in trakrf, and this makes that explicit
-- rather than incidental.
--
-- The logic is byte-for-byte equivalent (pg_catalog.upper IS upper), so the
-- STORED generated column needs no recompute — no rewrite is triggered and none
-- is required. Contrast the path-derived-column rule in CONVENTIONS.md, which
-- applies when the derivation actually changes.
CREATE OR REPLACE FUNCTION trakrf.normalize_tag_value(v text) RETURNS text
    LANGUAGE sql IMMUTABLE PARALLEL SAFE
    AS $$
        SELECT pg_catalog.regexp_replace(
                   pg_catalog.regexp_replace(pg_catalog.upper(v), '[^0-9A-F]', '', 'g'),
                   '^0+(?=[0-9])', '')
    $$;

COMMENT ON FUNCTION trakrf.normalize_tag_value(text) IS 'TRA-944: hex tag-value match key (uppercase, strip non-hex, strip leading zeros keeping >=1 digit); mirrors handheld getMatchingKey. Shared by tags.normalized_value and the ingest query. TRA-1076: pg_catalog-qualified built-ins rather than a pinned search_path, to stay inlinable.';
