-- 001199_attestation_editorial_seo.up.sql
-- Section 97 (Attestation Guide) + Section 14 (Content Type System) —
--
-- 1) Expand the attestation_pathways document_type CHECK to the full
--    canonical list. The original six (birth/marriage/degree/police/
--    professional licence/other) missed several document families that
--    routinely go through apostille/legalisation for emigration:
--    transcripts, civil-status records (death/divorce/single-status/
--    adoption), powers of attorney, driving licences, employment letters,
--    company-registration documents, and medical certificates.
--
-- 2) Seed the "attestation" guide category + field group so the editorial
--    team can author indexable attestation guides in /studio/create
--    (?type=guide). Field keys mirror the attestation_pathways domain:
--    document_type × issuing_country × destination_country, apostille flag,
--    ordered pathway_steps (repeater with the §97 step sub-keys), a
--    last_verified date, and an optional pathway_id cross-link to a live
--    pathway row in the interactive tool. Fields are optional — editorial
--    guides are prose-led and sparse field_values must not fail
--    required-field validation on unrelated edits (001120 precedent).
--
-- Ref: japaconnect-engineering/sections/97-attestation-guide.md
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

-- ---------------------------------------------------------------------------
-- 1) Widen the document_type CHECK (drop + re-add NOT VALID → VALIDATE keeps
--    the long scan off the write path; all existing rows satisfy the new,
--    strictly larger list).
-- ---------------------------------------------------------------------------
ALTER TABLE public.attestation_pathways
    DROP CONSTRAINT IF EXISTS attestation_pathways_document_type_check;

ALTER TABLE public.attestation_pathways
    ADD CONSTRAINT attestation_pathways_document_type_check
    CHECK (document_type IN (
        'birth_certificate', 'degree', 'academic_transcript', 'marriage',
        'death_certificate', 'divorce_certificate', 'single_status',
        'adoption_papers', 'police_clearance', 'professional_licence',
        'power_of_attorney', 'driving_licence', 'employment_letter',
        'company_registration', 'medical_certificate', 'other'
    )) NOT VALID;

ALTER TABLE public.attestation_pathways
    VALIDATE CONSTRAINT attestation_pathways_document_type_check;

-- ---------------------------------------------------------------------------
-- 2) Attestation guide category for /studio/create?type=guide.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('guide_category:attestation', 'Attestation Guide', false, 14)
ON CONFLICT (value) DO NOTHING;

INSERT INTO custom_field_groups (name, slug, description, is_system)
VALUES ('Attestation Fields', 'guide_attestation_fields', 'Structured fields for attestation guide content (§97).', false)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:attestation', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_attestation_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:attestation'
  );

-- Field keys mirror the §97 pathway domain so editorial values map 1:1 onto
-- the tool's domain model. document_type options mirror the widened CHECK
-- above — keep them in sync.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Document Type', 'document_type', 'select',
    '[{"value":"birth_certificate","label":"Birth certificate"},{"value":"degree","label":"Degree"},{"value":"academic_transcript","label":"Academic transcript"},{"value":"marriage","label":"Marriage certificate"},{"value":"death_certificate","label":"Death certificate"},{"value":"divorce_certificate","label":"Divorce certificate"},{"value":"single_status","label":"Single-status certificate"},{"value":"adoption_papers","label":"Adoption papers"},{"value":"police_clearance","label":"Police clearance"},{"value":"professional_licence","label":"Professional licence"},{"value":"power_of_attorney","label":"Power of attorney"},{"value":"driving_licence","label":"Driving licence"},{"value":"employment_letter","label":"Employment letter"},{"value":"company_registration","label":"Company registration"},{"value":"medical_certificate","label":"Medical certificate"},{"value":"other","label":"Other"}]'::jsonb,
    false, false, true, true, true, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Issuing Country', 'issuing_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 2
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Apostille Required', 'apostille_required', 'boolean', NULL,
    false, false, true, true, false, false, 4
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- pathway_steps — structured repeater; sub-keys mirror the §97
-- attestation_pathways.pathway_steps object keys so editorial step data
-- renders through the same ordered-fields helper the tool uses.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Pathway Steps', 'pathway_steps', 'repeater',
    '{"item_fields":[{"key":"step","label":"Step"},{"key":"agency","label":"Agency"},{"key":"what_to_bring","label":"What to bring"},{"key":"fee_estimate","label":"Fee estimate"},{"key":"turnaround_days","label":"Turnaround (days)"},{"key":"official_url","label":"Official URL","type":"url"},{"key":"note","label":"Note"}]}'::jsonb,
    false, false, true, false, false, true, 5
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Last Verified', 'last_verified', 'date', NULL,
    false, false, true, true, true, false, 6
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- pathway_id — optional cross-link to a live attestation_pathways row so
-- the public page can deep-link straight to the interactive pathway.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Linked Pathway ID', 'pathway_id', 'text', NULL,
    false, false, true, true, false, true, 7
FROM custom_field_groups g WHERE g.slug = 'guide_attestation_fields'
ON CONFLICT (group_id, key) DO NOTHING;

COMMIT;
