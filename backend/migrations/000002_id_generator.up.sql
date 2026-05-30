-- TRA-720 — keyed Feistel ID generator + updated_at trigger function.
-- Defined before any table file so subsequent CREATE TRIGGER statements can
-- reference these functions.
--
-- Architecture:
--   * trakrf._feistel_encrypt(seq_value BIGINT) — internal pure function,
--     exposed for test parity against the Go reference at
--     backend/internal/obfuscatedid.
--   * trakrf.generate_obfuscated_id() — the TRIGGER function that wraps
--     _feistel_encrypt and pulls seq_value from nextval('trakrf.id_seq').
--
-- A SINGLE shared sequence (trakrf.id_seq) feeds every BEFORE INSERT trigger
-- (TRA-886). One global counter means no two rows ever share an ordinal, so no
-- two rows share an id — surrogate ids are globally unique by construction, not
-- merely unique within a type. The function reads id_seq by name and takes no
-- argument, so a per-table sequence cannot be reintroduced via TG_ARGV.
--
-- Construction: pure 52-bit Feistel cipher (2 x 26-bit halves), 6 rounds,
-- HMAC-SHA256 round function truncated to 26 bits. Output range: [0, 2^52).
--
-- Master key is set per database via:
--   ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';
--
-- Sequence overflow guard: a sequence reaching 2^52 means 4.5 quadrillion
-- inserts on a single table — unreachable in practice. The exception is
-- defensive.

SET search_path = trakrf, public;

-- The single shared surrogate-id sequence. Every Feistel id trigger draws from
-- this one counter (TRA-886). Do NOT add per-table sequences.
CREATE SEQUENCE trakrf.id_seq AS BIGINT;

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
