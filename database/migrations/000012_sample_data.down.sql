SET search_path=trakrf,public;

-- No-op: sample data cleanup handled by table CASCADE drops
-- This migration is reversible via down migrations 000011 -> 000001
--
-- Alternative approach (not used): Could delete sample data via CASCADE delete on accounts:
--   DELETE FROM accounts WHERE domain IN ('acme.com', 'techstart.io', 'research-lab.edu', 'trakrf.id') CASCADE;
-- However, this would gunk up the transaction log with DELETE operations for all dependent rows.
-- Cleaner to let table drops handle cleanup via CASCADE in earlier down migrations.
