SET search_path = trakrf, public;

DROP FUNCTION IF EXISTS trakrf.org_capability_set(BIGINT);

DROP TABLE IF EXISTS org_capabilities;
DROP TABLE IF EXISTS capabilities;
