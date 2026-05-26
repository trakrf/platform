-- TRA-810 — api_keys: SKIPPED.
--
-- Per `project_prelaunch_no_prod_keys` and user direction 2026-05-26: no active
-- API customers exist, so no integration depends on these keys surviving the
-- cutover. Pre-launch source api_keys are entirely BB-test / BB9-eval churn
-- (all revoked, ~117 rows total). Carrying them adds no value, and the
-- `api_keys_creator_exactly_one` CHECK constraint would force two-phase load
-- gymnastics with DROP/re-ADD of the CHECK for key-rooted children.
--
-- If a future pre-cutover audit shows a real customer with active keys, revisit
-- this script to implement the two-phase load (or have the customer rotate
-- their keys post-cutover and accept the discontinuity).
\set ON_ERROR_STOP on

DO $$ BEGIN
    RAISE NOTICE 'api_keys: SKIPPED by design (no active API customers pre-launch)';
END $$;
