SET search_path=trakrf,public;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

ALTER TABLE assets
    ALTER COLUMN type DROP NOT NULL,
    ALTER COLUMN type DROP DEFAULT;
