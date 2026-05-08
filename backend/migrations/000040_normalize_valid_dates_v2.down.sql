SET search_path=trakrf,public;

-- TRA-624 is one-way data cleanup: destroyed sentinel values cannot be
-- reconstructed. Down migration is intentionally a no-op.
SELECT 1;
