-- 001219_eca_provider_expansion.up.sql
-- Section 109 (ECA Guide) — expands the provider catalog to the remaining
-- Japa.to corridor bodies that 001217 did not cover:
--
--   • Canada — the two IRCC-designated bodies reserved for regulated
--     professions: MCC (physicians) and PEBC (pharmacists).
--   • United States — the remaining established NACES/AICE evaluators.
--   • Australia — the occupation assessing authorities that serve the
--     same role as an ECA for skilled migration (TRA, Engineers
--     Australia, ACS, ANMAC, CPA, AITSL).
--   • Europe — additional national recognition centres (CIMEA, NOKUT,
--     UHR, France Éducation international).
--   • Gulf — DataFlow (primary-source verification for UAE/QA/SA
--     licensure corridors).
--
-- Idempotent upserts on slug — the natural identifier used by editorial
-- field_values and the ?provider= filter.
--
-- Ref: japaconnect-engineering/sections/109-eca-guide.md

BEGIN;

INSERT INTO eca_providers (slug, name, website)
SELECT v.slug, v.name, v.website
FROM (VALUES
    -- Canada — IRCC-designated bodies for regulated professions.
    ('mcc',  'Medical Council of Canada (physicians)',     'https://physiciansapply.ca'),
    ('pebc', 'Pharmacy Examining Board of Canada (PEBC)',  'https://www.pebc.ca'),
    -- United States — additional NACES/AICE evaluators.
    ('educational-perspectives', 'Educational Perspectives',                  'https://www.edperspective.org'),
    ('facs',                   'Foreign Academic Credentials Service (FACS)', 'https://www.facsusa.com'),
    ('fis',                    'Foundation for International Services (FIS)', 'https://www.fis-web.com'),
    ('transcript-research',    'Transcript Research',                         'https://transcriptresearch.com'),
    ('ucredo',                 'Ucredo',                                      'https://ucredo.com'),
    ('aes',                    'Academic Evaluation Services (AES)',          'https://www.aes-edu.org'),
    -- Australia — skilled-migration assessing authorities.
    ('tra',                 'Trades Recognition Australia (TRA)',   'https://www.tradesrecognitionaustralia.gov.au'),
    ('engineers-australia', 'Engineers Australia',                  'https://www.engineersaustralia.org.au'),
    ('acs',                 'Australian Computer Society (ACS)',    'https://www.acs.org.au'),
    ('anmac',               'Australian Nursing & Midwifery Accreditation Council (ANMAC)', 'https://www.anmac.org.au'),
    ('cpaa',                'CPA Australia',                        'https://www.cpaaustralia.com.au'),
    ('aitsl',               'Australian Institute for Teaching and School Leadership (AITSL)', 'https://www.aitsl.edu.au'),
    -- Europe — national recognition centres.
    ('cimea', 'CIMEA (Italy ENIC-NARIC)',                  'https://cimea.diplo-me.it'),
    ('nokut', 'NOKUT (Norway)',                            'https://www.nokut.no'),
    ('uhr',   'Swedish Council for Higher Education (UHR)','https://www.uhr.se'),
    ('fei',   'France Éducation international (ENIC-NARIC)','https://www.france-education-international.fr'),
    -- Gulf — primary-source verification for GCC licensure.
    ('dataflow', 'DataFlow Group (Gulf PSV)', 'https://www.dataflowgroup.com')
) AS v(slug, name, website)
ON CONFLICT (slug) DO UPDATE SET
    name    = EXCLUDED.name,
    website = EXCLUDED.website;

COMMIT;
