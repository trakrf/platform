-- TRA-810 — pull users and org_users.
-- users.last_org_id resolved via organizations.identifier.
-- org_users FKs resolved via org.identifier + user.email.
\set ON_ERROR_STOP on

INSERT INTO trakrf.users
    (email, name, last_login_at, password_hash, settings, metadata,
     is_superadmin, last_org_id, created_at, updated_at)
SELECT
    s.email, s.name, s.last_login_at, s.password_hash, s.settings, s.metadata,
    s.is_superadmin,
    t_org.id,                 -- map source.last_org_id → target.organizations.id via identifier
    s.created_at, s.updated_at
FROM cloud_src.users s
LEFT JOIN cloud_src.organizations src_org ON src_org.id = s.last_org_id
LEFT JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.users WHERE deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.users;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'users count mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'users OK: % rows', tgt_n;
END $$;

INSERT INTO trakrf.org_users
    (org_id, user_id, role, status, last_login_at, settings, metadata,
     created_at, updated_at)
SELECT
    t_org.id, t_usr.id, s.role, s.status, s.last_login_at, s.settings, s.metadata,
    s.created_at, s.updated_at
FROM cloud_src.org_users s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN cloud_src.users src_usr ON src_usr.id = s.user_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
JOIN trakrf.users t_usr ON t_usr.email = src_usr.email
WHERE s.deleted_at IS NULL
  AND src_org.deleted_at IS NULL
  AND src_usr.deleted_at IS NULL;

DO $$
DECLARE src_n INT; tgt_n INT;
BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.org_users s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.user_id AND u.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.org_users;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'org_users count mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'org_users OK: % rows', tgt_n;
END $$;
