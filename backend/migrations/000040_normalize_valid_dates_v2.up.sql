SET search_path=trakrf,public;

-- TRA-624 / BB20 §F1: re-sweep valid_from/valid_to sentinel values.
--
-- Migration 000030 (TRA-468) established the wire convention (valid_to NULL
-- means "no expiry"; never a sentinel like 0001-01-01 or 2099-12-31), but the
-- frontend AssetForm continued substituting '2099-12-31' for an empty
-- valid_to, re-introducing the sentinel on subsequent writes. The frontend
-- fix lands in this PR; this migration cleans the live preview rows.
--
-- Identical range-based predicate to 000030, applied to the post-rename
-- table set (identifiers → tags via 000033).

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

-- tags (formerly identifiers, renamed in 000033)
UPDATE tags SET valid_from = created_at
 WHERE valid_from < TIMESTAMPTZ '1900-01-01';
UPDATE tags SET valid_to = NULL
 WHERE valid_to IS NOT NULL
   AND (valid_to < TIMESTAMPTZ '1900-01-01' OR valid_to > TIMESTAMPTZ '2099-01-01');
