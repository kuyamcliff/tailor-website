-- Garments, configuration options, fabrics, measurements, fit rules, 3D assets, designs, products.

CREATE TABLE garment_types (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  key               text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9-]+$'),
  name              text NOT NULL,
  category          text NOT NULL DEFAULT 'other',
  description       text NOT NULL DEFAULT '',
  base_price_minor  bigint NOT NULL DEFAULT 0 CHECK (base_price_minor >= 0),
  studio_enabled    boolean NOT NULL DEFAULT false,
  body_model_hint   text NOT NULL DEFAULT 'any' CHECK (body_model_hint IN ('any','masculine','feminine')),
  quote_only        boolean NOT NULL DEFAULT true,
  sort_order        integer NOT NULL DEFAULT 0,
  active            boolean NOT NULL DEFAULT true,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER garment_types_updated BEFORE UPDATE ON garment_types FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE option_groups (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  garment_type_id  uuid NOT NULL REFERENCES garment_types(id) ON DELETE CASCADE,
  key              text NOT NULL CHECK (key ~ '^[a-z0-9_]+$'),
  name             text NOT NULL,
  section          text NOT NULL DEFAULT 'general',
  selection        text NOT NULL DEFAULT 'single' CHECK (selection IN ('single','number')),
  required         boolean NOT NULL DEFAULT true,
  min_value        numeric,
  max_value        numeric,
  step_value       numeric,
  default_number   numeric,
  unit             text,
  sort_order       integer NOT NULL DEFAULT 0,
  active           boolean NOT NULL DEFAULT true,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (garment_type_id, key)
);
CREATE TRIGGER option_groups_updated BEFORE UPDATE ON option_groups FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE option_values (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  group_id     uuid NOT NULL REFERENCES option_groups(id) ON DELETE CASCADE,
  key          text NOT NULL CHECK (key ~ '^[a-z0-9_]+$'),
  name         text NOT NULL,
  description  text NOT NULL DEFAULT '',
  price_minor  bigint NOT NULL DEFAULT 0,
  -- Semantic 3D parts this value shows, e.g. {"show": ["lapel_peak"], "hide": ["lapel_notch"]}.
  asset_parts  jsonb NOT NULL DEFAULT '{}',
  -- Dimension adjustments per fit zone in mm, e.g. {"jacket_length": 20}.
  adjustments  jsonb NOT NULL DEFAULT '{}',
  is_default   boolean NOT NULL DEFAULT false,
  sort_order   integer NOT NULL DEFAULT 0,
  active       boolean NOT NULL DEFAULT true,
  archived_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (group_id, key)
);
CREATE TRIGGER option_values_updated BEFORE UPDATE ON option_values FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE measurement_fields (
  id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  key            text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9_]+$'),
  label          text NOT NULL,
  body_location  text NOT NULL DEFAULT '',
  instruction    text NOT NULL DEFAULT '',
  helper_note    text NOT NULL DEFAULT '',
  diagram_key    text,
  kind           text NOT NULL DEFAULT 'circumference' CHECK (kind IN ('circumference','length','width','height')),
  min_mm         integer NOT NULL CHECK (min_mm > 0),
  max_mm         integer NOT NULL,
  sort_order     integer NOT NULL DEFAULT 0,
  active         boolean NOT NULL DEFAULT true,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now(),
  CHECK (max_mm > min_mm)
);
CREATE TRIGGER measurement_fields_updated BEFORE UPDATE ON measurement_fields FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE garment_measurement_fields (
  garment_type_id  uuid NOT NULL REFERENCES garment_types(id) ON DELETE CASCADE,
  field_id         uuid NOT NULL REFERENCES measurement_fields(id) ON DELETE CASCADE,
  required         boolean NOT NULL DEFAULT true,
  sort_order       integer NOT NULL DEFAULT 0,
  PRIMARY KEY (garment_type_id, field_id)
);

