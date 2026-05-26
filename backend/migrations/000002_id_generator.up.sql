-- TRA-720 — keyed Feistel ID generator + updated_at trigger function.
-- Defined before any table file so subsequent CREATE TRIGGER statements can
-- reference these functions.
--
-- Architecture:
--   * trakrf._feistel_encrypt(seq_value BIGINT) — internal pure function,
--     exposed for test parity against the Go reference at
--     backend/internal/obfuscatedid.
--   * trakrf.generate_obfuscated_id() — the TRIGGER function that wraps
--     _feistel_encrypt and pulls seq_value from nextval(TG_ARGV[0]).
--
-- Construction: 50-bit block (2 x 25-bit halves), 6 rounds, HMAC-SHA256
-- round function truncated to 25 bits, output OR'd with (1::bigint << 50)
-- so values land in [2^50, 2^51) — disjoint from migrated 31-bit IDs from
-- the legacy generate_hashed_id / generate_permuted_id stack.
--
-- Master key is set per database via:
--   ALTER DATABASE <db> SET app.obfuscation_key = '<64-hex-char-secret>';
--
-- Sequence overflow guard: a sequence reaching 2^50 means 1.1 quadrillion
-- inserts on a single table — unreachable in practice. The exception is
-- defensive.

SET search_path = trakrf, public;

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
    MASK25 CONSTANT BIGINT := (1::bigint << 25) - 1;
BEGIN
    IF seq_value >= (1::bigint << 50) THEN
        RAISE EXCEPTION 'Feistel input overflow: % >= 2^50', seq_value;
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

    L := (seq_value >> 25) & MASK25;
    R := seq_value & MASK25;

    FOR round_idx IN 1..6 LOOP
        round_key := hmac(('round-' || round_idx)::bytea, master_key, 'sha256');
        -- Take first 4 bytes of HMAC(int8send(R), round_key), interpret as
        -- big-endian uint32, mask to 25 bits.
        f_out := ('x' || encode(substring(
                    hmac(int8send(R), round_key, 'sha256')
                    FROM 1 FOR 4), 'hex'))::bit(32)::bigint & MASK25;
        L_new := R;
        R := L # f_out;
        L := L_new;
    END LOOP;

    RETURN ((L << 25) | R) | (1::bigint << 50);
END;
$$;

CREATE OR REPLACE FUNCTION trakrf.generate_obfuscated_id() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
DECLARE
    seq_name TEXT := TG_ARGV[0];
    seq_value BIGINT;
BEGIN
    seq_value := nextval(seq_name);
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
