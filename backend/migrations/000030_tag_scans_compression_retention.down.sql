-- Revert TRA-921. Restores the 000008 posture on tag_scans: 30d retention, no
-- compression. Compression must be disabled with all chunks decompressed; in the
-- migrate-down test path the table is empty so show_chunks() returns nothing and
-- decompress_chunk is never invoked. In a populated DB this decompresses first.
--
-- drop_chunks under the old 30d policy cannot restore rows already reaped by the
-- 7d policy — retention is destructive by nature. This down file restores the
-- policy, not the data.
SET search_path = trakrf, public;

SELECT remove_compression_policy('tag_scans', if_exists => true);

SELECT decompress_chunk(c, if_compressed => true)
FROM show_chunks('tag_scans') c;

ALTER TABLE tag_scans SET (timescaledb.compress = false);

SELECT remove_retention_policy('tag_scans', if_exists => true);
SELECT add_retention_policy('tag_scans', INTERVAL '30 days');
