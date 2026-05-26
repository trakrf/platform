-- TRA-810 — pull org_invitations. Natural key: token. id is BIGSERIAL (auto-assigned).
\set ON_ERROR_STOP on

INSERT INTO trakrf.org_invitations
    (org_id, email, role, token, invited_by, expires_at, accepted_at,
     cancelled_at, created_at)
SELECT
    t_org.id, s.email, s.role, s.token, t_usr.id, s.expires_at, s.accepted_at,
    s.cancelled_at, s.created_at
FROM cloud_src.org_invitations s
JOIN cloud_src.organizations src_org   ON src_org.id = s.org_id
LEFT JOIN cloud_src.users src_usr      ON src_usr.id = s.invited_by
JOIN trakrf.organizations t_org        ON t_org.identifier = src_org.identifier
LEFT JOIN trakrf.users t_usr           ON t_usr.email = src_usr.email
WHERE src_org.deleted_at IS NULL
  AND (src_usr.id IS NULL OR src_usr.deleted_at IS NULL);

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.org_invitations s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE (s.invited_by IS NULL
               OR EXISTS (SELECT 1 FROM cloud_src.users u
                            WHERE u.id = s.invited_by AND u.deleted_at IS NULL));
    SELECT count(*) INTO tgt_n FROM trakrf.org_invitations;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'org_invitations mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'org_invitations OK: % rows', tgt_n;
END $$;
