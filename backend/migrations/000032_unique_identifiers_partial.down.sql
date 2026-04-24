SET search_path=trakrf,public;

-- TRA-482 down: drop the partial index and restore the legacy 4-column
-- UNIQUE constraint. Soft-deletions performed by the up migration's
-- dedup step are NOT reverted — the tombstones remain. Re-running up
-- is idempotent.

DROP INDEX IF EXISTS trakrf.identifiers_org_id_type_value_unique;

ALTER TABLE trakrf.identifiers
    ADD CONSTRAINT identifiers_org_id_type_value_valid_from_key
    UNIQUE (org_id, type, value, valid_from);
