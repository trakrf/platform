SET search_path=trakrf,public;

-- Revert any rows using the expanded values before restoring the narrow CHECK.
-- Pre-launch data loss is acceptable per spec risks section.
UPDATE assets SET type = 'asset' WHERE type IN ('person', 'inventory');

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check
    CHECK (type = 'asset');

COMMENT ON COLUMN assets.type IS 'Optional asset type/classification (person, equipment, inventory, etc.)';