-- Data-driven fit rules. Ease values are per fit preference.
CREATE TABLE fit_rules (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  garment_type_id  uuid NOT NULL REFERENCES garment_types(id) ON DELETE CASCADE,
  zone             text NOT NULL CHECK (zone ~ '^[a-z0-9_]+$'),
  label            text NOT NULL,
  measurement_key  text NOT NULL,
  kind             text NOT NULL DEFAULT 'circumference' CHECK (kind IN ('circumference','length')),
  ease_slim_mm     integer NOT NULL DEFAULT 0,
  ease_regular_mm  integer NOT NULL DEFAULT 0,
  ease_relaxed_mm  integer NOT NULL DEFAULT 0,
  tolerance_mm     integer NOT NULL DEFAULT 10 CHECK (tolerance_mm > 0),
  stretch_pct      integer NOT NULL DEFAULT 0 CHECK (stretch_pct BETWEEN 0 AND 50),
  version          integer NOT NULL DEFAULT 1,
  updated_by       uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at       timestamptz NOT NULL DEFAULT now(),
  updated_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE (garment_type_id, zone)
);
CREATE TRIGGER fit_rules_updated BEFORE UPDATE ON fit_rules FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Standard size baselines (garment variants) with finished garment dimensions per zone.
CREATE TABLE garment_sizes (
  id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  garment_type_id  uuid NOT NULL REFERENCES garment_types(id) ON DELETE CASCADE,
  label            text NOT NULL,
  dims             jsonb NOT NULL DEFAULT '{}',
  sort_order       integer NOT NULL DEFAULT 0,
  UNIQUE (garment_type_id, label)
);

