-- Location maps: a floor plan or site image on a location, and the positions
-- of the locations below it on that image.
--
-- location_maps holds one image per location, the parent that carries the
-- map (a site, a building, a floor). The image is stored in the database
-- rather than in object storage so the feature works the same on-prem and
-- offline as it does in the cloud. Size and type are capped here as well as
-- in the handler: PNG, JPEG or WebP, at most 5 MiB.
--
-- location_map_pins places a descendant location on its ancestor's map. The
-- position is a percentage of the image's width and height, not pixels, so
-- replacing the image with one at another resolution (same framing) keeps
-- every pin where it was. The position belongs to the location, not to any
-- reader: two gateways serving one room share that room's pin.
--
-- Locations are soft-deleted, so the location FKs rarely fire. Readers filter
-- out pins whose location has a deleted_at.
SET search_path = trakrf, public;

CREATE TABLE location_maps (
    location_id  BIGINT      PRIMARY KEY REFERENCES locations(id),
    org_id       BIGINT      NOT NULL REFERENCES organizations(id),
    content_type TEXT        NOT NULL CHECK (content_type IN ('image/png', 'image/jpeg', 'image/webp')),
    image        BYTEA       NOT NULL CHECK (octet_length(image) BETWEEN 1 AND 5242880),
    sha256       TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_location_maps_org ON location_maps(org_id);

CREATE TRIGGER update_location_maps_updated_at
    BEFORE UPDATE ON location_maps
    FOR EACH ROW
    EXECUTE FUNCTION trakrf.update_updated_at_column();

CREATE TABLE location_map_pins (
    map_location_id BIGINT           NOT NULL REFERENCES location_maps(location_id) ON DELETE CASCADE,
    location_id     BIGINT           NOT NULL REFERENCES locations(id),
    org_id          BIGINT           NOT NULL REFERENCES organizations(id),
    x_pct           DOUBLE PRECISION NOT NULL CHECK (x_pct >= 0 AND x_pct <= 100),
    y_pct           DOUBLE PRECISION NOT NULL CHECK (y_pct >= 0 AND y_pct <= 100),
    PRIMARY KEY (map_location_id, location_id),
    CONSTRAINT location_map_pins_not_self CHECK (map_location_id <> location_id)
);

CREATE INDEX idx_location_map_pins_org ON location_map_pins(org_id);

COMMENT ON TABLE location_maps IS 'One map image (floor plan, site plan) per location. Stored in-database so it works on-prem and offline.';
COMMENT ON COLUMN location_maps.sha256 IS 'Hex SHA-256 of image; the image endpoint''s ETag.';
COMMENT ON TABLE location_map_pins IS 'A descendant location''s position on an ancestor''s map, as a percentage of the image size.';

-- RLS: org isolation, USING-only, same shape as output_drives.
ALTER TABLE location_maps ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_location_maps ON location_maps
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

ALTER TABLE location_map_pins ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_location_map_pins ON location_map_pins
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
