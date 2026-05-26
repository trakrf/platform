-- TRA-810 — pull assets. Natural key: external_key (unique per org for live rows).
\set ON_ERROR_STOP on

INSERT INTO trakrf.assets
    (org_id, external_key, name, description, valid_from, valid_to,
     is_active, metadata, created_at, updated_at)
SELECT
    t_org.id, s.external_key, s.name, s.description, s.valid_from, s.valid_to,
    s.is_active, s.metadata, s.created_at, s.updated_at
FROM cloud_src.assets s
JOIN cloud_src.organizations src_org ON src_org.id = s.org_id
JOIN trakrf.organizations t_org ON t_org.identifier = src_org.identifier
WHERE s.deleted_at IS NULL AND src_org.deleted_at IS NULL;

DO $$ DECLARE src_n INT; tgt_n INT; BEGIN
    SELECT count(*) INTO src_n FROM cloud_src.assets s
        JOIN cloud_src.organizations o ON o.id = s.org_id AND o.deleted_at IS NULL
        WHERE s.deleted_at IS NULL;
    SELECT count(*) INTO tgt_n FROM trakrf.assets;
    IF src_n <> tgt_n THEN RAISE EXCEPTION 'assets mismatch: src=% tgt=%', src_n, tgt_n; END IF;
    RAISE NOTICE 'assets OK: % rows', tgt_n;
END $$;
