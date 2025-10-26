SET search_path = trakrf,public;

DROP TRIGGER IF EXISTS trigger_process_identifier_scans ON identifier_scans;
DROP FUNCTION IF EXISTS process_identifier_scans();
