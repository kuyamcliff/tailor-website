-- Foundation: identity, roles, sessions, audit, settings, flags, content, uploads.

CREATE EXTENSION IF NOT EXISTS btree_gist;

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TABLE roles (
  key          text PRIMARY KEY CHECK (key ~ '^[a-z_]+$'),
  name         text NOT NULL,
  permissions  text[] NOT NULL DEFAULT '{}',
  is_staff     boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER roles_updated BEFORE UPDATE ON roles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

INSERT INTO roles (key, name, is_staff, permissions) VALUES
  ('customer', 'Customer', false, '{}'),
  ('owner', 'Owner', true, ARRAY[
    'orders.read','orders.update','customers.read','customers.update','measurements.write',
    'payments.read','payments.refund','payments.settings','quotes.write','requests.read','requests.update',
    'products.write','garments.write','fabrics.write','fit_rules.write','assets.write',
    'appointments.read','appointments.write','support.read','support.write','content.write',
    'portfolio.write','testimonials.write','analytics.read','settings.write','staff.write','audit.read']),
  ('manager', 'Manager', true, ARRAY[
    'orders.read','orders.update','customers.read','customers.update','measurements.write',
    'payments.read','quotes.write','requests.read','requests.update','products.write','garments.write',
    'fabrics.write','assets.write','appointments.read','appointments.write','support.read','support.write',
    'content.write','portfolio.write','testimonials.write','analytics.read','audit.read']),
  ('tailor', 'Tailor', true, ARRAY[
    'orders.read','orders.update','customers.read','measurements.write','requests.read',
    'appointments.read','appointments.write','support.read']),
  ('support', 'Support', true, ARRAY[
    'orders.read','customers.read','requests.read','appointments.read','appointments.write',
    'support.read','support.write']),
  ('content_manager', 'Content manager', true, ARRAY[
    'products.write','fabrics.write','content.write','portfolio.write','testimonials.write']);

CREATE TABLE users (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  email              text,
  phone              text,
  password_hash      text NOT NULL,
  full_name          text NOT NULL CHECK (length(full_name) BETWEEN 1 AND 160),
  role               text NOT NULL REFERENCES roles(key) DEFAULT 'customer',
  status             text NOT NULL DEFAULT 'active' CHECK (status IN ('active','locked','disabled','deleted')),
  failed_login_count integer NOT NULL DEFAULT 0,
  locked_until       timestamptz,
  email_verified_at  timestamptz,
  phone_verified_at  timestamptz,
  last_login_at      timestamptz,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz,
  CHECK (email IS NOT NULL OR phone IS NOT NULL)
);
CREATE UNIQUE INDEX users_email_unique ON users (lower(email)) WHERE email IS NOT NULL AND deleted_at IS NULL;
CREATE UNIQUE INDEX users_phone_unique ON users (phone) WHERE phone IS NOT NULL AND deleted_at IS NULL;
CREATE TRIGGER users_updated BEFORE UPDATE ON users FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE sessions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash   bytea NOT NULL UNIQUE,
  csrf_token   text NOT NULL,
  ip           text,
  user_agent   text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  last_seen_at timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  revoked_at   timestamptz
);
CREATE INDEX sessions_user ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE password_resets (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  token_hash  bytea NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  created_at  timestamptz NOT NULL DEFAULT now()
);

