-- Reverse TRA-973: restore the plain unique constraint over all rows.
-- Note: this can fail if duplicate cancelled/accepted rows have accumulated
-- for the same (org_id, email) — expected, since those are exactly what the
-- partial index was added to allow.
SET search_path = trakrf, public;

DROP INDEX IF EXISTS unique_org_email_live;

ALTER TABLE org_invitations
    ADD CONSTRAINT unique_org_email UNIQUE(org_id, email);
