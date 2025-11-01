SET search_path=trakrf,public;

-- Fix sequence references in triggers for existing databases
-- This migration drops and recreates all ID generation triggers with fully-qualified sequence names
-- to resolve "relation does not exist" errors when sequences are in trakrf schema

-- Drop and recreate user ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_user_id_trigger ON users;
CREATE TRIGGER generate_user_id_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('trakrf.user_seq');

-- Drop and recreate organization ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_id_trigger ON organizations;
CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('trakrf.organization_seq');

-- Drop and recreate location ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_location_id_trigger ON locations;
CREATE TRIGGER generate_location_id_trigger
    BEFORE INSERT ON locations
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.location_seq');

-- Drop and recreate scan_device ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_scan_device_id_trigger ON scan_devices;
CREATE TRIGGER generate_scan_device_id_trigger
    BEFORE INSERT ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.scan_device_seq');

-- Drop and recreate scan_point ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_scan_point_id_trigger ON scan_points;
CREATE TRIGGER generate_scan_point_id_trigger
    BEFORE INSERT ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.scan_point_seq');

-- Drop and recreate asset ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_asset_id_trigger ON assets;
CREATE TRIGGER generate_asset_id_trigger
    BEFORE INSERT ON assets
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.asset_seq');

-- Drop and recreate identifier ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_identifier_id_trigger ON identifiers;
CREATE TRIGGER generate_identifier_id_trigger
    BEFORE INSERT ON identifiers
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.identifier_seq');

-- Drop and recreate bulk_import_job ID trigger with qualified sequence name
DROP TRIGGER IF EXISTS generate_bulk_import_job_id_trigger ON bulk_import_jobs;
CREATE TRIGGER generate_bulk_import_job_id_trigger
    BEFORE INSERT ON bulk_import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('trakrf.bulk_import_job_seq');
