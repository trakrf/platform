-- Reverse of 000038. org_capabilities.capability references capabilities(name)
-- with no ON DELETE action, so this fails loudly if any org still holds the
-- grant. That is deliberate: silently revoking a live grant on a down-migration
-- would be worse than a noisy failure. Revoke the grants first, then migrate
-- down.
SET search_path = trakrf, public;

DELETE FROM capabilities WHERE name = 'kitting';
