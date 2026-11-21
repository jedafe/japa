-- 001221_professions_expansion_purposes.up.sql
-- Sections 94 (Professional Credential Checker) + 109 (ECA Guide):
--
-- 1) Expands the profession catalog to comprehensive regulated-occupation
--    coverage for Japa.to corridors — licensure-track health roles
--    (LPN, NP, dental hygiene, pharmacy tech, respiratory therapy,
--    audiology, podiatry, chiropractic, osteopathy, opticianry,
--    physician associate, counselling, prosthetics, vet tech), the
--    remaining professional-engineering disciplines incl. software and
--    engineering technologist, legal roles incl. immigration consultant
--    (RCIC), finance roles incl. statutory audit/advice, education
--    support roles, and the Red Seal / TRA trade occupations plus the
--    licensed commercial roles migrants actually enter (real estate,
--    customs broker, security, sworn translation, drivers).
--
-- 2) Extends the §109 purpose enum from 4 to 7 values — scholarship
--    (japa.to's scholarship pathway; funding applications require ECA
--    reports), professional_membership (chartered body membership,
--    distinct from statutory licensure), and pension_recognition
--    (EU/state pension equivalence — a documented ZAB use case). The
--    enum is enforced in ecaguide service code (no CHECK constraint);
--    this migration keeps the editorial purpose select options in sync.
--
-- Ref: japaconnect-engineering/sections/94-professional-credential-checker.md
-- Ref: japaconnect-engineering/sections/109-eca-guide.md

BEGIN;

