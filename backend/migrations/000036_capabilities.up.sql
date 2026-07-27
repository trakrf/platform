-- TRA-1024 / ADR 0002: per-org capability grants. A capability is a named grant
-- that unlocks a use-case surface (routes + UI); a customer one-off is the same
-- thing with a grant count of one. Enforcement middleware is TRA-1025, frontend
-- gating TRA-1026, grant management TRA-1027 — this migration is schema only.
--
-- Asset management is NOT a capability. It is the always-on base every org gets
-- with zero grants, so a provisioning bug can degrade an org but never brick it.
-- Note `inventory` here means stock levels of fungible items — a module that
-- does not exist yet — not the Scan tab, which is part of that base.
--
-- Numbering: this is 000036, not 000034. 000034 is permanently unused (TRA-1043
-- landed 000035 first). golang-migrate tracks one integer version, so a file
-- numbered below an already-applied version is silently skipped, not errored —
-- filling the gap would leave preview and prod without these tables and no
-- warning. Leave the gap.
--
-- No GRANTs here: the infra init-grants Job owns privileges (ALTER DEFAULT
-- PRIVILEGES covers migrate-created tables and functions).
SET search_path = trakrf, public;

-- ── capability vocabulary (code-owned; the Go registry in TRA-1025 mirrors it) ─
-- Name-only by design (ADR 0002 §"Frontend"): presentation policy — absent vs.
-- locked — is a build-time frontend/spec concern. A policy column here would
-- advertise runtime flexibility that shipped bundles and published specs cannot
-- honor. Do not add columns.
CREATE TABLE capabilities (
    name TEXT PRIMARY KEY
);

INSERT INTO capabilities (name) VALUES ('inventory'), ('geofence'), ('mustering');

COMMENT ON TABLE capabilities IS
    'TRA-1024/ADR 0002: the capability vocabulary. Name-only — presentation policy is a build-time frontend/spec concern, never a DB column. Mirrored by the Go capability registry (TRA-1025); a test pins the two in sync.';

-- ── grants ────────────────────────────────────────────────────────────────
-- Grant = insert, revoke = delete, "which orgs have X" = a WHERE clause. The
-- FK to the lookup table is what keeps the vocabulary code-owned: an invented
-- or typo'd name cannot be granted. No expires_at / config jsonb yet — both
-- have an obvious home here and are additive at first real need.
CREATE TABLE org_capabilities (
    org_id     BIGINT      NOT NULL REFERENCES organizations(id),
    capability TEXT        NOT NULL REFERENCES capabilities(name),
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (org_id, capability)
);

-- No RLS, deliberately, and this is the one table where that needs saying.
-- Grants are entitlement metadata, not tenant data — the sibling entitlement
-- state (subscription_enabled / subscription_expires_at) likewise lives on
-- `organizations`, which has no RLS either. Two consumers need cross-org or
-- pre-org-context reach: the request middleware reads the set before org
-- context exists (via the SECURITY DEFINER function below), and superadmin
-- grant management (TRA-1027) writes rows for an org the acting principal is
-- not a member of. An org_id-isolation policy would block that write path
-- outright, for no confidentiality gain — a grant row names a surface, and
-- route existence is public anyway under BSL source availability.
COMMENT ON TABLE org_capabilities IS
    'TRA-1024/ADR 0002: per-org capability grants. Zero rows is the default and the norm — asset management is the ungated base. Grant = insert, revoke = delete; revocation takes effect on the next request (grants are never baked into tokens). No RLS: read pre-org-context by middleware, written cross-org by superadmin grant management.';

-- ── the read path ─────────────────────────────────────────────────────────
-- Sibling to trakrf.org_is_entitled: one indexed lookup per request, called by
-- middleware with NO org context set, hence SECURITY DEFINER. Returns an empty
-- array rather than NULL for an ungranted org so callers branch on membership,
-- never on nil — "loaded and empty" must not read as "not loaded".
--
-- Named org_capability_set, not org_capabilities, to avoid colliding with the
-- table name.
CREATE OR REPLACE FUNCTION trakrf.org_capability_set(p_org_id BIGINT)
RETURNS TEXT[]
LANGUAGE sql
STABLE
SECURITY DEFINER
SET search_path = trakrf, public
AS $$
    SELECT COALESCE(array_agg(oc.capability ORDER BY oc.capability), '{}'::TEXT[])
    FROM trakrf.org_capabilities oc
    WHERE oc.org_id = p_org_id;
$$;

COMMENT ON FUNCTION trakrf.org_capability_set(BIGINT) IS
    'TRA-1024: the org''s granted capability names, sorted, empty array (never NULL) when none. SECURITY DEFINER so the RLS-enforced app role can call it pre-org-context in middleware, same posture as org_is_entitled.';

-- No backfill. Every org — pre-existing and newly created, on both creation
-- paths — starts at zero grants; asset management is what they get. Segmentation
-- starts now rather than only for orgs created after this deploy. The known
-- exceptions (Frederick Health, demo orgs -> geofence) are granted by hand
-- post-deploy; an allowlist in a migration for a handful of rows is not it.
--
-- SEQUENCING: this must not reach production before TRA-1026 gates the nav, or
-- every existing org sees geofence UI that 403s underneath. TRA-1046 carries it.
