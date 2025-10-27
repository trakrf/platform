SET search_path=trakrf,public;

CREATE TABLE bulk_import_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id INT NOT NULL REFERENCES accounts(id),
    status TEXT NOT NULL CHECK (status IN ('pending', 'processing', 'completed', 'failed')),
    total_rows INT NOT NULL DEFAULT 0,
    processed_rows INT NOT NULL DEFAULT 0,
    failed_rows INT NOT NULL DEFAULT 0,
    errors JSONB NOT NULL DEFAULT '[]'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    CONSTRAINT valid_row_counts CHECK (processed_rows <= total_rows AND failed_rows <= processed_rows)
);

-- Indexes for common access patterns
CREATE INDEX idx_bulk_import_jobs_account_id ON bulk_import_jobs(account_id);
CREATE INDEX idx_bulk_import_jobs_status ON bulk_import_jobs(status);
CREATE INDEX idx_bulk_import_jobs_created_at ON bulk_import_jobs(created_at DESC);

-- Row Level Security
ALTER TABLE bulk_import_jobs ENABLE ROW LEVEL SECURITY;

CREATE POLICY account_isolation_bulk_import_jobs ON bulk_import_jobs
    USING (account_id = current_setting('app.current_account_id')::INT);

-- Documentation
COMMENT ON TABLE bulk_import_jobs IS 'Tracks async bulk import operations for assets';
COMMENT ON COLUMN bulk_import_jobs.status IS 'Job status: pending, processing, completed, failed';
COMMENT ON COLUMN bulk_import_jobs.errors IS 'Array of error objects: [{row: int, field: string, error: string}]';
