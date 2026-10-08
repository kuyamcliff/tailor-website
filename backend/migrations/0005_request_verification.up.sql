-- Tailor-verified measurements recorded while reviewing a request, kept separate from what the customer submitted.
ALTER TABLE quote_requests ADD COLUMN verified_measurement_version_id uuid REFERENCES measurement_versions(id) ON DELETE SET NULL;
ALTER TABLE quote_requests ADD COLUMN body_model text NOT NULL DEFAULT 'masculine';
ALTER TABLE quote_requests ADD COLUMN fit_preference text NOT NULL DEFAULT 'regular';
ALTER TABLE quote_requests ADD COLUMN occasion_note text NOT NULL DEFAULT '';
ALTER TABLE quote_requests ADD COLUMN support_thread_id uuid REFERENCES support_threads(id) ON DELETE SET NULL;
