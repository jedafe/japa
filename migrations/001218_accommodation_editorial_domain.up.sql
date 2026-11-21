-- 001218_accommodation_editorial_domain.up.sql
-- Section 96 (Housing Guide) + Section 14 (Content Type System) — studio
-- editorial integration, mirroring the §97 attestation pattern (001199)
-- and the §109/§94 ECA/credential pattern (001217).
--
-- The baseline seed shipped a `guide_accommodation_fields` group shaped
-- like a rental-listing form (required `country`/`city`, display-label
-- enum options, monthly_cost_usd/deposit_required/lease_duration/
-- is_furnished/utilities_included/booking_link). That shape neither maps
-- onto the housing_guides domain nor follows the editorial convention the
-- other tool guides use. This migration re-keys the group onto the §96
-- domain:
--
--   destination_country (geo_code suggest) × city (free text → city
--   space inside the destination) × accommodation_type (§96 enum) is the
--   tool's natural key; search_platforms + rent_ranges are structured
--   repeaters; landlord_requirements/scam_categories/tips are prose;
--   last_verified feeds housing_guides.last_verified_at; guide_id is the
--   optional explicit cross-link (attestation pathway_id precedent).
--
-- Fields are optional — editorial guides are prose-led (001120/001199
-- precedent). Safe to rewrite: no guide_category='accommodation' content
-- rows exist yet.
--
-- Ref: japaconnect-engineering/sections/96-housing-guide.md
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

UPDATE custom_field_groups
SET description = 'Structured fields for housing/accommodation guide content (§96).'
WHERE slug = 'guide_accommodation_fields';

-- Replace the listing-form fields wholesale — zero guide content rows
-- reference them, and keeping stale keys would double-write into
-- field_values under keys the publish hook doesn't read.
DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_accommodation_fields';

-- destination_country — geo_countries suggest stores the geo code; the
-- publish hook resolves it to the destination geo space (§97 pattern).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- city — free text; the publish hook resolves it to a city-level space
-- inside the destination (city→state→country parent chain). Empty =
-- country-level guide (NULL city_space_id per §96).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'City (optional)', 'city', 'text', NULL,
    false, false, true, true, true, true, 2
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- accommodation_type options mirror the §96 enum — keep in sync with the
-- constants in internal/housingguide/store.go. Free text stays storable
-- (the column is unconstrained; existing rows use e.g. 'apartment').
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Accommodation Type', 'accommodation_type', 'select',
    '[{"value":"private_rental","label":"Private rental"},{"value":"flatshare","label":"Flatshare"},{"value":"student","label":"Student housing"},{"value":"employer_provided","label":"Employer-provided"},{"value":"all","label":"All types"}]'::jsonb,
    false, false, true, true, true, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- search_platforms — sub-keys mirror the §96 {name, url, description}
-- object shape housing_guides.search_platforms stores.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Search Platforms', 'search_platforms', 'repeater',
    '{"item_fields":[{"key":"name","label":"Platform name"},{"key":"url","label":"URL","type":"url"},{"key":"description","label":"Description"}]}'::jsonb,
    false, false, true, false, false, true, 4
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- rent_ranges — one row per neighbourhood tier; the publish hook maps
-- rows into the §96 {tier: {studio, 1bed, 2bed, currency}} object shape.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Rent Ranges', 'rent_ranges', 'repeater',
    '{"item_fields":[{"key":"tier","label":"Tier (city_centre | inner | outer)"},{"key":"studio","label":"Studio"},{"key":"one_bed","label":"1-bed"},{"key":"two_bed","label":"2-bed"},{"key":"currency","label":"Currency"}]}'::jsonb,
    false, false, true, false, false, true, 5
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Landlord Requirements', 'landlord_requirements', 'textarea', NULL,
    false, false, true, false, false, true, 6
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- scam_categories — one per line; the publish hook parses the textarea
-- into the text[] shape housing_guides stores.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Scam Categories (one per line)', 'scam_categories', 'textarea', NULL,
    false, false, true, false, false, true, 7
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Tips', 'tips', 'textarea', NULL,
    false, false, true, false, false, true, 8
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Last Verified', 'last_verified', 'date', NULL,
    false, false, true, true, true, false, 9
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- guide_id — optional explicit cross-link to a live housing_guides row so
-- the public page can deep-link straight to the interactive guide
-- (attestation pathway_id precedent). Auto-backfilled on publish.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Linked Housing Guide ID', 'guide_id', 'text', NULL,
    false, false, true, true, false, true, 10
FROM custom_field_groups g WHERE g.slug = 'guide_accommodation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
