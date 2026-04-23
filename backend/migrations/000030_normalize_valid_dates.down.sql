SET search_path=trakrf,public;

-- TRA-468 is one-way data cleanup: destroyed zero-time and far-future sentinels
-- cannot be reconstructed. Down migration is intentionally a no-op.
SELECT 1;
