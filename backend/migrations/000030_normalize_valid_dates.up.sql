SET search_path=trakrf,public;

-- TRA-468: normalize legacy valid_from/valid_to values to the single wire convention.
-- valid_from: NOT NULL, must represent a real effective moment (>= 1900, < 2100).
-- valid_to:   NULL = "no expiry"; RFC3339 timestamp otherwise; never a sentinel.
--
-- Range-based thresholds sweep the two observed sentinels (0001-01-01, 2099-12-31)
-- plus any future unknown sentinels (e.g., 1970-01-01 epoch, 9999-12-31). No legitimate
-- business data lives outside [1900-01-01, 2099-01-01).

-- organizations
UPDATE organizations SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE organizations SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- assets
UPDATE assets SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE assets SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- locations
UPDATE locations SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE locations SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');

-- identifiers
UPDATE identifiers SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE identifiers SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');
