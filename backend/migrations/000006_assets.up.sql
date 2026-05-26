-- TRA-720 — assets table. Post-rename (external_key, not identifier).
-- No current_location_id (dropped 000043). No type column (dropped 000035).

SET search_path = trakrf, public;

CREATE SEQUENCE asset_seq AS BIGINT;

CREATE TABLE assets (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    external_key    VARCHAR(255) NOT NULL,
    name            VARCHAR(255) NOT NULL,
    description     TEXT,
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ
    -- Note: legacy 000031 (TRA-475) explicitly dropped the 3-column
    -- UNIQUE(org_id, external_key, valid_from) in favor of the partial
    -- unique index below. Do NOT re-add it here.
);

CREATE INDEX idx_assets_org ON assets(org_id);
CREATE INDEX idx_assets_external_key ON assets(external_key);
CREATE INDEX idx_assets_valid ON assets(valid_from, valid_to);
CREATE INDEX idx_assets_active ON assets(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX assets_org_id_external_key_unique
    ON assets(org_id, external_key) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_asset_id_trigger
    BEFORE INSERT ON assets
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('asset_seq');

CREATE TRIGGER update_assets_updated_at
    BEFORE UPDATE ON assets
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE assets ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_assets ON assets
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE assets IS 'Stores tracked assets with temporal validity';
COMMENT ON COLUMN assets.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN assets.external_key IS 'External natural key for the asset (caller-supplied or auto-generated ASSET-NNNN, unique per org for live rows)';
