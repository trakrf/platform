-- TRA-978 — muster events + entries. POC-grade persistence for mustering
-- drills. Muster events are org-scoped; one active event per org at a time
-- (partial unique index). Entries snapshot the expected-person set at drill
-- trigger time. Both tables follow house conventions: Feistel-obfuscated id
-- off the shared trakrf.id_seq (TRA-886), updated_at trigger,
-- org-scoped RLS (identical shape to alarm_devices / 000014).
-- No in-migration GRANTs: the infra init-grants Job sets ALTER DEFAULT
-- PRIVILEGES for the migrate role, and the integration harness grants CRUD ON
-- ALL TABLES to the RLS role post-migrate.
SET search_path = trakrf, public;

-- ── muster_events ─────────────────────────────────────────────────────────
CREATE TABLE muster_events (
    id             BIGINT PRIMARY KEY,
    org_id         BIGINT NOT NULL REFERENCES organizations(id),
    status         VARCHAR(20) NOT NULL DEFAULT 'active'
                       CHECK (status IN ('active', 'completed', 'cancelled')),
    started_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    ended_at       TIMESTAMPTZ,
    window_minutes INT NOT NULL DEFAULT 15,
    started_by     BIGINT REFERENCES users(id),
    report         JSONB,
    metadata       JSONB NOT NULL DEFAULT '{}',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- One active event per org at a time.
CREATE UNIQUE INDEX muster_events_one_active_per_org
    ON muster_events (org_id) WHERE status = 'active';

CREATE INDEX idx_muster_events_org_started
    ON muster_events (org_id, started_at DESC);

CREATE TRIGGER generate_muster_event_id_trigger
    BEFORE INSERT ON muster_events
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_muster_events_updated_at
    BEFORE UPDATE ON muster_events
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE muster_events ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_muster_events ON muster_events
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE muster_events IS 'TRA-978: mustering drill events. One active event per org at a time. POC-grade.';
COMMENT ON COLUMN muster_events.window_minutes IS 'Presence window used when snapshotting the expected set at drill start.';
COMMENT ON COLUMN muster_events.report IS 'JSON summary computed at all-clear: total_seconds, counts, zones, muster_points.';
COMMENT ON COLUMN muster_events.metadata IS 'Break-glass unlock log stored as metadata.unlocks: [{user_id,email,at}].';

-- ── muster_event_entries ──────────────────────────────────────────────────
CREATE TABLE muster_event_entries (
    id                   BIGINT PRIMARY KEY,
    org_id               BIGINT NOT NULL REFERENCES organizations(id),
    muster_event_id      BIGINT NOT NULL REFERENCES muster_events(id) ON DELETE CASCADE,
    asset_id             BIGINT NOT NULL REFERENCES assets(id),
    label                VARCHAR(255) NOT NULL,
    expected_location_id BIGINT REFERENCES locations(id),
    status               VARCHAR(20) NOT NULL DEFAULT 'missing'
                             CHECK (status IN ('missing', 'at_muster', 'verified', 'safe_manual')),
    muster_location_id   BIGINT REFERENCES locations(id),
    first_muster_seen_at TIMESTAMPTZ,
    verified_by          BIGINT REFERENCES users(id),
    verified_at          TIMESTAMPTZ,
    marked_safe_by       BIGINT REFERENCES users(id),
    marked_safe_at       TIMESTAMPTZ,
    marked_safe_note     TEXT,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (muster_event_id, asset_id)
);

CREATE INDEX idx_muster_event_entries_event
    ON muster_event_entries (muster_event_id);
CREATE INDEX idx_muster_event_entries_org_event
    ON muster_event_entries (org_id, muster_event_id);

CREATE TRIGGER generate_muster_event_entry_id_trigger
    BEFORE INSERT ON muster_event_entries
    FOR EACH ROW EXECUTE FUNCTION trakrf.generate_obfuscated_id();

CREATE TRIGGER update_muster_event_entries_updated_at
    BEFORE UPDATE ON muster_event_entries
    FOR EACH ROW EXECUTE FUNCTION trakrf.update_updated_at_column();

ALTER TABLE muster_event_entries ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_muster_event_entries ON muster_event_entries
    USING (org_id = current_setting('app.current_org_id')::BIGINT);

COMMENT ON TABLE muster_event_entries IS 'TRA-978: person-level entry for a mustering drill. Snapshotted at drill start; status transitions missing→at_muster→verified/safe_manual.';
COMMENT ON COLUMN muster_event_entries.label IS 'Person label snapshotted from asset.name at drill start.';
COMMENT ON COLUMN muster_event_entries.expected_location_id IS 'Zone where the person was last seen at drill start (nullable if no recent sighting).';
COMMENT ON COLUMN muster_event_entries.muster_location_id IS 'Muster point where the person was first scanned after drill start.';
COMMENT ON COLUMN muster_event_entries.verified_by IS 'User id of the operator who pressed Verify.';
COMMENT ON COLUMN muster_event_entries.marked_safe_by IS 'User id of the operator who pressed Mark Safe.';
