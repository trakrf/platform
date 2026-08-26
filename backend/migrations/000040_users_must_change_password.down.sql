-- TRA-1135 down: drop the forced-rotation flag.
--
-- Dropping it un-gates anyone currently held at the change-password screen, so
-- rolling back is a decision about live accounts, not just about schema.
ALTER TABLE trakrf.users
    DROP COLUMN IF EXISTS must_change_password;
