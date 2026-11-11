-- A location change can record an asset passing a gate. The origin is
-- reader_crossing and, like a reader move, it names the scan point that saw it.

SET search_path = trakrf, public;

-- The origin check was declared inline, so its name was generated.
DO $$
DECLARE
    con text;
BEGIN
    SELECT c.conname INTO con
    FROM pg_constraint c
    WHERE c.conrelid = 'trakrf.location_changes'::regclass
      AND c.contype = 'c'
      AND pg_get_constraintdef(c.oid) LIKE '%manual_override%'
      AND pg_get_constraintdef(c.oid) NOT LIKE '%scan_point_id%';
    IF con IS NULL THEN
        RAISE EXCEPTION 'location_changes origin check not found';
    END IF;
    EXECUTE format('ALTER TABLE location_changes DROP CONSTRAINT %I', con);
END $$;

ALTER TABLE location_changes
    DROP CONSTRAINT location_changes_scan_point_matches_origin;

ALTER TABLE location_changes
    ADD CONSTRAINT location_changes_origin_check
        CHECK (origin IN ('reader', 'reader_crossing', 'manual_save', 'manual_override')),
    ADD CONSTRAINT location_changes_scan_point_matches_origin
        CHECK ((origin IN ('reader', 'reader_crossing')) = (scan_point_id IS NOT NULL));

COMMENT ON COLUMN location_changes.origin IS
    'Which path detected the move: reader (fixed reader), reader_crossing (asset passed a gate), manual_save (handheld save), manual_override (same-minute correction of a reader row)';
