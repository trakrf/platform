-- TRA-1135: force an operator-provisioned account to rotate its bootstrap
-- password before it can use the app.
--
-- Onsite onboarding is synchronous and every account-creation flow we own is
-- asynchronous: an org invite expires in 7 days and a password reset in 72
-- hours, but both require the user to receive mail, which is no use inside a
-- two-hour onsite session. So an operator sets a password in the room. That is
-- a supported flow only if the account cannot stay on that password — which is
-- what this column buys.
--
-- DEFAULT FALSE is load-bearing: every account that predates this migration is
-- one an operator has no reason to force, and a default of TRUE would lock out
-- all 49 production users at once.
--
-- The flag is cleared by storage.UpdateUserPassword, which is the single funnel
-- both the authenticated change-password path (TRA-1130) and the token reset
-- path already write through — so neither caller has to know the flag exists.
ALTER TABLE trakrf.users
    ADD COLUMN must_change_password BOOLEAN NOT NULL DEFAULT FALSE;

COMMENT ON COLUMN trakrf.users.must_change_password IS
    'TRA-1135: when true the app is gated behind the change-password screen until the user sets their own password. Cleared by any password write.';
