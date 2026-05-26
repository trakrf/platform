-- TRA-810 — drop FDW objects after pull. Does NOT touch trakrf.* data.
\set ON_ERROR_STOP on

DROP SCHEMA IF EXISTS cloud_src CASCADE;
DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER cloud_src_srv;
DROP SERVER IF EXISTS cloud_src_srv CASCADE;
-- Leave postgres_fdw extension installed (might be needed by other tooling).

DO $$ BEGIN
    RAISE NOTICE 'TRA-810 teardown OK: FDW objects removed';
END $$;
