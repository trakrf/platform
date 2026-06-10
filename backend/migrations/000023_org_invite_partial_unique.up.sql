-- TRA-973 — cancelled org invitation blocks re-invite.
--
-- Cancelling an invitation is a soft delete (sets cancelled_at, leaves the
-- row). The plain unique_org_email UNIQUE(org_id, email) constraint covered
-- every row, including cancelled/accepted ones, so re-inviting a previously
-- cancelled address collided and surfaced a 500. The constraint is too broad:
-- uniqueness should hold only over *live* invites (one open invite per
-- (org_id, email)). Replace it with a partial unique index that excludes
-- cancelled and accepted rows.
SET search_path = trakrf, public;

ALTER TABLE org_invitations DROP CONSTRAINT unique_org_email;

CREATE UNIQUE INDEX unique_org_email_live
    ON org_invitations (org_id, email)
    WHERE cancelled_at IS NULL AND accepted_at IS NULL;
