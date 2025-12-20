SET search_path=trakrf,public;

ALTER TABLE bulk_import_jobs DROP COLUMN IF EXISTS tags_created;
