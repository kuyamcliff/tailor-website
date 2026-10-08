-- Bespoke requests, quotes, orders, payments, appointments, production, support, portfolio.

CREATE TABLE quote_requests (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number                  text NOT NULL UNIQUE,
  customer_id             uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  status                  text NOT NULL DEFAULT 'new' CHECK (status IN ('new','reviewing','need_information','quote_sent','accepted','converted','closed')),
  garment_type_key        text NOT NULL,
  occasion                text NOT NULL,
  measurement_mode        text NOT NULL CHECK (measurement_mode IN ('entered','saved_profile','in_store')),
  measurement_version_id  uuid REFERENCES measurement_versions(id) ON DELETE SET NULL,
  fabric_mode             text NOT NULL CHECK (fabric_mode IN ('catalog','recommend','reference')),
  fabric_key              text,
  color_key               text,
  design_version_id       uuid REFERENCES design_versions(id) ON DELETE SET NULL,
  notes                   text NOT NULL DEFAULT '',
  desired_date            date,
  date_flexibility        text NOT NULL DEFAULT 'flexible' CHECK (date_flexibility IN ('fixed','flexible','very_flexible')),
  urgency                 text NOT NULL DEFAULT 'standard' CHECK (urgency IN ('standard','soon','urgent')),
  contact_name            text NOT NULL,
  contact_phone           text NOT NULL,
  contact_email           text,
  preferred_contact       text NOT NULL DEFAULT 'phone',
  info_requested          text,
  internal_notes          text NOT NULL DEFAULT '',
  access_token_hash       bytea NOT NULL,
  idempotency_key         text NOT NULL UNIQUE,
  version                 integer NOT NULL DEFAULT 1,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX quote_requests_status ON quote_requests (status, created_at DESC);
CREATE INDEX quote_requests_customer ON quote_requests (customer_id);
CREATE TRIGGER quote_requests_updated BEFORE UPDATE ON quote_requests FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE request_references (
  request_id  uuid NOT NULL REFERENCES quote_requests(id) ON DELETE CASCADE,
  upload_id   uuid NOT NULL REFERENCES uploads(id) ON DELETE CASCADE,
  tag         text NOT NULL DEFAULT 'overall' CHECK (tag IN ('overall','color','silhouette','collar','sleeve','pocket','fabric','embroidery','embellishment','back','front','other')),
  note        text NOT NULL DEFAULT '',
  sort_order  integer NOT NULL DEFAULT 0,
  PRIMARY KEY (request_id, upload_id)
);

CREATE TABLE quotes (
  id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number                text NOT NULL UNIQUE,
  request_id            uuid REFERENCES quote_requests(id) ON DELETE SET NULL,
  customer_id           uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  status                text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','sent','accepted','declined','changes_requested','expired','withdrawn')),
  current_revision_id   uuid,
  accepted_revision_id  uuid,
  decision_note         text,
  decided_at            timestamptz,
  access_token_hash     bytea NOT NULL,
  version               integer NOT NULL DEFAULT 1,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX quotes_status ON quotes (status, updated_at DESC);
CREATE INDEX quotes_customer ON quotes (customer_id);
CREATE TRIGGER quotes_updated BEFORE UPDATE ON quotes FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Immutable quote snapshots: every revision stores the fully calculated pricing.
CREATE TABLE quote_revisions (
  id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  quote_id               uuid NOT NULL REFERENCES quotes(id) ON DELETE CASCADE,
  revision_no            integer NOT NULL,
  currency               text NOT NULL,
  lines                  jsonb NOT NULL,
  subtotal_minor         bigint NOT NULL,
  discount_minor         bigint NOT NULL DEFAULT 0,
  delivery_minor         bigint NOT NULL DEFAULT 0,
  tax_minor              bigint NOT NULL DEFAULT 0,
  tax_rate_bp            integer NOT NULL DEFAULT 0,
  total_minor            bigint NOT NULL CHECK (total_minor >= 0),
  deposit_minor          bigint NOT NULL CHECK (deposit_minor >= 0),
  balance_minor          bigint NOT NULL CHECK (balance_minor >= 0),
  expires_at             timestamptz NOT NULL,
  customer_notes         text NOT NULL DEFAULT '',
  terms                  text NOT NULL DEFAULT '',
  estimated_ready_date   date,
  measurement_version_id uuid REFERENCES measurement_versions(id) ON DELETE SET NULL,
  design_version_id      uuid REFERENCES design_versions(id) ON DELETE SET NULL,
  created_by             uuid REFERENCES users(id) ON DELETE SET NULL,
  sent_at                timestamptz,
  created_at             timestamptz NOT NULL DEFAULT now(),
  UNIQUE (quote_id, revision_no),
  CHECK (deposit_minor <= total_minor),
  CHECK (deposit_minor + balance_minor = total_minor)
);
ALTER TABLE quotes ADD CONSTRAINT quotes_current_fk FOREIGN KEY (current_revision_id) REFERENCES quote_revisions(id) ON DELETE SET NULL;
ALTER TABLE quotes ADD CONSTRAINT quotes_accepted_fk FOREIGN KEY (accepted_revision_id) REFERENCES quote_revisions(id) ON DELETE SET NULL;

CREATE TABLE orders (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number                  text NOT NULL UNIQUE,
  customer_id             uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  kind                    text NOT NULL CHECK (kind IN ('ready_made','bespoke')),
  status                  text NOT NULL DEFAULT 'submitted',
  quote_id                uuid REFERENCES quotes(id) ON DELETE SET NULL,
  quote_revision_id       uuid REFERENCES quote_revisions(id) ON DELETE SET NULL,
  request_id              uuid REFERENCES quote_requests(id) ON DELETE SET NULL,
  measurement_version_id  uuid REFERENCES measurement_versions(id) ON DELETE SET NULL,
  currency                text NOT NULL,
  subtotal_minor          bigint NOT NULL,
  discount_minor          bigint NOT NULL DEFAULT 0,
  delivery_minor          bigint NOT NULL DEFAULT 0,
  tax_minor               bigint NOT NULL DEFAULT 0,
  total_minor             bigint NOT NULL CHECK (total_minor >= 0),
  deposit_required_minor  bigint NOT NULL DEFAULT 0 CHECK (deposit_required_minor >= 0),
  amount_paid_minor       bigint NOT NULL DEFAULT 0 CHECK (amount_paid_minor >= 0),
  amount_refunded_minor   bigint NOT NULL DEFAULT 0 CHECK (amount_refunded_minor >= 0),
  payment_status          text NOT NULL DEFAULT 'unpaid' CHECK (payment_status IN ('unpaid','deposit_paid','paid','partially_refunded','refunded')),
  fulfillment_method      text NOT NULL DEFAULT 'pickup' CHECK (fulfillment_method IN ('pickup','local_delivery','courier')),
  delivery_address        jsonb,
  delivery_note           text NOT NULL DEFAULT '',
  delivery_status         text NOT NULL DEFAULT 'not_started' CHECK (delivery_status IN ('not_started','preparing','ready_for_pickup','dispatched','delivered','picked_up')),
  contact                 jsonb NOT NULL,
  customer_notes          text NOT NULL DEFAULT '',
  urgency                 text NOT NULL DEFAULT 'standard',
  due_date                date,
  access_token_hash       bytea NOT NULL,
  idempotency_key         text NOT NULL UNIQUE,
  version                 integer NOT NULL DEFAULT 1,
  created_at              timestamptz NOT NULL DEFAULT now(),
  updated_at              timestamptz NOT NULL DEFAULT now(),
  CHECK (amount_refunded_minor <= amount_paid_minor)
);
CREATE INDEX orders_status ON orders (status, updated_at DESC);
CREATE INDEX orders_customer ON orders (customer_id, created_at DESC);
CREATE UNIQUE INDEX orders_one_per_quote ON orders (quote_id) WHERE quote_id IS NOT NULL;
CREATE TRIGGER orders_updated BEFORE UPDATE ON orders FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE order_items (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id           uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  kind               text NOT NULL CHECK (kind IN ('product','bespoke','fee')),
  product_id         uuid REFERENCES products(id) ON DELETE SET NULL,
  variant_id         uuid REFERENCES product_variants(id) ON DELETE SET NULL,
  design_version_id  uuid REFERENCES design_versions(id) ON DELETE SET NULL,
  name               text NOT NULL,
  description        text NOT NULL DEFAULT '',
  snapshot           jsonb NOT NULL DEFAULT '{}',
  quantity           integer NOT NULL CHECK (quantity > 0),
  unit_price_minor   bigint NOT NULL CHECK (unit_price_minor >= 0),
  total_minor        bigint NOT NULL CHECK (total_minor >= 0)
);
CREATE INDEX order_items_order ON order_items (order_id);

CREATE TABLE order_notes (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id    uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  author_id   uuid REFERENCES users(id) ON DELETE SET NULL,
  visibility  text NOT NULL CHECK (visibility IN ('internal','customer')),
  body        text NOT NULL CHECK (length(body) BETWEEN 1 AND 4000),
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX order_notes_order ON order_notes (order_id, created_at);

CREATE TABLE production_tasks (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id      uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  stage         text NOT NULL,
  title         text NOT NULL,
  assigned_to   uuid REFERENCES users(id) ON DELETE SET NULL,
  status        text NOT NULL DEFAULT 'open' CHECK (status IN ('open','done')),
  due_at        timestamptz,
  completed_at  timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX production_tasks_order ON production_tasks (order_id);
CREATE TRIGGER production_tasks_updated BEFORE UPDATE ON production_tasks FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payments (
  id                       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id                 uuid NOT NULL REFERENCES orders(id) ON DELETE RESTRICT,
  purpose                  text NOT NULL CHECK (purpose IN ('deposit','balance','full')),
  provider                 text NOT NULL CHECK (provider IN ('mtn','orange','dev')),
  amount_minor             bigint NOT NULL CHECK (amount_minor > 0),
  currency                 text NOT NULL,
  status                   text NOT NULL DEFAULT 'created' CHECK (status IN ('created','pending','customer_action_required','processing','succeeded','failed','expired','cancelled','refunded','partially_refunded')),
  payer_msisdn             text NOT NULL,
  provider_reference       text NOT NULL UNIQUE,
  provider_transaction_id  text,
  provider_payment_url     text,
  idempotency_key          text NOT NULL UNIQUE,
  failure_code             text,
  failure_message          text,
  expires_at               timestamptz NOT NULL,
  succeeded_at             timestamptz,
  last_checked_at          timestamptz,
  refunded_minor           bigint NOT NULL DEFAULT 0,
  version                  integer NOT NULL DEFAULT 1,
  created_at               timestamptz NOT NULL DEFAULT now(),
  updated_at               timestamptz NOT NULL DEFAULT now()
);
-- At most one in-flight payment per order: prevents double charging from double clicks or two tabs.
CREATE UNIQUE INDEX payments_one_active_per_order ON payments (order_id)
  WHERE status IN ('created','pending','customer_action_required','processing');
CREATE INDEX payments_status ON payments (status, updated_at);
CREATE INDEX payments_order ON payments (order_id);
CREATE TRIGGER payments_updated BEFORE UPDATE ON payments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE payment_attempts (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id       uuid NOT NULL REFERENCES payments(id) ON DELETE CASCADE,
  operation        text NOT NULL,
  http_status      integer,
  outcome          text NOT NULL,
  detail           jsonb NOT NULL DEFAULT '{}',
  duration_ms      integer,
  created_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX payment_attempts_payment ON payment_attempts (payment_id, created_at);

CREATE TABLE payment_provider_events (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  provider      text NOT NULL,
  event_key     text NOT NULL,
  payment_id    uuid REFERENCES payments(id) ON DELETE SET NULL,
  payload       jsonb NOT NULL,
  verified      boolean NOT NULL DEFAULT false,
  outcome       text,
  received_at   timestamptz NOT NULL DEFAULT now(),
  processed_at  timestamptz,
  UNIQUE (provider, event_key)
);

CREATE TABLE refunds (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  payment_id          uuid NOT NULL REFERENCES payments(id) ON DELETE RESTRICT,
  amount_minor        bigint NOT NULL CHECK (amount_minor > 0),
  reason              text NOT NULL,
  method              text NOT NULL DEFAULT 'manual' CHECK (method IN ('manual','provider')),
  status              text NOT NULL DEFAULT 'recorded' CHECK (status IN ('recorded','pending','succeeded','failed')),
  provider_reference  text,
  created_by          uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER refunds_updated BEFORE UPDATE ON refunds FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE availability_rules (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  weekday       smallint NOT NULL CHECK (weekday BETWEEN 0 AND 6),
  start_minute  integer NOT NULL CHECK (start_minute BETWEEN 0 AND 1439),
  end_minute    integer NOT NULL CHECK (end_minute BETWEEN 1 AND 1440),
  staff_id      uuid REFERENCES users(id) ON DELETE CASCADE,
  created_at    timestamptz NOT NULL DEFAULT now(),
  CHECK (end_minute > start_minute)
);

CREATE TABLE blocked_periods (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  starts_at   timestamptz NOT NULL,
  ends_at     timestamptz NOT NULL,
  kind        text NOT NULL DEFAULT 'blocked' CHECK (kind IN ('blocked','holiday')),
  reason      text NOT NULL DEFAULT '',
  staff_id    uuid REFERENCES users(id) ON DELETE CASCADE,
  created_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at)
);

CREATE TABLE appointments (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number             text NOT NULL UNIQUE,
  customer_id        uuid NOT NULL REFERENCES customers(id) ON DELETE RESTRICT,
  type               text NOT NULL CHECK (type IN ('consultation','measuring','fitting','final_fitting','pickup','alteration','video_consultation','other')),
  status             text NOT NULL DEFAULT 'booked' CHECK (status IN ('booked','cancelled','completed','no_show')),
  starts_at          timestamptz NOT NULL,
  ends_at            timestamptz NOT NULL,
  buffer_until       timestamptz NOT NULL,
  -- '00000000-0000-0000-0000-000000000000' represents the studio itself when no staff member is assigned.
  resource_id        uuid NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
  staff_id           uuid REFERENCES users(id) ON DELETE SET NULL,
  location           text NOT NULL DEFAULT 'studio',
  order_id           uuid REFERENCES orders(id) ON DELETE SET NULL,
  request_id         uuid REFERENCES quote_requests(id) ON DELETE SET NULL,
  customer_notes     text NOT NULL DEFAULT '',
  internal_notes     text NOT NULL DEFAULT '',
  contact            jsonb NOT NULL,
  access_token_hash  bytea NOT NULL,
  idempotency_key    text NOT NULL UNIQUE,
  reminder_sent_at   timestamptz,
  version            integer NOT NULL DEFAULT 1,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  CHECK (ends_at > starts_at),
  CHECK (buffer_until >= ends_at),
  -- Simultaneous bookings for the same resource are rejected by the database itself.
  EXCLUDE USING gist (resource_id WITH =, tstzrange(starts_at, buffer_until) WITH &&) WHERE (status = 'booked')
);
CREATE INDEX appointments_starts ON appointments (starts_at);
CREATE INDEX appointments_customer ON appointments (customer_id);
CREATE TRIGGER appointments_updated BEFORE UPDATE ON appointments FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE fitting_sessions (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id        uuid NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
  appointment_id  uuid REFERENCES appointments(id) ON DELETE SET NULL,
  notes           text NOT NULL DEFAULT '',
  customer_notes  text NOT NULL DEFAULT '',
  adjustments     jsonb NOT NULL DEFAULT '[]',
  created_by      uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX fitting_sessions_order ON fitting_sessions (order_id);

CREATE TABLE support_threads (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  number              text NOT NULL UNIQUE,
  customer_id         uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  subject             text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
  category            text NOT NULL CHECK (category IN ('general','order_issue','measurement_help','payment_issue','appointment_issue','alteration_request','other')),
  status              text NOT NULL DEFAULT 'open' CHECK (status IN ('open','pending','resolved')),
  priority            text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high','urgent')),
  assigned_to         uuid REFERENCES users(id) ON DELETE SET NULL,
  order_id            uuid REFERENCES orders(id) ON DELETE SET NULL,
  request_id          uuid REFERENCES quote_requests(id) ON DELETE SET NULL,
  unread_for_staff    integer NOT NULL DEFAULT 0,
  unread_for_customer integer NOT NULL DEFAULT 0,
  first_response_at   timestamptz,
  last_message_at     timestamptz NOT NULL DEFAULT now(),
  access_token_hash   bytea NOT NULL,
  idempotency_key     text UNIQUE,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX support_threads_status ON support_threads (status, last_message_at DESC);
CREATE INDEX support_threads_customer ON support_threads (customer_id);
CREATE TRIGGER support_threads_updated BEFORE UPDATE ON support_threads FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE support_messages (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  thread_id       uuid NOT NULL REFERENCES support_threads(id) ON DELETE CASCADE,
  author_type     text NOT NULL CHECK (author_type IN ('customer','staff','system')),
  author_id       uuid REFERENCES users(id) ON DELETE SET NULL,
  body            text NOT NULL CHECK (length(body) BETWEEN 1 AND 5000),
  attachment_ids  uuid[] NOT NULL DEFAULT '{}',
  internal        boolean NOT NULL DEFAULT false,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX support_messages_thread ON support_messages (thread_id, created_at);

CREATE TABLE portfolio_projects (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug                 text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]+$'),
  title                text NOT NULL,
  category             text NOT NULL CHECK (category IN ('suits','dresses','gowns','shirts','trousers','traditional','wedding','alterations','other')),
  description          text NOT NULL DEFAULT '',
  materials            text NOT NULL DEFAULT '',
  tags                 text[] NOT NULL DEFAULT '{}',
  customer_permission  text NOT NULL DEFAULT 'not_required' CHECK (customer_permission IN ('not_required','granted','pending','denied')),
  video_url            text,
  featured             boolean NOT NULL DEFAULT false,
  status               text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
  sort_order           integer NOT NULL DEFAULT 0,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  -- A project featuring customer images can only be published with permission.
  CHECK (status <> 'published' OR customer_permission IN ('not_required','granted'))
);
CREATE TRIGGER portfolio_projects_updated BEFORE UPDATE ON portfolio_projects FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE portfolio_media (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  project_id  uuid NOT NULL REFERENCES portfolio_projects(id) ON DELETE CASCADE,
  url         text NOT NULL,
  upload_id   uuid REFERENCES uploads(id) ON DELETE SET NULL,
  alt         text NOT NULL,
  width       integer,
  height      integer,
  sort_order  integer NOT NULL DEFAULT 0
);
CREATE INDEX portfolio_media_project ON portfolio_media (project_id, sort_order);

CREATE TABLE testimonials (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_name  text NOT NULL,
  quote          text NOT NULL CHECK (length(quote) BETWEEN 1 AND 1200),
  context        text NOT NULL DEFAULT '',
  source         text NOT NULL DEFAULT 'direct',
  consent        boolean NOT NULL DEFAULT false,
  status         text NOT NULL DEFAULT 'draft' CHECK (status IN ('draft','published')),
  published_at   timestamptz,
  created_by     uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  CHECK (status <> 'published' OR consent)
);
CREATE TRIGGER testimonials_updated BEFORE UPDATE ON testimonials FOR EACH ROW EXECUTE FUNCTION set_updated_at();
