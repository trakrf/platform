-- One row per output-device drive: which device, which edge, why it was
-- driven, and whether the drive worked.
--
-- alarm_events records that a rule fired for a read (detection). It does not
-- say which devices were driven or whether the drive reached them: one fire
-- drives every device at the location, each suppressed or driven on its own,
-- and a drive to an offline reader fails with only a log line to show for it.
-- These rows are the delivery record: an alarm's drive rows say which devices
-- were reached. They are best-effort writes, so a missing row is a lost
-- record, not a missed drive.
--
-- result:
--   ok     the device answered (http transport)
--   sent   handed to the MQTT broker, which is all an mqtt transport hears: a
--          cs463 gpo drive is fire-and-forget, and a powered-off reader still
--          lets the publish succeed. Not proof the output moved.
--   error  the drive failed; error says how
--
-- cause says what drove it:
--   alarm    the ON a fire sent; alarm_event_id names the fire, unless the
--            alarm row itself failed to write
--   refresh  a presence output's periodic re-ON while a tag stays in view
--   age_out  a presence output's OFF when no member has been read for its
--            age-out
--   test     the output-device test action (ON, then OFF)
--   reset    the output-device reset action (OFF)
--   delete   the OFF sent before an output device is deleted
--
-- output_device_id is the device as it was at drive time. Output devices are
-- soft-deleted, so the FK never blocks a delete.
--
-- Regular table, not a hypertable, like alarm_events. Volume is bounded by
-- fires plus one refresh per held presence output every 10 s.
SET search_path = trakrf, public;

CREATE TABLE output_drives (
    id               BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    org_id           BIGINT      NOT NULL REFERENCES organizations(id),
    output_device_id BIGINT      NOT NULL REFERENCES output_devices(id),
    alarm_event_id   BIGINT      REFERENCES alarm_events(id),
    edge             TEXT        NOT NULL CHECK (edge IN ('on', 'off')),
    cause            TEXT        NOT NULL
        CHECK (cause IN ('alarm', 'refresh', 'age_out', 'test', 'reset', 'delete')),
    result           TEXT        NOT NULL CHECK (result IN ('ok', 'sent', 'error')),
    error            TEXT,
    latency_ms       INT         NOT NULL CHECK (latency_ms >= 0),
    driven_at        TIMESTAMPTZ NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT output_drives_error_iff_failed CHECK ((result = 'error') = (error IS NOT NULL)),
    CONSTRAINT output_drives_alarm_only_for_alarm_cause CHECK (alarm_event_id IS NULL OR cause = 'alarm')
);

-- The per-output activity log: one device's drives, newest first.
CREATE INDEX idx_output_drives_device_time ON output_drives(org_id, output_device_id, driven_at DESC);
-- An alarm's delivery: its drive rows.
CREATE INDEX idx_output_drives_alarm ON output_drives(alarm_event_id) WHERE alarm_event_id IS NOT NULL;

COMMENT ON TABLE output_drives IS 'One row per output-device drive and its result. The delivery record for alarm_events.';
COMMENT ON COLUMN output_drives.error IS 'Transport error text when result = error (HTTP failure, broker publish failure, device config error), capped at 1000 bytes.';
COMMENT ON COLUMN output_drives.result IS 'ok = the device answered; sent = handed to the MQTT broker, unconfirmed; error = failed.';
COMMENT ON COLUMN output_drives.latency_ms IS 'Time the drive call took, success or failure.';

-- RLS: org isolation, USING-only, identical shape to alarm_events.
ALTER TABLE output_drives ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_output_drives ON output_drives
    USING (org_id = current_setting('app.current_org_id')::BIGINT);
