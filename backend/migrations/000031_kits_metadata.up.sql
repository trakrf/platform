-- TRA-1033: optional QA details on kits (Part #, Heat #, Operator, Date,
-- Vendor — the Howmet slide-deck fields). Free-form string map; Lot # stays
-- the required `label` column. Empty object for kits commissioned without.
ALTER TABLE trakrf.kits
    ADD COLUMN metadata jsonb NOT NULL DEFAULT '{}'::jsonb;
