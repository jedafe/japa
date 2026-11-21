-- 001217_eca_credential_editorial_domains.up.sql
-- Sections 109 (ECA Guide), 94 (Professional Credential Checker) +
-- 14 (Content Type System) — studio editorial integration, mirroring the
-- §97 attestation pattern shipped in 001199.
--
-- 1) eca_guides.guide_id — editorial cross-link column, same role as the
--    credential_pathways.guide_id column that already exists in baseline.
--
-- 2) "eca" + "credential" guide categories + field groups so the editorial
--    team can author indexable tool guides in /studio/create (?type=guide).
--    Field keys mirror each tool's domain natural key:
--      eca        → provider slug × destination_country geo code × purpose
--      credential → profession slug × issuing_country × destination_country
--    plus the structured pathway_steps repeater and an optional explicit
--    guide_id/pathway_id cross-link. Fields are optional — editorial guides
--    are prose-led (001120/001199 precedent).
--
-- 3) Comprehensive provider + profession catalogs. The demo seed
--    (db/seed/tools_eca_credential_examples.sql) ships only 5 providers and
--    5 professions; these idempotent upserts install the full Japa.to
--    launch catalog — IRCC-designated ECAs (WES/IQAS/ICAS/CES/ICES), the
--    US NACES evaluators, UK ENIC, and the national recognition bodies for
--    the other Japa.to corridors (NZQA, uni-assist/ZAB, QQI, Nuffic), plus
--    the regulated professions that dominate emigrant licensing demand.
--
-- Ref: japaconnect-engineering/sections/109-eca-guide.md
-- Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
-- Ref: japaconnect-engineering/sections/14-content-type-system.md

BEGIN;

-- ---------------------------------------------------------------------------
-- 1) eca_guides.guide_id — optional link back to the editorial guide that
--    claims this tool row (credential_pathways.guide_id parity).
-- ---------------------------------------------------------------------------
ALTER TABLE public.eca_guides
    ADD COLUMN IF NOT EXISTS guide_id uuid;

CREATE INDEX IF NOT EXISTS idx_eca_guides_guide_id
    ON public.eca_guides (guide_id)
    WHERE guide_id IS NOT NULL;

-- ---------------------------------------------------------------------------
-- 2a) Guide categories for /studio/create?type=guide.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('guide_category:eca', 'ECA Guide', false, 15)
ON CONFLICT (value) DO NOTHING;

INSERT INTO custom_field_entity_types (value, label, is_system, sort_order)
VALUES ('guide_category:credential', 'Credential Checker Guide', false, 16)
ON CONFLICT (value) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2b) ECA editorial field group — keys mirror the eca_guides domain.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_groups (name, slug, description, is_system)
VALUES ('ECA Guide Fields', 'guide_eca_fields', 'Structured fields for ECA (credential assessment) guide content (§109).', false)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:eca', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_eca_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:eca'
  );

-- provider — stores the eca_providers.slug (suggest source resolves the
-- live provider list; free text stays allowed per §06 suggest semantics).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Assessment Provider', 'provider', 'suggest', '{"source":"eca_providers"}'::jsonb,
    false, false, true, true, true, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 2
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- purpose options mirror the §109 eca_guides.purpose enum — keep in sync
-- with the CHECK constraint and the service-side validation list.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Purpose', 'purpose', 'select',
    '[{"value":"immigration","label":"Immigration"},{"value":"study","label":"Study"},{"value":"employment","label":"Employment"},{"value":"professional_licensure","label":"Professional licensure"}]'::jsonb,
    false, false, true, true, true, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- pathway_steps — sub-keys mirror the eca_guides.pathway_steps object
-- shape ({step, title, detail, duration}) so editorial steps hand off to
-- the tool row without reshaping.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Pathway Steps', 'pathway_steps', 'repeater',
    '{"item_fields":[{"key":"step","label":"Step"},{"key":"title","label":"Title"},{"key":"detail","label":"Detail"},{"key":"duration","label":"Duration"},{"key":"official_url","label":"Official URL","type":"url"},{"key":"note","label":"Note"}]}'::jsonb,
    false, false, true, false, false, true, 4
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- documents_required — one document per line; the publish hook parses the
-- textarea into the jsonb string-array shape eca_guides stores.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Documents Required (one per line)', 'documents_required', 'textarea', NULL,
    false, false, true, false, false, true, 5
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Accepted For (one per line)', 'accepted_for', 'textarea', NULL,
    false, false, true, false, false, true, 6
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Fee Estimate', 'fee_estimate', 'text', NULL,
    false, false, true, true, true, true, 7
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Currency', 'currency', 'suggest', '{"source":"wallet_currencies"}'::jsonb,
    false, false, true, true, false, true, 8
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Timeline Estimate', 'timeline_estimate', 'text', NULL,
    false, false, true, true, true, true, 9
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Official URL', 'official_url', 'url', NULL,
    false, false, true, true, false, true, 10
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Last Verified', 'last_verified', 'date', NULL,
    false, false, true, true, true, false, 11
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- guide_id — optional explicit cross-link to a live eca_guides row
-- (pathway_id parity on the attestation group).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Linked Guide ID', 'guide_id', 'text', NULL,
    false, false, true, true, false, true, 12
