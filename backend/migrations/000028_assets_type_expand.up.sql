SET search_path=trakrf,public;

ALTER TABLE assets DROP CONSTRAINT IF EXISTS assets_type_check;

ALTER TABLE assets
    ADD CONSTRAINT assets_type_check
    CHECK (type IN ('asset', 'person', 'inventory'));

COMMENT ON COLUMN assets.type IS 'Kind of tracked entity: one of ''asset'', ''person'', ''inventory''. Reserved for future kind-specific behavior; currently stored and returned as-is.';