-- Append-only audit log.
CREATE TABLE audit_logs (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
  actor_role   text,
  action       text NOT NULL,
  object_type  text NOT NULL,
  object_id    text,
  before       jsonb,
  after        jsonb,
  request_id   text,
  ip           text,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX audit_logs_object ON audit_logs (object_type, object_id, created_at DESC);
CREATE INDEX audit_logs_created ON audit_logs (created_at DESC);

CREATE OR REPLACE FUNCTION audit_logs_immutable() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'audit_logs is append-only';
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER audit_logs_no_update BEFORE UPDATE OR DELETE ON audit_logs
  FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();

-- Generic status history for business objects (requests, quotes, orders, payments, appointments).
CREATE TABLE status_history (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  object_type  text NOT NULL,
  object_id    uuid NOT NULL,
  old_status   text,
  new_status   text NOT NULL,
  actor_id     uuid REFERENCES users(id) ON DELETE SET NULL,
  actor_label  text NOT NULL DEFAULT 'system',
  note         text,
  customer_visible boolean NOT NULL DEFAULT true,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX status_history_object ON status_history (object_type, object_id, created_at);

CREATE TABLE business_settings (
  key         text PRIMARY KEY,
  value       jsonb NOT NULL,
  updated_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE feature_flags (
  key          text PRIMARY KEY,
  enabled      boolean NOT NULL DEFAULT false,
  description  text NOT NULL DEFAULT '',
  updated_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at   timestamptz NOT NULL DEFAULT now()
);

INSERT INTO feature_flags (key, enabled, description) VALUES
  ('studio', true, '3D fitting studio'),
  ('photo_body_estimation', false, 'Photo-based body estimation (requires provider)'),
  ('reference_analysis', false, 'Reference image analysis (requires provider)'),
  ('online_payments', false, 'Accept Mobile Money payments online'),
  ('payments_mtn', false, 'MTN Mobile Money'),
  ('payments_orange', false, 'Orange Money'),
  ('appointments', true, 'Online appointment booking'),
  ('customer_accounts', true, 'Customer sign up and sign in'),
  ('support_inbox', true, 'Customer support inbox'),
  ('guest_checkout', true, 'Guest checkout for ready-made items');

CREATE TABLE content_blocks (
  key         text PRIMARY KEY,
  value       jsonb NOT NULL,
  published   boolean NOT NULL DEFAULT true,
  version     integer NOT NULL DEFAULT 1,
  updated_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE customers (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id            uuid UNIQUE REFERENCES users(id) ON DELETE SET NULL,
  full_name          text NOT NULL CHECK (length(full_name) BETWEEN 1 AND 160),
  phone              text,
  email              text,
  preferred_contact  text NOT NULL DEFAULT 'phone' CHECK (preferred_contact IN ('phone','whatsapp','email','sms')),
  internal_notes     text NOT NULL DEFAULT '',
  notification_prefs jsonb NOT NULL DEFAULT '{"email": true, "sms": true, "in_app": true}',
  marketing_consent  boolean NOT NULL DEFAULT false,
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  deleted_at         timestamptz
);
CREATE INDEX customers_phone ON customers (phone);
CREATE INDEX customers_email ON customers (lower(email));
CREATE INDEX customers_name_search ON customers USING gin (to_tsvector('simple', full_name));
CREATE TRIGGER customers_updated BEFORE UPDATE ON customers FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE addresses (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id  uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  label        text NOT NULL DEFAULT 'Home',
  recipient    text NOT NULL,
  phone        text NOT NULL,
  line1        text NOT NULL,
  line2        text NOT NULL DEFAULT '',
  city         text NOT NULL,
  region       text NOT NULL DEFAULT '',
  country      text NOT NULL DEFAULT 'CM',
  notes        text NOT NULL DEFAULT '',
  is_default   boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX addresses_one_default ON addresses (customer_id) WHERE is_default;
CREATE TRIGGER addresses_updated BEFORE UPDATE ON addresses FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Uploaded objects. Only metadata lives here; bytes live in object storage.
CREATE TABLE uploads (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  purpose           text NOT NULL CHECK (purpose IN ('reference','portfolio','product','fabric','support','asset','content','body_photo')),
  visibility        text NOT NULL DEFAULT 'private' CHECK (visibility IN ('private','public')),
  customer_id       uuid REFERENCES customers(id) ON DELETE SET NULL,
  uploaded_by       uuid REFERENCES users(id) ON DELETE SET NULL,
  guest_token_hash  bytea,
  storage_key       text NOT NULL UNIQUE,
  original_name     text NOT NULL DEFAULT '',
  mime              text NOT NULL,
  bytes             bigint NOT NULL CHECK (bytes > 0),
  width             integer,
  height            integer,
  sha256            text NOT NULL,
  derivatives       jsonb NOT NULL DEFAULT '{}',
  license           jsonb,
  status            text NOT NULL DEFAULT 'complete' CHECK (status IN ('complete','deleted')),
  retention_until   timestamptz,
  created_at        timestamptz NOT NULL DEFAULT now(),
  deleted_at        timestamptz
);
CREATE INDEX uploads_customer ON uploads (customer_id) WHERE deleted_at IS NULL;
CREATE INDEX uploads_guest ON uploads (guest_token_hash) WHERE deleted_at IS NULL;
CREATE INDEX uploads_retention ON uploads (retention_until) WHERE deleted_at IS NULL;

CREATE TABLE notifications (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id   uuid REFERENCES customers(id) ON DELETE CASCADE,
  user_id       uuid REFERENCES users(id) ON DELETE CASCADE,
  audience      text NOT NULL DEFAULT 'customer' CHECK (audience IN ('customer','staff')),
  channel       text NOT NULL CHECK (channel IN ('in_app','email','sms')),
  event         text NOT NULL,
  dedupe_key    text NOT NULL,
  title         text NOT NULL,
  body          text NOT NULL,
  link          text,
  status        text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','sent','failed','skipped')),
  attempts      integer NOT NULL DEFAULT 0,
  last_error    text,
  sent_at       timestamptz,
  read_at       timestamptz,
  created_at    timestamptz NOT NULL DEFAULT now(),
  UNIQUE (dedupe_key, channel)
);
CREATE INDEX notifications_customer ON notifications (customer_id, created_at DESC);
CREATE INDEX notifications_pending ON notifications (status, created_at) WHERE status = 'pending';

CREATE TABLE wishlist_items (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id  uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  item_type    text NOT NULL CHECK (item_type IN ('product','fabric','design')),
  item_id      uuid NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (customer_id, item_type, item_id)
);

CREATE SEQUENCE request_number_seq;
CREATE SEQUENCE quote_number_seq;
CREATE SEQUENCE order_number_seq;
CREATE SEQUENCE support_number_seq;
CREATE SEQUENCE appointment_number_seq;