FROM custom_field_groups g WHERE g.slug = 'guide_eca_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 2c) Credential checker editorial field group.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_groups (name, slug, description, is_system)
VALUES ('Credential Checker Fields', 'guide_credential_fields', 'Structured fields for professional credential-checker guide content (§94).', false)
ON CONFLICT (slug) DO NOTHING;

INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, 'content_type:guide:credential', NULL, 0
FROM custom_field_groups g
WHERE g.slug = 'guide_credential_fields'
  AND NOT EXISTS (
      SELECT 1 FROM custom_field_group_assignments a
      WHERE a.group_id = g.id AND a.entity_class = 'content_type:guide:credential'
  );

-- profession — stores the credential_professions.slug.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Profession', 'profession', 'suggest', '{"source":"credential_professions"}'::jsonb,
    false, false, true, true, true, true, 1
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Issuing Country', 'issuing_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 2
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Destination Country', 'destination_country', 'suggest', '{"source":"geo_countries"}'::jsonb,
    false, false, true, true, false, true, 3
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Regulatory Body', 'regulatory_body', 'text', NULL,
    false, false, true, true, true, true, 4
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- pathway_type options mirror the §94 credential_pathways.pathway_type
-- enum — keep in sync with the CHECK constraint.
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Pathway Type', 'pathway_type', 'select',
    '[{"value":"direct","label":"Direct recognition"},{"value":"bridging_exam","label":"Bridging / exam"},{"value":"supervised_practice","label":"Supervised practice"},{"value":"full_requalification","label":"Full requalification"}]'::jsonb,
    false, false, true, true, true, true, 5
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Pathway Steps', 'pathway_steps', 'repeater',
    '{"item_fields":[{"key":"step","label":"Step"},{"key":"title","label":"Title"},{"key":"detail","label":"Detail"},{"key":"duration","label":"Duration"},{"key":"official_url","label":"Official URL","type":"url"},{"key":"note","label":"Note"}]}'::jsonb,
    false, false, true, false, false, true, 6
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Timeline Estimate', 'timeline_estimate', 'text', NULL,
    false, false, true, true, true, true, 7
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Cost Estimate', 'cost_estimate', 'text', NULL,
    false, false, true, true, true, true, 8
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Official Application URL', 'official_application_url', 'url', NULL,
    false, false, true, true, false, true, 9
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- pathway_id — optional explicit cross-link to a live credential_pathways
-- row (same name as the attestation group's link field — the categories
-- are disjoint so the key can be shared).
INSERT INTO custom_fields (group_id, label, key, field_type, options, is_required, is_system, show_in_frontend, filterable, sortable, searchable, position)
SELECT g.id, 'Linked Pathway ID', 'pathway_id', 'text', NULL,
    false, false, true, true, false, true, 10
FROM custom_field_groups g WHERE g.slug = 'guide_credential_fields'
ON CONFLICT (group_id, key) DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3a) ECA provider catalog (§109) — IRCC-designated organisations, the
--     major US NACES/member evaluators, UK ENIC, and the national
--     recognition bodies for the other Japa.to corridors. Slugs are stable
--     identifiers used by editorial field_values and the ?provider= filter.
-- ---------------------------------------------------------------------------
INSERT INTO eca_providers (slug, name, website)
SELECT v.slug, v.name, v.website
FROM (VALUES
    -- IRCC-designated for Canadian immigration ECA.
    ('wes',        'World Education Services (WES)',                          'https://www.wes.org'),
    ('iqas',       'International Qualifications Assessment Service (IQAS)', 'https://www.alberta.ca/iqas'),
    ('icas',       'International Credential Assessment Service (ICAS)',     'https://www.icascanada.ca'),
    ('ces',        'Comparative Education Service (CES)',                    'https://learn.utoronto.ca/programs-courses/comparative-education-service'),
    ('ices',       'International Credential Evaluation Service (ICES)',     'https://www.bcit.ca/ices'),
    -- United States evaluators.
    ('ece',        'Educational Credential Evaluators (ECE)',                'https://www.ece.org'),
    ('jsa',        'Josef Silny & Associates (JSA)',                         'https://www.jsilny.org'),
    ('ierf',       'International Education Research Foundation (IERF)',     'https://www.ierf.org'),
    ('iee',        'International Education Evaluations (IEE)',              'https://www.myiee.org'),
    ('spantran',   'SpanTran Evaluation Services',                           'https://www.spantran.com'),
    ('gce',        'Global Credential Evaluators (GCE)',                     'https://www.gceus.com'),
    ('eres',       'Educational Records Evaluation Service (ERES)',          'https://www.eres.com'),
    ('fce',        'Foreign Credential Evaluations (FCE)',                   'https://www.fceatlanta.net'),
    -- United Kingdom.
    ('ecctis',     'Ecctis (UK ENIC)',                                       'https://www.enic.org.uk'),
    -- Australia / New Zealand.
    ('vetassess',  'VETASSESS',                                              'https://www.vetassess.com.au'),
    ('nzqa',       'New Zealand Qualifications Authority (NZQA)',            'https://www.nzqa.govt.nz'),
    -- Germany.
    ('uni-assist', 'uni-assist',                                             'https://www.uni-assist.de'),
    ('zab',        'Zentralstelle für ausländisches Bildungswesen (ZAB)',    'https://zab.kmk.org'),
    -- Ireland / Netherlands.
    ('qqi',        'Quality and Qualifications Ireland (QQI / NARIC)',       'https://www.qqi.ie'),
    ('nuffic',     'Nuffic (IDW credential evaluation)',                     'https://www.nuffic.nl'),
    -- Catch-all for organisations not catalogued yet (§109).
    ('icewg',      'Other designated ECA organisation',                      NULL),
    ('other',      'Other credential assessment body',                       NULL)
) AS v(slug, name, website)
ON CONFLICT (slug) DO UPDATE SET
    name    = EXCLUDED.name,
    website = EXCLUDED.website;

