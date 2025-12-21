SET search_path=trakrf,public;

-- Add tags_created column to bulk_import_jobs table
ALTER TABLE bulk_import_jobs ADD COLUMN tags_created INT NOT NULL DEFAULT 0;

COMMENT ON COLUMN bulk_import_jobs.tags_created IS 'Number of tag identifiers created during the import';
