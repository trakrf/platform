-- TRA-921 — bound tag_scans growth for the live-MQTT ingestion firehose.
-- Enabling live ingestion (TRA-920) turned tag_scans into a continuous-write
-- raw MQTT audit log (~430-860 MB/day). It had retention (30d) but NO
-- compression, and on 2026-06-20 it filled the preview CNPG volume (8.6 GB /
-- ~8M rows) → Postgres refused WAL writes → app 503. This migration is the
-- durable fix: shorten retention 30d→7d and add Timescale compression.
--
-- Scope: tag_scans ONLY. asset_scans is deliberately left uncompressed — it
-- carries org-isolation RLS (000008), and Timescale does not enforce RLS on
-- COMPRESSED chunks (timescaledb#7830), so compressing it would open a
-- tenant-isolation hole. It is also low-volume business data (~800 MB / 365d).
--
-- No one-time drop_chunks/compress_chunk here: on a fresh prod / test / preview
-- rebuild the table starts empty so those would be no-ops, and the policies
-- registered below reclaim the current preview backlog on their next scheduled
-- run (~24h). Immediate reclaim, if wanted, is a one-off psql op (see PR).
--
-- All statements are transaction-safe (cf. add_retention_policy in 000008,
-- add_continuous_aggregate_policy in 000028), so they share one file / implicit
-- transaction. tag_scans has no continuous aggregate, so nothing here needs the
-- lone-statement/auto-commit treatment that 000027 did.
SET search_path = trakrf, public;

-- Retention: 30d → 7d. 7d of the firehose (even at the relaxed-dedupe ~2× rate)
-- stays far under the old 30d footprint and caps exposure to future rate steps.
-- tag_scans is a raw forensic/audit log; 7d is ample replay headroom.
SELECT remove_retention_policy('tag_scans', if_exists => true);
SELECT add_retention_policy('tag_scans', INTERVAL '7 days');

-- Compression: message_data is raw MQTT JSONB → ~10-20× on this workload.
-- segmentby = message_topic (per-reader MQTT topic; low cardinality, and the
-- axis idx_tag_scans_topic already groups by). orderby = created_at DESC, id DESC
-- covers the full PK (created_at, id) — the id tiebreaker keeps Timescale from
-- warning that a PK column is absent from the compression config.
ALTER TABLE tag_scans SET (
    timescaledb.compress,
    timescaledb.compress_segmentby = 'message_topic',
    timescaledb.compress_orderby   = 'created_at DESC, id DESC'
);

-- Compress chunks older than 2 days: keeps the most recent ~2d hot/uncompressed
-- for forensic reads and ingest locality, compresses the 2-7d tail. Paired with
-- 7d retention → ~2d hot + ~5d compressed → a few hundred MB steady-state.
SELECT add_compression_policy('tag_scans', INTERVAL '2 days');
