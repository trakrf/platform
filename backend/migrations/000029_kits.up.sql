-- TRA-1032: kits — expected-together asset groups (commission / verify / lookup).
-- Internal-only feature. No GRANTs here: the infra init-grants job owns privileges
-- (ALTER DEFAULT PRIVILEGES covers migrate-created tables).
SET search_path = trakrf, public;

CREATE TABLE kits (
    id BIGINT PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    label TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'closed')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TRIGGER generate_kit_id_trigger
    BEFORE INSERT ON kits
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_kits_updated_at
    BEFORE UPDATE ON kits
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

CREATE INDEX idx_kits_org_label ON kits (org_id, label);

ALTER TABLE kits ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_kits ON kits
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

-- removed_at is future-proofing only: NULL = active member; no code path writes it today.
-- The invariant "an asset is an active member of at most one ACTIVE kit" is enforced
-- app-level inside the commission transaction: a partial unique index on (asset_id)
-- WHERE removed_at IS NULL would keep blocking after the owning kit closes, because
-- kits.status cannot appear in the index predicate. Follow-up may add a trigger guard.
CREATE TABLE kit_members (
    id BIGINT PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    kit_id BIGINT NOT NULL REFERENCES kits(id) ON DELETE CASCADE,
    asset_id BIGINT NOT NULL REFERENCES assets(id),
    role TEXT,
    added_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    removed_at TIMESTAMPTZ
);

CREATE TRIGGER generate_kit_member_id_trigger
    BEFORE INSERT ON kit_members
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE INDEX idx_kit_members_kit_id ON kit_members (kit_id) WHERE removed_at IS NULL;
CREATE INDEX idx_kit_members_asset_id ON kit_members (asset_id) WHERE removed_at IS NULL;

ALTER TABLE kit_members ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_kit_members ON kit_members
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

-- Append-only audit of dock checks. Asset-id arrays are acceptable at demo scale;
-- deliberately not normalized (TRA-1032).
CREATE TABLE kit_verifications (
    id BIGINT PRIMARY KEY,
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    kit_id BIGINT NOT NULL REFERENCES kits(id) ON DELETE CASCADE,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    result TEXT NOT NULL CHECK (result IN ('complete', 'incomplete')),
    seen_asset_ids BIGINT[] NOT NULL DEFAULT '{}',
    missing_asset_ids BIGINT[] NOT NULL DEFAULT '{}',
    unexpected_asset_ids BIGINT[] NOT NULL DEFAULT '{}'
);

CREATE TRIGGER generate_kit_verification_id_trigger
    BEFORE INSERT ON kit_verifications
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE INDEX idx_kit_verifications_kit_latest ON kit_verifications (kit_id, verified_at DESC);

ALTER TABLE kit_verifications ENABLE ROW LEVEL SECURITY;
CREATE POLICY org_isolation_kit_verifications ON kit_verifications
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
