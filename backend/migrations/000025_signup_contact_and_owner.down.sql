-- Reverse TRA-971/TRA-970 signup contact + owner columns.
SET search_path = trakrf, public;

ALTER TABLE organizations DROP COLUMN IF EXISTS owner_user_id;
ALTER TABLE organizations DROP COLUMN IF EXISTS website;
ALTER TABLE users         DROP COLUMN IF EXISTS phone;
