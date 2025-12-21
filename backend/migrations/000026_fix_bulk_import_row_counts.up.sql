-- Fix the valid_row_counts constraint
-- The old constraint (failed_rows <= processed_rows) fails when all rows fail
-- because processed_rows=0 and failed_rows=N violates the constraint.
--
-- The new constraint:
-- - processed_rows = successful inserts only
-- - failed_rows = failed inserts
-- - processed_rows + failed_rows <= total_rows (can't process more than total)

-- First, drop the old constraint
ALTER TABLE trakrf.bulk_import_jobs DROP CONSTRAINT valid_row_counts;

-- Fix existing data where processed_rows + failed_rows > total_rows
-- These were incorrectly set; processed_rows should be successCount only
UPDATE trakrf.bulk_import_jobs
SET processed_rows = total_rows - failed_rows
WHERE processed_rows + failed_rows > total_rows AND failed_rows <= total_rows;

-- For any remaining edge cases where both exceed total, cap them
UPDATE trakrf.bulk_import_jobs
SET processed_rows = 0, failed_rows = total_rows
WHERE processed_rows + failed_rows > total_rows;

-- Add the corrected constraint
ALTER TABLE trakrf.bulk_import_jobs ADD CONSTRAINT valid_row_counts
    CHECK (processed_rows >= 0 AND failed_rows >= 0 AND processed_rows + failed_rows <= total_rows);
