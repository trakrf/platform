SET search_path=trakrf,public;

UPDATE assets SET type = 'asset';

ALTER TABLE assets
    ALTER COLUMN type SET DEFAULT 'asset',
    ALTER COLUMN type SET NOT NULL;

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check CHECK (type = 'asset');
