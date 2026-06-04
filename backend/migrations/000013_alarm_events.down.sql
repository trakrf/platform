-- TRA-901 down: drop the alarm event log. Policy and indexes drop with the table.
SET search_path = trakrf, public;

DROP TABLE IF EXISTS alarm_events;
