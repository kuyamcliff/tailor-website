DROP INDEX IF EXISTS payments_reconcile;
ALTER TABLE payments DROP COLUMN IF EXISTS recorded_by;
ALTER TABLE payments DROP COLUMN IF EXISTS method_note;
ALTER TABLE payments DROP COLUMN IF EXISTS simulated;
DELETE FROM payments WHERE provider = 'manual';
ALTER TABLE payments DROP CONSTRAINT payments_provider_check;
ALTER TABLE payments ADD CONSTRAINT payments_provider_check CHECK (provider IN ('mtn','orange','dev'));
UPDATE payments SET payer_msisdn = '' WHERE payer_msisdn IS NULL;
ALTER TABLE payments ALTER COLUMN payer_msisdn SET NOT NULL;
