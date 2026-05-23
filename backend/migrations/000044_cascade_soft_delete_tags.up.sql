SET search_path = trakrf,public;

-- TRA-816 — one-shot sweep for tag rows orphaned by a previously soft-deleted
-- parent asset or location. Before this fix, DeleteAsset / DeleteLocation
-- soft-deleted the parent row but left attached `trakrf.tags` rows with
-- deleted_at IS NULL. The partial unique constraint
-- (org_id, type, value) WHERE deleted_at IS NULL stayed satisfied by the
-- orphan, blocking re-use of the value on a different entity in the same org
-- with no UI path to clear it.
--
-- The durable fix lives in the application layer (Storage.DeleteAsset /
-- Storage.DeleteLocation cascade into the tag rows in the same transaction).
-- This migration only cleans up existing orphans so the value space is
-- usable again. It is idempotent: re-running matches zero rows once the
-- orphans are gone.
--
-- Preview footprint at write time (2026-05-23): 174 asset-tag orphans + 16
-- location-tag orphans across 8 orgs.

UPDATE trakrf.tags t
   SET deleted_at = COALESCE(a.deleted_at, l.deleted_at, NOW())
  FROM trakrf.tags t2
  LEFT JOIN trakrf.assets    a ON a.id = t2.asset_id
  LEFT JOIN trakrf.locations l ON l.id = t2.location_id
 WHERE t.id = t2.id
   AND t.deleted_at IS NULL
   AND (
        (t2.asset_id    IS NOT NULL AND a.deleted_at IS NOT NULL)
     OR (t2.location_id IS NOT NULL AND l.deleted_at IS NOT NULL)
   );