-- ---------------------------------------------------------------------------
-- 3b) Profession catalog (§94) — the regulated professions that dominate
--     Japa.to licensing demand, grouped by the §94 categories.
-- ---------------------------------------------------------------------------
INSERT INTO credential_professions (slug, name, category)
SELECT v.slug, v.name, v.category
FROM (VALUES
    ('physician',                    'Physician / Medical Doctor',            'medical'),
    ('registered-nurse',             'Registered Nurse',                      'medical'),
    ('midwife',                      'Midwife',                               'medical'),
    ('pharmacist',                   'Pharmacist',                            'medical'),
    ('dentist',                      'Dentist',                               'medical'),
    ('physiotherapist',              'Physiotherapist',                       'medical'),
    ('medical-laboratory-scientist', 'Medical Laboratory Scientist',          'medical'),
    ('radiographer',                 'Radiographer',                          'medical'),
    ('occupational-therapist',       'Occupational Therapist',                'medical'),
    ('speech-language-therapist',    'Speech & Language Therapist',           'medical'),
    ('optometrist',                  'Optometrist',                           'medical'),
    ('clinical-psychologist',        'Clinical Psychologist',                 'medical'),
    ('dietitian',                    'Dietitian',                             'medical'),
    ('paramedic',                    'Paramedic',                             'medical'),
    ('veterinarian',                 'Veterinarian',                          'medical'),
    ('civil-engineer',               'Civil Engineer',                        'engineering'),
    ('mechanical-engineer',          'Mechanical Engineer',                   'engineering'),
    ('electrical-engineer',          'Electrical Engineer',                   'engineering'),
    ('chemical-engineer',            'Chemical Engineer',                     'engineering'),
    ('petroleum-engineer',           'Petroleum Engineer',                    'engineering'),
    ('structural-engineer',          'Structural Engineer',                   'engineering'),
    ('geoscientist',                 'Geoscientist',                          'engineering'),
    ('lawyer',                       'Lawyer / Barrister & Solicitor',        'legal'),
    ('notary-public',                'Notary Public',                         'legal'),
    ('paralegal',                    'Paralegal',                             'legal'),
    ('chartered-accountant',         'Chartered Accountant',                  'finance'),
    ('actuary',                      'Actuary',                               'finance'),
    ('early-childhood-educator',     'Early Childhood Educator',              'education'),
    ('primary-teacher',              'Primary School Teacher',                'education'),
    ('secondary-teacher',            'Secondary School Teacher',              'education'),
    ('special-education-teacher',    'Special Education Teacher',             'education'),
    ('architect',                    'Architect',                             'other'),
    ('social-worker',                'Social Worker',                         'other'),
    ('quantity-surveyor',            'Quantity Surveyor',                     'other'),
    ('urban-regional-planner',       'Urban & Regional Planner',              'other'),
    ('land-surveyor',                'Land Surveyor',                         'other'),
    ('airline-pilot',                'Airline Pilot',                         'other'),
    ('electrician',                  'Electrician',                           'other'),
    ('plumber',                      'Plumber',                               'other'),
    ('welder',                       'Welder',                                'other')
) AS v(slug, name, category)
ON CONFLICT (slug) DO UPDATE SET
    name     = EXCLUDED.name,
    category = EXCLUDED.category;

COMMIT;
