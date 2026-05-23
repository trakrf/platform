SET search_path = trakrf,public;

-- TRA-816 — no rollback. The up-migration soft-deletes tag rows whose parent
-- asset or location was already soft-deleted; the original deleted_at value
-- on those rows was unrecoverable (NULL) before the sweep, so there is no
-- safe way to distinguish swept rows from rows soft-deleted by normal user
-- action between the sweep and the rollback. Down is intentionally a no-op.
