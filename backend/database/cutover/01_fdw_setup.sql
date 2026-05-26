-- TRA-810 — postgres_fdw setup pointing at legacy Cloud source.
-- Idempotent. Foreign objects live in schema `cloud_src`.

\set ON_ERROR_STOP on

DROP SCHEMA IF EXISTS cloud_src CASCADE;
DROP USER MAPPING IF EXISTS FOR CURRENT_USER SERVER cloud_src_srv;
DROP SERVER IF EXISTS cloud_src_srv CASCADE;
CREATE EXTENSION IF NOT EXISTS postgres_fdw;

CREATE SERVER cloud_src_srv
    FOREIGN DATA WRAPPER postgres_fdw
    OPTIONS (host :'src_host', port :'src_port', dbname :'src_db',
             sslmode 'require', fetch_size '5000');

CREATE USER MAPPING FOR CURRENT_USER SERVER cloud_src_srv
    OPTIONS (user :'src_user', password :'src_pw');

CREATE SCHEMA cloud_src;

-- Import only the tables we will read. The source `trakrf` schema is the live
-- v44 schema; foreign-table column types follow the source. BIGINT vs INT4 at
-- the source is fine — we re-mint IDs on the target side regardless.
IMPORT FOREIGN SCHEMA trakrf
    LIMIT TO (organizations, users, org_users, org_invitations, locations,
              scan_devices, scan_points, assets, tags, api_keys, asset_scans)
    FROM SERVER cloud_src_srv INTO cloud_src;

-- Smoke: must return >0 organizations or fail loudly.
DO $$
DECLARE n INT;
BEGIN
    SELECT count(*) INTO n FROM cloud_src.organizations WHERE deleted_at IS NULL;
    IF n = 0 THEN
        RAISE EXCEPTION 'FDW smoke failed: 0 live organizations visible at source';
    END IF;
    RAISE NOTICE 'FDW smoke OK: % live organizations visible at source', n;
END $$;
