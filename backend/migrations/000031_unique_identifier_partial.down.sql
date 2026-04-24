SET search_path=trakrf,public;

-- TRA-475 down: drop the partial indexes and restore the legacy
-- 3-column UNIQUE constraint. Soft-deletions performed by the up
-- migration's dedup step are NOT reverted — the partition tombstones
-- remain. Re-running up is idempotent.

DROP INDEX IF EXISTS trakrf.assets_org_id_identifier_unique;
DROP INDEX IF EXISTS trakrf.locations_org_id_identifier_unique;

ALTER TABLE trakrf.assets
    ADD CONSTRAINT assets_org_id_identifier_valid_from_key
    UNIQUE (org_id, identifier, valid_from);

ALTER TABLE trakrf.locations
    ADD CONSTRAINT locations_org_id_identifier_valid_from_key
    UNIQUE (org_id, identifier, valid_from);
