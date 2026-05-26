-- TRA-720 — extensions + schema foundation for the clean migration stack.
--
-- pgcrypto is explicit (Cloud had it implicit via TimescaleDB Cloud defaults;
-- CNPG does not). Required by trakrf.generate_obfuscated_id() in 000002 for
-- pgcrypto.hmac().
--
-- ltree is intentionally NOT installed. It was used by the dropped
-- locations.path column (000018, dropped in 000042). Nothing else depends on
-- it; reinstall when a future feature requires it.

CREATE EXTENSION IF NOT EXISTS timescaledb;
CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE SCHEMA IF NOT EXISTS trakrf;
