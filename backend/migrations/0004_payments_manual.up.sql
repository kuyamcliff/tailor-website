-- Offline payments recorded by staff (cash, bank transfer) and explicit marking of simulated payments.
ALTER TABLE payments DROP CONSTRAINT payments_provider_check;
ALTER TABLE payments ADD CONSTRAINT payments_provider_check CHECK (provider IN ('mtn','orange','manual'));
ALTER TABLE payments ADD COLUMN simulated boolean NOT NULL DEFAULT false;
ALTER TABLE payments ADD COLUMN method_note text NOT NULL DEFAULT '';
ALTER TABLE payments ADD COLUMN recorded_by uuid REFERENCES users(id) ON DELETE SET NULL;
ALTER TABLE payments ALTER COLUMN payer_msisdn DROP NOT NULL;
CREATE INDEX payments_reconcile ON payments (last_checked_at NULLS FIRST)
  WHERE status IN ('created','pending','customer_action_required','processing');