CREATE TABLE fabrics (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  key                  text NOT NULL UNIQUE CHECK (key ~ '^[a-z0-9-]+$'),
  name                 text NOT NULL,
  material_type        text NOT NULL,
  composition          text NOT NULL DEFAULT '',
  weight_gsm           integer CHECK (weight_gsm IS NULL OR weight_gsm > 0),
  texture_description  text NOT NULL DEFAULT '',
  season               text NOT NULL DEFAULT 'all season',
  care_instructions    text NOT NULL DEFAULT '',
  price_impact_minor   bigint NOT NULL DEFAULT 0,
  stock_status         text NOT NULL DEFAULT 'available' CHECK (stock_status IN ('available','low_stock','out_of_stock','discontinued','custom_order')),
  stock_meters         numeric(8,2),
  low_stock_meters     numeric(8,2),
  swatch_url           text,
  pbr                  jsonb NOT NULL DEFAULT '{}',
  suitable_garments    text[] NOT NULL DEFAULT '{}',
  sort_order           integer NOT NULL DEFAULT 0,
  active               boolean NOT NULL DEFAULT true,
  archived_at          timestamptz,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE TRIGGER fabrics_updated BEFORE UPDATE ON fabrics FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE fabric_colors (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  fabric_id     uuid NOT NULL REFERENCES fabrics(id) ON DELETE CASCADE,
  key           text NOT NULL CHECK (key ~ '^[a-z0-9-]+$'),
  name          text NOT NULL,
  hex           text NOT NULL CHECK (hex ~ '^#[0-9a-fA-F]{6}$'),
  swatch_url    text,
  stock_status  text NOT NULL DEFAULT 'available' CHECK (stock_status IN ('available','low_stock','out_of_stock','discontinued','custom_order')),
  sort_order    integer NOT NULL DEFAULT 0,
  UNIQUE (fabric_id, key)
);

-- Versioned 3D asset manifests.
CREATE TABLE asset_manifests (
  id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  asset_key            text NOT NULL CHECK (asset_key ~ '^[a-z0-9-]+$'),
  version              integer NOT NULL CHECK (version > 0),
  kind                 text NOT NULL CHECK (kind IN ('body','garment','texture_pack','preview_render')),
  garment_type_id      uuid REFERENCES garment_types(id) ON DELETE SET NULL,
  status               text NOT NULL DEFAULT 'uploaded' CHECK (status IN ('uploaded','validating','processed','preview','approved','published','archived','rejected')),
  files                jsonb NOT NULL DEFAULT '[]',
  body_compat          jsonb NOT NULL DEFAULT '{}',
  supported_options    jsonb NOT NULL DEFAULT '{}',
  texture_set_version  text NOT NULL DEFAULT '',
  license              jsonb NOT NULL DEFAULT '{}',
  production_quality   boolean NOT NULL DEFAULT false,
  notes                text NOT NULL DEFAULT '',
  created_by           uuid REFERENCES users(id) ON DELETE SET NULL,
  created_at           timestamptz NOT NULL DEFAULT now(),
  updated_at           timestamptz NOT NULL DEFAULT now(),
  UNIQUE (asset_key, version)
);
CREATE UNIQUE INDEX asset_manifests_one_published ON asset_manifests (asset_key) WHERE status = 'published';
CREATE TRIGGER asset_manifests_updated BEFORE UPDATE ON asset_manifests FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE measurement_profiles (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id         uuid NOT NULL REFERENCES customers(id) ON DELETE CASCADE,
  name                text NOT NULL DEFAULT 'Personal',
  is_default          boolean NOT NULL DEFAULT false,
  body_model          text NOT NULL DEFAULT 'masculine' CHECK (body_model IN ('masculine','feminine')),
  unit                text NOT NULL DEFAULT 'cm' CHECK (unit IN ('cm','in')),
  age_range           text,
  fit_preference      text NOT NULL DEFAULT 'regular' CHECK (fit_preference IN ('slim','regular','relaxed')),
  current_version_id  uuid,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz
);
CREATE UNIQUE INDEX measurement_profiles_one_default ON measurement_profiles (customer_id) WHERE is_default AND deleted_at IS NULL;
CREATE TRIGGER measurement_profiles_updated BEFORE UPDATE ON measurement_profiles FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Immutable measurement snapshots. Orders reference a specific version.
CREATE TABLE measurement_versions (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  profile_id   uuid REFERENCES measurement_profiles(id) ON DELETE SET NULL,
  customer_id  uuid REFERENCES customers(id) ON DELETE SET NULL,
  version_no   integer NOT NULL,
  source       text NOT NULL CHECK (source IN ('customer_entered','tailor_verified','imported','estimated')),
  height_mm    integer CHECK (height_mm IS NULL OR height_mm BETWEEN 1200 AND 2300),
  body_model   text NOT NULL DEFAULT 'masculine',
  fit_preference text NOT NULL DEFAULT 'regular',
  values_mm    jsonb NOT NULL DEFAULT '{}',
  entered      jsonb NOT NULL DEFAULT '{}',
  unit         text NOT NULL DEFAULT 'cm',
  notes        text NOT NULL DEFAULT '',
  review_flags jsonb NOT NULL DEFAULT '[]',
  created_by   uuid REFERENCES users(id) ON DELETE SET NULL,
  verified_by  uuid REFERENCES users(id) ON DELETE SET NULL,
  verified_at  timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX measurement_versions_no ON measurement_versions (profile_id, version_no) WHERE profile_id IS NOT NULL;
ALTER TABLE measurement_profiles ADD CONSTRAINT measurement_profiles_current_fk
  FOREIGN KEY (current_version_id) REFERENCES measurement_versions(id) ON DELETE SET NULL;

CREATE OR REPLACE FUNCTION measurement_versions_immutable() RETURNS trigger AS $$
BEGIN
  IF TG_OP = 'UPDATE' AND (NEW.values_mm IS DISTINCT FROM OLD.values_mm OR NEW.height_mm IS DISTINCT FROM OLD.height_mm
      OR NEW.source IS DISTINCT FROM OLD.source OR NEW.version_no IS DISTINCT FROM OLD.version_no) THEN
    RAISE EXCEPTION 'measurement versions are immutable';
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
CREATE TRIGGER measurement_versions_guard BEFORE UPDATE ON measurement_versions
  FOR EACH ROW EXECUTE FUNCTION measurement_versions_immutable();

CREATE TABLE designs (
  id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  customer_id         uuid REFERENCES customers(id) ON DELETE CASCADE,
  guest_token_hash    bytea,
  name                text NOT NULL DEFAULT 'Untitled design',
  garment_type_key    text NOT NULL,
  current_version_id  uuid,
  version             integer NOT NULL DEFAULT 1,
  created_at          timestamptz NOT NULL DEFAULT now(),
  updated_at          timestamptz NOT NULL DEFAULT now(),
  deleted_at          timestamptz,
  CHECK (customer_id IS NOT NULL OR guest_token_hash IS NOT NULL)
);
CREATE INDEX designs_customer ON designs (customer_id) WHERE deleted_at IS NULL;
CREATE TRIGGER designs_updated BEFORE UPDATE ON designs FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- Reproducible, immutable design snapshots.
CREATE TABLE design_versions (
  id                      uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  design_id               uuid NOT NULL REFERENCES designs(id) ON DELETE CASCADE,
  version_no              integer NOT NULL,
  snapshot                jsonb NOT NULL,
  garment_type_key        text NOT NULL,
  asset_key               text,
  asset_version           integer,
  fabric_key              text,
  color_key               text,
  measurement_version_id  uuid REFERENCES measurement_versions(id) ON DELETE SET NULL,
  estimated_price_minor   bigint NOT NULL DEFAULT 0,
  notes                   text NOT NULL DEFAULT '',
  created_at              timestamptz NOT NULL DEFAULT now(),
  UNIQUE (design_id, version_no)
);
ALTER TABLE designs ADD CONSTRAINT designs_current_fk
  FOREIGN KEY (current_version_id) REFERENCES design_versions(id) ON DELETE SET NULL;

CREATE TABLE product_categories (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug        text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]+$'),
  name        text NOT NULL,
  sort_order  integer NOT NULL DEFAULT 0
);

CREATE TABLE products (
  id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  slug              text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]+$'),
  name              text NOT NULL,
  summary           text NOT NULL DEFAULT '',
  description       text NOT NULL DEFAULT '',
  category_id       uuid REFERENCES product_categories(id) ON DELETE SET NULL,
  garment_type_id   uuid REFERENCES garment_types(id) ON DELETE SET NULL,
  fabric_id         uuid REFERENCES fabrics(id) ON DELETE SET NULL,
  fit_notes         text NOT NULL DEFAULT '',
  care              text NOT NULL DEFAULT '',
  measurement_info  text NOT NULL DEFAULT '',
  requires_fitting  boolean NOT NULL DEFAULT false,
  customizable      boolean NOT NULL DEFAULT false,
  visibility        text NOT NULL DEFAULT 'draft' CHECK (visibility IN ('draft','published','hidden','archived')),
  price_minor       bigint NOT NULL CHECK (price_minor >= 0),
  featured          boolean NOT NULL DEFAULT false,
  sort_order        integer NOT NULL DEFAULT 0,
  version           integer NOT NULL DEFAULT 1,
  created_at        timestamptz NOT NULL DEFAULT now(),
  updated_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_visible ON products (visibility, featured, sort_order);
CREATE INDEX products_search ON products USING gin (to_tsvector('simple', name || ' ' || summary || ' ' || description));
CREATE TRIGGER products_updated BEFORE UPDATE ON products FOR EACH ROW EXECUTE FUNCTION set_updated_at();

CREATE TABLE product_variants (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id   uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  sku          text NOT NULL UNIQUE,
  size_label   text NOT NULL DEFAULT 'One size',
  color_name   text NOT NULL DEFAULT '',
  color_hex    text CHECK (color_hex IS NULL OR color_hex ~ '^#[0-9a-fA-F]{6}$'),
  price_minor  bigint CHECK (price_minor IS NULL OR price_minor >= 0),
  stock_qty    integer NOT NULL DEFAULT 0 CHECK (stock_qty >= 0),
  made_to_order boolean NOT NULL DEFAULT false,
  sort_order   integer NOT NULL DEFAULT 0,
  active       boolean NOT NULL DEFAULT true
);
CREATE INDEX product_variants_product ON product_variants (product_id);

CREATE TABLE product_media (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  product_id  uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
  url         text NOT NULL,
  upload_id   uuid REFERENCES uploads(id) ON DELETE SET NULL,
  alt         text NOT NULL,
  width       integer,
  height      integer,
  sort_order  integer NOT NULL DEFAULT 0
);
CREATE INDEX product_media_product ON product_media (product_id, sort_order);
