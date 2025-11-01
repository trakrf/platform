SET search_path=trakrf,public;

-- Revert to unqualified sequence names (original broken state)
-- This is for rollback purposes only - the unqualified names will fail in practice

-- Revert user ID trigger
DROP TRIGGER IF EXISTS generate_user_id_trigger ON users;
CREATE TRIGGER generate_user_id_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('user_seq');

-- Revert organization ID trigger
DROP TRIGGER IF EXISTS generate_id_trigger ON organizations;
CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON organizations
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('organization_seq');

-- Revert location ID trigger
DROP TRIGGER IF EXISTS generate_location_id_trigger ON locations;
CREATE TRIGGER generate_location_id_trigger
    BEFORE INSERT ON locations
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('location_seq');

-- Revert scan_device ID trigger
DROP TRIGGER IF EXISTS generate_scan_device_id_trigger ON scan_devices;
CREATE TRIGGER generate_scan_device_id_trigger
    BEFORE INSERT ON scan_devices
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('scan_device_seq');

-- Revert scan_point ID trigger
DROP TRIGGER IF EXISTS generate_scan_point_id_trigger ON scan_points;
CREATE TRIGGER generate_scan_point_id_trigger
    BEFORE INSERT ON scan_points
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('scan_point_seq');

-- Revert asset ID trigger
DROP TRIGGER IF EXISTS generate_asset_id_trigger ON assets;
CREATE TRIGGER generate_asset_id_trigger
    BEFORE INSERT ON assets
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('asset_seq');

-- Revert identifier ID trigger
DROP TRIGGER IF EXISTS generate_identifier_id_trigger ON identifiers;
CREATE TRIGGER generate_identifier_id_trigger
    BEFORE INSERT ON identifiers
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('identifier_seq');

-- Revert bulk_import_job ID trigger
DROP TRIGGER IF EXISTS generate_bulk_import_job_id_trigger ON bulk_import_jobs;
CREATE TRIGGER generate_bulk_import_job_id_trigger
    BEFORE INSERT ON bulk_import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('bulk_import_job_seq');
