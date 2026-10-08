DROP TABLE IF EXISTS product_media, product_variants, products, product_categories, design_versions, designs,
  measurement_versions, measurement_profiles, asset_manifests, fabric_colors, fabrics, garment_sizes, fit_rules,
  garment_measurement_fields, measurement_fields, option_values, option_groups, garment_types CASCADE;
DROP FUNCTION IF EXISTS measurement_versions_immutable();
