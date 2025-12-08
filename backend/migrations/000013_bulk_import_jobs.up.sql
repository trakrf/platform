SET search_path=trakrf,public;

-- Create sequence for ID generation (used by trigger)
CREATE SEQUENCE bulk_import_job_seq;

CREATE TABLE bulk_import_jobs (
    id INT PRIMARY KEY,
    org_id INT NOT NULL REFERENCES organizations(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows INT NOT NULL DEFAULT 0,
    processed_rows INT NOT NULL DEFAULT 0,
    failed_rows INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (processed_rows <= total_rows AND failed_rows <= processed_rows)
);

-- Use the same ID generation pattern as assets (permuted sequence for obfuscation)
CREATE TRIGGER generate_bulk_import_job_id_trigger
    BEFORE INSERT ON bulk_import_jobs
    FOR EACH ROW
    EXECUTE FUNCTION generate_permuted_id('bulk_import_job_seq');

-- Indexes for common access patterns
CREATE INDEX idx_bulk_import_jobs_org_id ON bulk_import_jobs(org_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);

-- Row Level Security
ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY org_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (org_id = current_setting('app.current_org_id')::INT);

-- Documentation
COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.status IS 'Job status: pending, processing, completed, failed';
COMMENT ON COLUMN bulk_import_jobs.errors IS 'Array of error objects: [{row: int, field: string, error: string}]';
