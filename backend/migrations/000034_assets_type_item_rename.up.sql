SET search_path=trakrf,public;

-- TRA-540 §3.3: rename default asset type "asset" → "item" to eliminate
-- the tautological enum value (Asset(asset_type=AssetType.ASSET)).
-- Pre-launch posture: existing rows with type='asset' get translated to 'item'.

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

UPDATE assets SET type = 'item' WHERE type = 'asset';

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check
    CHECK (type IN ('item', 'person', 'inventory'));

COMMENT ON COLUMN assets.type IS 'Kind of tracked entity: one of ''item'', ''person'', ''inventory''. Reserved for future kind-specific behavior; currently stored and returned as-is.';
