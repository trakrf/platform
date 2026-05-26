-- TRA-810 — end-to-end verification after pull.
-- Cross-checks counts, asserts no orphan FKs, samples natural-key parity.
\set ON_ERROR_STOP on

-- ---------- A. Row count parity (live source vs target) ----------
DO $$
DECLARE
    src_org INT; src_usr INT; src_ou INT; src_loc INT; src_dev INT; src_sp INT;
    src_ast INT; src_tag INT; src_inv INT;
    tgt_org INT; tgt_usr INT; tgt_ou INT; tgt_loc INT; tgt_dev INT; tgt_sp INT;
    tgt_ast INT; tgt_tag INT; tgt_inv INT;
BEGIN
    SELECT count(*) INTO src_org FROM cloud_src.organizations WHERE deleted_at IS NULL;
    SELECT count(*) INTO src_usr FROM cloud_src.users         WHERE deleted_at IS NULL;
    SELECT count(*) INTO src_ou  FROM cloud_src.org_users s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.user_id AND u.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_loc FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_dev FROM cloud_src.scan_devices s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_sp  FROM cloud_src.scan_points s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.scan_devices d  ON d.id = s.scan_device_id AND d.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (s.location_id IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.locations l
                            WHERE l.id = s.location_id AND l.deleted_at IS NULL));
    SELECT count(*) INTO src_ast FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO src_tag FROM cloud_src.tags s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL
          AND (
                (s.asset_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.assets a
                      WHERE a.id = s.asset_id AND a.deleted_at IS NULL))
             OR (s.location_id IS NOT NULL AND EXISTS (
                    SELECT 1 FROM cloud_src.locations l
                      WHERE l.id = s.location_id AND l.deleted_at IS NULL))
          );
    SELECT count(*) INTO src_inv FROM cloud_src.org_invitations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE (s.invited_by IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.users u
                            WHERE u.id = s.invited_by AND u.deleted_at IS NULL));

    SELECT count(*) INTO tgt_org FROM trakrf.organizations;
    SELECT count(*) INTO tgt_usr FROM trakrf.users;
    SELECT count(*) INTO tgt_ou  FROM trakrf.org_users;
    SELECT count(*) INTO tgt_loc FROM trakrf.locations;
    SELECT count(*) INTO tgt_dev FROM trakrf.scan_devices;
    SELECT count(*) INTO tgt_sp  FROM trakrf.scan_points;
    SELECT count(*) INTO tgt_ast FROM trakrf.assets;
    SELECT count(*) INTO tgt_tag FROM trakrf.tags;
    SELECT count(*) INTO tgt_inv FROM trakrf.org_invitations;

    IF (src_org, src_usr, src_ou, src_loc, src_dev, src_sp, src_ast, src_tag, src_inv)
       <> (tgt_org, tgt_usr, tgt_ou, tgt_loc, tgt_dev, tgt_sp, tgt_ast, tgt_tag, tgt_inv) THEN
        RAISE EXCEPTION 'verify A — count parity failed: src=(%,%,%,%,%,%,%,%,%) tgt=(%,%,%,%,%,%,%,%,%)',
            src_org, src_usr, src_ou, src_loc, src_dev, src_sp, src_ast, src_tag, src_inv,
            tgt_org, tgt_usr, tgt_ou, tgt_loc, tgt_dev, tgt_sp, tgt_ast, tgt_tag, tgt_inv;
    END IF;
    RAISE NOTICE 'verify A OK: row-count parity across 9 entity tables';
END $$;

-- ---------- B. FK integrity — every FK column resolves to a target row ----------
DO $$
DECLARE n INT;
BEGIN
    -- users.last_org_id (nullable)
    SELECT count(*) INTO n FROM trakrf.users u
        WHERE u.last_org_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.organizations o WHERE o.id = u.last_org_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — users.last_org_id orphans: %', n; END IF;

    -- locations.parent_location_id (nullable)
    SELECT count(*) INTO n FROM trakrf.locations l
        WHERE l.parent_location_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations p WHERE p.id = l.parent_location_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — locations.parent_location_id orphans: %', n; END IF;

    -- scan_points.location_id (nullable)
    SELECT count(*) INTO n FROM trakrf.scan_points sp
        WHERE sp.location_id IS NOT NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations l WHERE l.id = sp.location_id);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — scan_points.location_id orphans: %', n; END IF;

    -- tags: exactly one of asset_id/location_id NOT NULL (CHECK tag_target).
    SELECT count(*) INTO n FROM trakrf.tags t
        WHERE (t.asset_id IS NULL) = (t.location_id IS NULL);
    IF n > 0 THEN RAISE EXCEPTION 'verify B — tags violating tag_target CHECK: %', n; END IF;

    RAISE NOTICE 'verify B OK: no FK orphans, tag_target CHECK respected';
END $$;

-- ---------- C. Natural-key parity — every source live row's natural key
-- has a matching target row. Sample-based for speed but exhaustive per entity. ----------
DO $$
DECLARE missing INT;
BEGIN
    SELECT count(*) INTO missing FROM cloud_src.organizations s
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.organizations t WHERE t.identifier = s.identifier);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live organizations missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.users s
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.users t WHERE t.email = s.email);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live users missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.locations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN trakrf.organizations t_org ON t_org.identifier = o.identifier
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.locations t
                            WHERE t.org_id = t_org.id AND t.external_key = s.external_key);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live locations missing on target', missing; END IF;

    SELECT count(*) INTO missing FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN trakrf.organizations t_org ON t_org.identifier = o.identifier
        WHERE s.deleted_at IS NULL
          AND NOT EXISTS (SELECT 1 FROM trakrf.assets t
                            WHERE t.org_id = t_org.id AND t.external_key = s.external_key);
    IF missing > 0 THEN RAISE EXCEPTION 'verify C — % live assets missing on target', missing; END IF;

    RAISE NOTICE 'verify C OK: natural-key parity for org/user/location/asset';
END $$;

-- ---------- D. Surrogate ID sanity — every target id is in [1, 2^52) ----------
DO $$
DECLARE bad INT;
BEGIN
    SELECT count(*) INTO bad FROM (
        SELECT id FROM trakrf.organizations
        UNION ALL SELECT id FROM trakrf.users
        UNION ALL SELECT id FROM trakrf.locations
        UNION ALL SELECT id FROM trakrf.scan_devices
        UNION ALL SELECT id FROM trakrf.scan_points
        UNION ALL SELECT id FROM trakrf.assets
        UNION ALL SELECT id FROM trakrf.tags
    ) t WHERE id < 0 OR id >= (1::BIGINT << 52);
    IF bad > 0 THEN RAISE EXCEPTION 'verify D — % rows with id outside Feistel [0, 2^52)', bad; END IF;
    RAISE NOTICE 'verify D OK: all surrogate ids in Feistel range';
END $$;

DO $$ BEGIN RAISE NOTICE 'TRA-810 verification: all gates passed'; END $$;