-- ---------------------------------------------------------------------------
-- 1) Profession catalog expansion (idempotent upserts on slug).
-- ---------------------------------------------------------------------------
INSERT INTO credential_professions (slug, name, category)
SELECT v.slug, v.name, v.category
FROM (VALUES
    -- Health — additional licensure-track roles.
    ('licensed-practical-nurse',     'Licensed Practical / Vocational Nurse',      'medical'),
    ('nurse-practitioner',           'Nurse Practitioner',                         'medical'),
    ('dental-hygienist',             'Dental Hygienist',                           'medical'),
    ('dental-technician',            'Dental Technician / Technologist',           'medical'),
    ('pharmacy-technician',          'Pharmacy Technician',                        'medical'),
    ('respiratory-therapist',        'Respiratory Therapist',                      'medical'),
    ('audiologist',                  'Audiologist',                                'medical'),
    ('podiatrist',                   'Podiatrist / Chiropodist',                   'medical'),
    ('chiropractor',                 'Chiropractor',                               'medical'),
    ('osteopath',                    'Osteopath / Osteopathic Practitioner',       'medical'),
    ('dispensing-optician',          'Dispensing Optician',                        'medical'),
    ('physician-associate',          'Physician Associate / Assistant',            'medical'),
    ('mental-health-counsellor',     'Mental Health Counsellor / Psychotherapist', 'medical'),
    ('prosthetist-orthotist',        'Prosthetist / Orthotist',                    'medical'),
    ('veterinary-technician',        'Veterinary Technician / Technologist',       'medical'),
    -- Engineering — remaining licensed disciplines.
    ('software-engineer',            'Software Engineer',                          'engineering'),
    ('computer-engineer',            'Computer Engineer',                          'engineering'),
    ('mining-engineer',              'Mining Engineer',                            'engineering'),
    ('environmental-engineer',       'Environmental Engineer',                     'engineering'),
    ('industrial-engineer',          'Industrial Engineer',                        'engineering'),
    ('aerospace-engineer',           'Aerospace Engineer',                         'engineering'),
    ('biomedical-engineer',          'Biomedical Engineer',                        'engineering'),
    ('marine-engineer',              'Marine Engineer / Naval Architect',          'engineering'),
    ('metallurgical-engineer',       'Metallurgical / Materials Engineer',         'engineering'),
    ('agricultural-engineer',        'Agricultural Engineer',                      'engineering'),
    ('engineering-technologist',     'Certified Engineering Technologist (C.E.T)', 'engineering'),
    -- Legal — additional regulated roles.
    ('patent-attorney',              'Patent / Trademark Attorney & Agent',        'legal'),
    ('conveyancer',                  'Licensed Conveyancer',                       'legal'),
    ('immigration-consultant',       'Regulated Immigration Consultant (RCIC)',    'legal'),
    ('legal-executive',              'Chartered Legal Executive',                  'legal'),
    -- Finance — regulated advice/audit roles.
    ('statutory-auditor',            'Statutory Auditor',                          'finance'),
    ('financial-adviser',            'Financial Adviser / Planner',                'finance'),
    ('insurance-agent-broker',       'Insurance Agent / Broker',                   'finance'),
    ('mortgage-broker',              'Mortgage Broker / Adviser',                  'finance'),
    ('tax-adviser',                  'Tax Adviser / Consultant',                   'finance'),
    -- Education — regulated support roles.
    ('vocational-teacher',           'Vocational / Technical Education Teacher',   'education'),
    ('school-counsellor',            'School Counsellor',                          'education'),
    ('school-psychologist',          'School Psychologist',                        'education'),
    -- Other — Red Seal / TRA trades and licensed commercial roles.
    ('carpenter',                    'Carpenter',                                  'other'),
    ('hvac-technician',              'HVAC / Refrigeration Technician',            'other'),
    ('elevator-mechanic',            'Elevator / Escalator Mechanic',              'other'),
    ('millwright',                   'Millwright / Industrial Mechanic',           'other'),
    ('sheet-metal-worker',           'Sheet Metal Worker',                         'other'),
    ('ironworker',                   'Ironworker / Structural Steel Worker',       'other'),
    ('boilermaker',                  'Boilermaker',                                'other'),
    ('automotive-technician',        'Automotive Service Technician / Mechanic',   'other'),
    ('aircraft-maintenance-engineer','Aircraft Maintenance Engineer (AME)',        'other'),
    ('marine-officer',               'Marine Officer / Seafarer (STCW)',           'other'),
    ('train-driver',                 'Train Driver / Locomotive Engineer',         'other'),
    ('truck-driver',                 'Truck Driver (CDL / HGV)',                   'other'),
    ('heavy-equipment-operator',     'Heavy Equipment Operator',                   'other'),
    ('crane-operator',               'Crane Operator',                             'other'),
    ('hairdresser-barber',           'Hairdresser / Barber',                       'other'),
    ('cosmetologist',                'Cosmetologist / Esthetician',                'other'),
    ('chef',                         'Chef / Cook',                                'other'),
    ('baker',                        'Baker / Pastry Chef',                        'other'),
    ('butcher',                      'Butcher / Meat Cutter',                      'other'),
    ('real-estate-agent',            'Real Estate Agent / Realtor',                'other'),
    ('customs-broker',               'Licensed Customs Broker',                    'other'),
    ('sworn-translator',             'Sworn / Certified Translator & Interpreter', 'other'),
    ('security-guard',               'Security Guard / Officer',                   'other'),
    ('building-inspector',           'Building Inspector / Surveyor',              'other')
) AS v(slug, name, category)
ON CONFLICT (slug) DO UPDATE SET
    name     = EXCLUDED.name,
    category = EXCLUDED.category;

-- ---------------------------------------------------------------------------
-- 2) ECA purpose select options — sync with the extended §109 enum
--    (immigration, study, scholarship, employment,
--     professional_licensure, professional_membership,
--     pension_recognition).
-- ---------------------------------------------------------------------------
UPDATE custom_fields f
SET options = '[
  {"value":"immigration","label":"Immigration"},
  {"value":"study","label":"Study"},
  {"value":"scholarship","label":"Scholarship"},
  {"value":"employment","label":"Employment"},
  {"value":"professional_licensure","label":"Professional licensure"},
  {"value":"professional_membership","label":"Professional membership"},
  {"value":"pension_recognition","label":"Pension recognition"}
]'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_eca_fields'
  AND f.key = 'purpose';

COMMIT;
