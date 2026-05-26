-- Optional pre-pull cleanup. ONLY runs when invoked explicitly via --truncate.
-- DO NOT run against production. Drops all entity-table rows on the target.
-- Sequences are restarted so Feistel inputs start fresh.
\set ON_ERROR_STOP on

TRUNCATE TABLE
    trakrf.asset_scans,
    trakrf.tags,
    trakrf.api_keys,
    trakrf.org_invitations,
    trakrf.assets,
    trakrf.scan_points,
    trakrf.scan_devices,
    trakrf.locations,
    trakrf.org_users,
    trakrf.users,
    trakrf.organizations
RESTART IDENTITY CASCADE;

DO $$ BEGIN RAISE NOTICE 'target entity tables truncated'; END $$;
