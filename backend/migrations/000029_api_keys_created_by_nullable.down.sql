SET search_path=trakrf,public;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM api_keys WHERE created_by IS NULL) THEN
    RAISE EXCEPTION 'cannot downgrade: % api_keys rows have NULL created_by',
      (SELECT COUNT(*) FROM api_keys WHERE created_by IS NULL);
  END IF;
END$$;

ALTER TABLE api_keys DROP CONSTRAINT api_keys_creator_exactly_one;
ALTER TABLE api_keys DROP COLUMN created_by_key_id;
ALTER TABLE api_keys ALTER COLUMN created_by SET NOT NULL;
