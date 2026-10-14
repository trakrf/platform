-- notification_deliveries.event_id becomes BIGINT.
--
-- 000048 added it as TEXT. An event is a row with a BIGINT id, and one event
-- has many deliveries (one per recipient and channel), so event_id is that
-- row's id, repeated on each of its deliveries. It is deliberately not unique;
-- delivery_id is. The foreign key is added with the events table.
--
-- Nothing writes event_id before this migration, so the cast never sees a
-- non-numeric value.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries
    ALTER COLUMN event_id TYPE BIGINT USING event_id::BIGINT;

COMMENT ON COLUMN notification_deliveries.event_id IS
    'The detected event this delivery came from; one event fans out to many deliveries, so not unique';
