-- TRA-720 — locations table. Post-rename (external_key, not identifier).
-- No ltree path/depth (dropped in legacy 000042). BIGINT throughout.

SET search_path = trakrf, public;

CREATE SEQUENCE location_seq AS BIGINT;

CREATE TABLE locations (
    id                  BIGINT PRIMARY KEY,
    org_id              BIGINT NOT NULL REFERENCES organizations(id),
    external_key        VARCHAR(255) NOT NULL,
    name                VARCHAR(255) NOT NULL,
    description         TEXT,
    parent_location_id  BIGINT REFERENCES locations(id),
    valid_from          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to            TIMESTAMPTZ DEFAULT NULL,
    is_active           BOOLEAN NOT NULL DEFAULT true,
    metadata            JSONB DEFAULT '{}',
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at          TIMESTAMPTZ,
    CONSTRAINT no_self_reference CHECK (id != parent_location_id)
);

CREATE INDEX idx_locations_org ON locations(org_id);
CREATE INDEX idx_locations_external_key ON locations(external_key);
CREATE INDEX idx_locations_parent ON locations(parent_location_id);
CREATE INDEX idx_locations_valid ON locations(valid_from, valid_to);
CREATE INDEX idx_locations_active ON locations(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX locations_org_id_external_key_unique
    ON locations(org_id, external_key) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_location_id_trigger
    BEFORE INSERT ON locations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id('location_seq');

CREATE TRIGGER update_locations_updated_at
    BEFORE UPDATE ON locations
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE locations ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_locations ON locations
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE locations IS 'Stores location information with temporal validity';
COMMENT ON COLUMN locations.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN locations.external_key IS 'External natural key for the location (unique per org for live rows)';
