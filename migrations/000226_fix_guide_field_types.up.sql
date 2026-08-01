-- Migration 000226: Fix seeded guide field types
--
-- The original seed (migration 000225) used "text" for fields that should
-- use more specific types: duration, multiselect, percentage, repeater.
-- This migration updates the field_type column for those fields so the
-- composer renders the correct input widget.
--
-- Repeater is a new field type added in this migration for multi-value
-- free-text lists (e.g. eligible countries, required exams, transit hubs).

-- ===== Visa Guide =====
UPDATE custom_fields SET field_type = 'duration' WHERE key = 'processing_time' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_visa_fields');
UPDATE custom_fields SET field_type = 'duration' WHERE key = 'validity_period' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_visa_fields');

-- ===== Travel Routes =====
UPDATE custom_fields SET field_type = 'duration' WHERE key = 'travel_duration' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_travel_routes_fields');
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'transit_hubs' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_travel_routes_fields');
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'recommended_airlines' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_travel_routes_fields');

-- ===== Scholarship Guide =====
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'countries_eligible' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_scholarship_fields');

-- ===== Admission Process =====
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'required_exams' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_admission_fields');
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'language_requirements' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_admission_fields');
UPDATE custom_fields SET field_type = 'percentage' WHERE key = 'acceptance_rate' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_admission_fields');

-- ===== Accommodation Guide =====
UPDATE custom_fields SET field_type = 'duration' WHERE key = 'lease_duration' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_accommodation_fields');

-- ===== Banking & Finance =====
UPDATE custom_fields SET field_type = 'currency' WHERE key = 'international_transfer_fee' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_banking_fields');

-- ===== Work & Career =====
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'in_demand_sectors' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_work_career_fields');
UPDATE custom_fields SET field_type = 'percentage' WHERE key = 'tax_rate' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_work_career_fields');

-- ===== Health & Insurance =====
-- (no changes needed — all fields already use correct types)

-- ===== Culture & Lifestyle =====
UPDATE custom_fields SET field_type = 'repeater' WHERE key = 'language_spoken' AND group_id = (SELECT id FROM custom_field_groups WHERE slug = 'guide_culture_lifestyle_fields');
