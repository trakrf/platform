-- TRA-720 — tags table (was 'identifiers' in legacy 000009, renamed 000033).
-- Mutually exclusive asset_id or location_id (tag_target check constraint).
-- Partial unique on (org_id, type, value) WHERE deleted_at IS NULL.

SET search_path = trakrf, public;

CREATE TABLE tags (
    id              BIGINT PRIMARY KEY,
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    type            VARCHAR(50) NOT NULL,
    value           VARCHAR(255) NOT NULL,
    asset_id        BIGINT REFERENCES assets(id),
    location_id     BIGINT REFERENCES locations(id),
    valid_from      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    valid_to        TIMESTAMPTZ DEFAULT NULL,
    is_active       BOOLEAN NOT NULL DEFAULT true,
    metadata        JSONB DEFAULT '{}',
    created_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    deleted_at      TIMESTAMPTZ,

    CONSTRAINT tag_target CHECK (
        (asset_id IS NOT NULL AND location_id IS NULL) OR
        (asset_id IS NULL AND location_id IS NOT NULL)
    )
);

CREATE INDEX idx_tags_org ON tags(org_id);
CREATE INDEX idx_tags_asset ON tags(asset_id);
CREATE INDEX idx_tags_location ON tags(location_id);
CREATE INDEX idx_tags_value ON tags(value);
CREATE INDEX idx_tags_valid ON tags(valid_from, valid_to);
CREATE INDEX idx_tags_type ON tags(type);
CREATE INDEX idx_tags_active ON tags(is_active) WHERE is_active = true;
CREATE UNIQUE INDEX tags_org_id_type_value_unique
    ON tags(org_id, type, value) WHERE deleted_at IS NULL;

CREATE TRIGGER generate_tag_id_trigger
    BEFORE INSERT ON tags
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_tags_updated_at
    BEFORE UPDATE ON tags
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE tags ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_tags ON tags
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE tags IS 'Stores physical/logical tags (RFID, BLE, NFC, barcode, serial, etc.) with temporal validity';
COMMENT ON COLUMN tags.id IS 'Primary key - keyed Feistel ID';
COMMENT ON COLUMN tags.type IS 'Tag type: rfid, ble, nfc, barcode, serial, mac, qr, etc.';
COMMENT ON COLUMN tags.value IS 'The actual tag value (EPC, MAC address, NFC UID, barcode digits, etc.)';
