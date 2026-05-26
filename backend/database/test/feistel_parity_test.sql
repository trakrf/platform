-- TRA-720 — Validate trakrf._feistel_encrypt against the blessed Go test
-- vectors at backend/internal/obfuscatedid/testdata/vectors.json.
--
-- Usage (from repo root):
--   export PG_URL_LOCAL=...
--   ./backend/database/test/run_feistel_parity.sh
-- The runner script sets app.obfuscation_key from vectors.json, then loads
-- this file with the VALUES block replaced by real numbers from vectors.json.
-- This template's placeholder VALUES are overwritten before execution.

\set ON_ERROR_STOP on
SET search_path = trakrf, public;

DO $$
DECLARE
    expected BIGINT;
    got BIGINT;
    seq BIGINT;
    fail_count INT := 0;
BEGIN
    FOR seq, expected IN
        SELECT s::BIGINT, e::BIGINT FROM (VALUES
            -- These five rows are populated by the runner script from
            -- vectors.json. Placeholders here are overwritten before this
            -- block runs. (Runner uses sed to substitute the real values.)
            (1::BIGINT, 0::BIGINT),
            (2::BIGINT, 0::BIGINT),
            (100::BIGINT, 0::BIGINT),
            (12345::BIGINT, 0::BIGINT),
            (16777216::BIGINT, 0::BIGINT),
            (33554432::BIGINT, 0::BIGINT),
            (562949953421312::BIGINT, 0::BIGINT)
        ) AS v(s, e)
    LOOP
        got := trakrf._feistel_encrypt(seq);
        IF got <> expected THEN
            RAISE WARNING 'Mismatch at seq=%: got=%, expected=%', seq, got, expected;
            fail_count := fail_count + 1;
        END IF;
    END LOOP;
    IF fail_count > 0 THEN
        RAISE EXCEPTION 'Feistel parity test FAILED: % mismatch(es)', fail_count;
    END IF;
    RAISE NOTICE 'Feistel parity test PASSED for all vectors';
END $$;
