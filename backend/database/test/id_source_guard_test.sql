-- TRA-886 — Regression guard: a single shared sequence is the ONLY surrogate-id
-- source. Asserts the per-table *_seq sequences cannot be quietly reintroduced
-- and that every Feistel id trigger mints from trakrf.id_seq via the no-arg
-- generate_obfuscated_id() function.
--
-- Run against a freshly-migrated DB (no seed data required):
--   export PG_URL_LOCAL=...
--   ./backend/database/test/run_id_source_guard.sh
--
-- Like feistel_parity_test.sql, this is a manual/local check — it is not wired
-- into CI or a `just` recipe (neither is the parity test). RAISE EXCEPTION on
-- any failure (with ON_ERROR_STOP that exits non-zero); RAISE NOTICE on pass.

\set ON_ERROR_STOP on
SET search_path = trakrf, public;

DO $$
DECLARE
    -- The complete set of sequences allowed to exist in the trakrf schema:
    --   id_seq                       — the one shared Feistel surrogate-id source
    --   *_id_seq (4)                 — implicit sequences owned by non-Feistel
    --                                  auto-id columns (BIGSERIAL / IDENTITY):
    --                                  org_invitations, password_reset_tokens,
    --                                  refresh_tokens, tag_scans
    allowed_seqs TEXT[] := ARRAY[
        'id_seq',
        'org_invitations_id_seq',
        'password_reset_tokens_id_seq',
        'refresh_tokens_id_seq',
        'tag_scans_id_seq'
    ];
    stray TEXT;
    n INT;
    src TEXT;
BEGIN
    -- 1. The shared sequence exists.
    IF NOT EXISTS (
        SELECT 1 FROM pg_sequences
        WHERE schemaname = 'trakrf' AND sequencename = 'id_seq'
    ) THEN
        RAISE EXCEPTION 'TRA-886 guard: trakrf.id_seq is missing — the shared surrogate-id sequence must exist';
    END IF;

    -- 2. No sequence outside the allow-list exists. This catches any per-table
    --    *_seq (asset_seq, tag_seq, …) being reintroduced.
    SELECT string_agg(sequencename, ', ' ORDER BY sequencename) INTO stray
    FROM pg_sequences
    WHERE schemaname = 'trakrf'
      AND NOT (sequencename = ANY(allowed_seqs));
    IF stray IS NOT NULL THEN
        RAISE EXCEPTION 'TRA-886 guard: unexpected sequence(s) in trakrf schema: % — surrogate ids must mint only from id_seq', stray;
    END IF;

    -- 3. Every BEFORE INSERT trigger bound to generate_obfuscated_id() must pass
    --    ZERO arguments — i.e. no per-table sequence name. tgnargs = 0 means the
    --    trigger relies on the function's hardcoded nextval('trakrf.id_seq').
    SELECT count(*) INTO n
    FROM pg_trigger t
    JOIN pg_proc p ON p.oid = t.tgfoid
    WHERE p.proname = 'generate_obfuscated_id'
      AND t.tgnargs <> 0;
    IF n > 0 THEN
        RAISE EXCEPTION 'TRA-886 guard: % generate_obfuscated_id trigger(s) still pass a sequence-name argument — they must mint from the shared id_seq with no args', n;
    END IF;

    -- 4. There must actually be surrogate-id triggers (guards against the count
    --    in check 3 being trivially zero because the function/triggers vanished).
    SELECT count(*) INTO n
    FROM pg_trigger t
    JOIN pg_proc p ON p.oid = t.tgfoid
    WHERE p.proname = 'generate_obfuscated_id';
    IF n < 9 THEN
        RAISE EXCEPTION 'TRA-886 guard: expected >= 9 generate_obfuscated_id triggers, found % — surrogate-id minting is not wired up', n;
    END IF;

    -- 5. The trigger function reads the shared sequence by name and no longer
    --    consults TG_ARGV — so a future trigger cannot redirect it to another
    --    sequence even if one were created.
    SELECT prosrc INTO src
    FROM pg_proc
    WHERE proname = 'generate_obfuscated_id'
      AND pronamespace = 'trakrf'::regnamespace;
    IF src IS NULL THEN
        RAISE EXCEPTION 'TRA-886 guard: trakrf.generate_obfuscated_id() is missing';
    END IF;
    IF position('id_seq' IN src) = 0 THEN
        RAISE EXCEPTION 'TRA-886 guard: generate_obfuscated_id() does not reference id_seq — it must mint from the shared sequence';
    END IF;
    IF position('TG_ARGV' IN src) <> 0 THEN
        RAISE EXCEPTION 'TRA-886 guard: generate_obfuscated_id() still references TG_ARGV — the per-table sequence-name plumbing must be removed';
    END IF;

    RAISE NOTICE 'TRA-886 id-source guard PASSED: single shared id_seq is the only surrogate-id source';
END $$;
