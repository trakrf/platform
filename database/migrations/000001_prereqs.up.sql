CREATE EXTENSION IF NOT EXISTS timescaledb;

CREATE SCHEMA IF NOT EXISTS trakrf;

-- function to generate psuedo-random keys using a hash strategy
CREATE OR REPLACE FUNCTION generate_hashed_id()
RETURNS TRIGGER AS $$
DECLARE
    secret TEXT := 'your-application-secret-key';  -- todo: replace with secret key
    seq_name TEXT := TG_ARGV[0];  -- Get sequence name from trigger definition
    seq_id INT;
    hashed_id INT;
BEGIN
    SELECT nextval(seq_name) INTO seq_id;
    hashed_id := (('x' || md5(seq_id::text || secret))::bit(32)::int) & 2147483647;
    NEW.id := hashed_id;
    RETURN NEW;
EXCEPTION
   WHEN OTHERS THEN
       RAISE WARNING 'Failed to generate hashed key: %', SQLERRM;
       RAISE;
END;
$$ LANGUAGE plpgsql;

-- function to generate psuedo-random keys using a permutation strategy
CREATE OR REPLACE FUNCTION trakrf.generate_permuted_id()
RETURNS TRIGGER AS $$
DECLARE
    seq_name TEXT := TG_ARGV[0];  -- Get sequence name from trigger definition
    account_id BIGINT := 1304140453;  -- todo: get from environment
    seq_id BIGINT;
    permuted_id BIGINT;
    max_int BIGINT := 2147483647;  -- 2^31 - 1
BEGIN
SELECT nextval(seq_name) INTO seq_id;

    -- Step 1: First transformation
    permuted_id := ((seq_id * 16807::bigint) % max_int);

    -- Step 2: XOR with account seed
    permuted_id := permuted_id # (account_id * 31337::bigint);

    -- Step 3: Multiplication (potential overflow point)
    -- We'll take the modulo first to keep the number smaller
    permuted_id := permuted_id % max_int;
    permuted_id := (permuted_id * 747796405::bigint) % max_int;

    -- Step 4: Bit rotation
    permuted_id := ((permuted_id >> 16) | (permuted_id << 16)) & max_int;

    -- Assign the generated ID to the appropriate field in NEW (already within INT range)
    NEW.id := permuted_id::int;
    RETURN NEW;  -- Return the modified row
EXCEPTION
   WHEN OTHERS THEN
       RAISE WARNING 'Failed to generate permuted key: %', SQLERRM;
       RAISE;
END;
$$ LANGUAGE plpgsql;

-- Trigger function for updated_at
-- todo: add mod user_id
CREATE OR REPLACE FUNCTION trakrf.update_updated_at_column()
RETURNS TRIGGER AS $$
BEGIN
   NEW.updated_at = CURRENT_TIMESTAMP;
RETURN NEW;
END;
$$ LANGUAGE plpgsql;
