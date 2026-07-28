-- TRA-1065: mint the `kitting` capability. Kits (TRA-1032/1033) ships gated
-- because it is still shaped around one customer's workflow and unfinished —
-- we don't want it reading as a production feature yet.
--
-- Vocabulary only. NO grants: ADR 0002's default is zero grants for every org,
-- and a backfill here would hardcode an org id that differs between preview and
-- prod. `kitting` is granted to the Howmet Demo org on preview by hand, via the
-- superadmin capabilities UI (TRA-1027). Prod gets none.
--
-- Mirrored by the Go registry in internal/capability; a test pins the two in
-- sync, so a name added here and nowhere else fails CI rather than drifting.
--
-- No GRANTs here: the infra init-grants Job owns privileges.
SET search_path = trakrf, public;

INSERT INTO capabilities (name) VALUES ('kitting');
