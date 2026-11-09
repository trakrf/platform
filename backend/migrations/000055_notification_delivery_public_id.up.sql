-- A public id for notification deliveries, and an index for listing sent ones.
--
-- notification_deliveries.id is an identity sequence shared by every org, so
-- exposing it lets one org estimate how many notifications the others send.
-- public_id is drawn from the same keyed Feistel generator as every other
-- surrogate id (000002): non-sequential and globally unique. It is a column
-- default rather than the generate_obfuscated_id trigger because that trigger
-- writes NEW.id, which here stays the internal identity.
--
-- The partial index serves the alert history list: one org's delivered rows,
-- newest sent first.

SET search_path = trakrf, public;

ALTER TABLE notification_deliveries ADD COLUMN public_id BIGINT;

UPDATE notification_deliveries
    SET public_id = trakrf._feistel_encrypt(nextval('trakrf.id_seq'));

ALTER TABLE notification_deliveries
    ALTER COLUMN public_id SET DEFAULT trakrf._feistel_encrypt(nextval('trakrf.id_seq')),
    ALTER COLUMN public_id SET NOT NULL,
    ADD CONSTRAINT notification_deliveries_public_id_key UNIQUE (public_id);

CREATE INDEX idx_notification_deliveries_org_sent
    ON notification_deliveries (org_id, finalized_at DESC, id DESC)
    WHERE state = 'delivered';

COMMENT ON COLUMN notification_deliveries.public_id IS
    'Non-sequential id exposed by the API; id is the internal identity and is never exposed';
