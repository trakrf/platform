SET search_path=trakrf,public;

-- Sequence for ID generation
CREATE SEQUENCE account_seq;

-- Account table
CREATE TABLE accounts (
                          id INT PRIMARY KEY,
                          name VARCHAR(255) NOT NULL,
                          domain VARCHAR(255) UNIQUE,
                          status VARCHAR(20) NOT NULL DEFAULT 'active',
                          subscription_tier VARCHAR(50) NOT NULL DEFAULT 'free',
                          max_users INTEGER NOT NULL DEFAULT 5,
                          max_storage_gb INTEGER NOT NULL DEFAULT 1,
                          settings JSONB DEFAULT '{}',
                          metadata JSONB DEFAULT '{}',
                          billing_email VARCHAR(255) NOT NULL,
                          technical_email VARCHAR(255),
                          valid_from TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          valid_to TIMESTAMPTZ DEFAULT NULL,
                          is_active BOOLEAN NOT NULL DEFAULT true,
                          created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          updated_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
                          deleted_at TIMESTAMPTZ
);

-- Indexes for common lookups
CREATE INDEX idx_account_domain ON accounts(domain);
CREATE INDEX idx_account_status ON accounts(status);

-- Create the insert trigger
CREATE TRIGGER generate_id_trigger
    BEFORE INSERT ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION generate_hashed_id('account_seq');

-- Create the update trigger
CREATE TRIGGER update_accounts_updated_at
    BEFORE UPDATE ON accounts
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();
