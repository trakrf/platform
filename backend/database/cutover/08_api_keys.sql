-- TRA-810 — pull api_keys. Natural key: jti (UUID).
-- Self-FK created_by_key_id requires two-phase, complicated by the
-- api_keys_creator_exactly_one CHECK which forbids both NULL.
-- Phase 1: insert user-rooted keys only (created_by IS NOT NULL).
-- Phase 2: assert no key-rooted children on source; if present, abort.
\set ON_ERROR_STOP on

-- Phase 1: user-rooted keys.
INSERT INTO trakrf.api_keys
    (jti, org_id, name, scopes, created_by, created_by_key_id,
     created_at, expires_at, last_used_at, revoked_at)
SELECT
    s.jti, t_org.id, s.name, s.scopes, t_usr.id, NULL,
    s.created_at, s.expires_at, s.last_used_at, s.revoked_at
FROM cloud_src.api_keys s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN cloud_src.users src_usr         ON src_usr.id = s.created_by
JOIN trakrf.organizations t_org      ON t_org.identifier = src_org.identifier
JOIN trakrf.users t_usr              ON t_usr.email = src_usr.email
WHERE src_org.deleted_at IS NULL
  AND src_usr.deleted_at IS NULL
  AND s.created_by IS NOT NULL;

-- Phase 2: assert no key-rooted children on source. If any exist we punt.
DO $$
DECLARE child_n INT;
BEGIN
    SELECT count(*) INTO child_n FROM cloud_src.api_keys
        WHERE created_by IS NULL AND created_by_key_id IS NOT NULL;
    IF child_n > 0 THEN
        RAISE EXCEPTION 'api_keys: % key-rooted children on source — extend 08_api_keys.sql to handle them (drop+re-add CHECK, or two-phase)', child_n;
    END IF;
END $$;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.api_keys s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        JOIN cloud_src.users u ON u.id = s.created_by AND u.deleted_at IS NULL
        WHERE s.created_by IS NOT NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.api_keys;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'api_keys mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'api_keys OK: % rows (user-rooted only)', tgt_n;
END $$;
