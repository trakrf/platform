SET search_path=trakrf,public;

-- Sequence for ID generation
CREATE SEQUENCE user_seq;

CREATE TABLE users (
                       id INT PRIMARY KEY,
                       email VARCHAR(255) NOT NULL,
                       name VARCHAR(255) NOT NULL,
                       last_login_at TIMESTAMPTZ,
                       password_hash VARCHAR(255),
                       settings JSONB DEFAULT '{}',
                       metadata JSONB DEFAULT '{}',
                       created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                       deleted_at TIMESTAMPTZ
);

-- Indexes for common queries
CREATE UNIQUE INDEX idx_users_email ON users(email);

-- Create the insert trigger
CREATE TRIGGER generate_user_id_trigger
    BEFORE INSERT ON users
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('user_seq');

-- Trigger for updated_at
CREATE TRIGGER update_users_updated_at
    BEFORE UPDATE ON users
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

ALTER TABLE users ENABLE ROW LEVEL SECURITY;

CREATE POLICY user_isolation_users ON users
    USING (id = current_setting('app.current_user_id')::INT);

-- Add comments for documentation
COMMENT ON TABLE users IS 'Stores users associated with accounts in the SaaS application';
COMMENT ON COLUMN users.id IS 'Primary key - hashed ID';
