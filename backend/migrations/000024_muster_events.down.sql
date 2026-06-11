-- TRA-978 — drop muster_events tables (down migration).
SET search_path = trakrf, public;

DROP TABLE IF EXISTS muster_event_entries;
DROP TABLE IF EXISTS muster_events;
