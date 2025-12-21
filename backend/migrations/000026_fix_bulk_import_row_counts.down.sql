-- Revert to original constraint
ALTER TABLE trakrf.bulk_import_jobs DROP CONSTRAINT valid_row_counts;

ALTER TABLE trakrf.bulk_import_jobs ADD CONSTRAINT valid_row_counts
    CHECK (processed_rows <= total_rows AND failed_rows <= processed_rows);
