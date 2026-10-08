ALTER TABLE quote_requests DROP COLUMN IF EXISTS support_thread_id;
ALTER TABLE quote_requests DROP COLUMN IF EXISTS occasion_note;
ALTER TABLE quote_requests DROP COLUMN IF EXISTS fit_preference;
ALTER TABLE quote_requests DROP COLUMN IF EXISTS body_model;
ALTER TABLE quote_requests DROP COLUMN IF EXISTS verified_measurement_version_id;
