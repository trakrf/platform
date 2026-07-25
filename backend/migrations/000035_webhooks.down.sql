-- TRA-1043 rollback. The triggers and the RLS policy are dropped with the table.
SET search_path = trakrf, public;

DROP TABLE IF EXISTS webhooks;
