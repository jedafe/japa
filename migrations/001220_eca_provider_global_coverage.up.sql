-- 001220_eca_provider_global_coverage.up.sql
-- Section 109 (ECA Guide) — completes the provider catalog to global
-- coverage: every ENIC-NARIC national information centre (the official
-- network of recognition authorities under the Lisbon Recognition
-- Convention + EU NARIC), plus the major non-European national
-- equivalency bodies (Japan, China, South Africa, Kenya, India) and
-- the newest Lisbon parties (Kyrgyzstan, Uzbekistan).
--
-- Centres whose own public domain is not confidently known link the
-- official ENIC-NARIC gateway (enic-naric.net), which lists every
-- member centre — a wrong guess would be worse than the canonical
-- directory. Already-seeded national bodies (ZAB, QQI, Nuffic, CIMEA,
-- NOKUT, UHR, FEI, Ecctis, MCC, PEBC) keep their direct URLs.
--
-- Slugs are stable identifiers used by editorial field_values and the
-- ?provider= filter. Idempotent upserts.
--
-- Ref: japaconnect-engineering/sections/109-eca-guide.md

BEGIN;

INSERT INTO eca_providers (slug, name, website)
SELECT v.slug, v.name, v.website
FROM (VALUES
    -- ── ENIC-NARIC member centres (alphabetical by country) ────────────
    ('enic-albania',          'ENIC Albania (Ministry of Education and Sport)',                        'https://www.enic-naric.net'),
    ('enic-andorra',          'ENIC Andorra (Ministry of Education)',                                  'https://www.enic-naric.net'),
    ('armenic',               'ArmENIC (Armenia — National Center for Professional Education QA)',     'https://armenic.am'),
    ('enic-austria',          'ENIC NARIC AUSTRIA (Federal Ministry of Education)',                    'https://www.enic-naric.at'),
    ('enic-azerbaijan',       'ENIC Azerbaijan (Ministry of Science and Education)',                   'https://www.enic-naric.net'),
    ('enic-belarus',          'ENIC Belarus (Ministry of Education)',                                  'https://www.enic-naric.net'),
    ('naric-vlaanderen',      'NARIC-Vlaanderen (Belgium — Flanders)',                                 'https://www.naricvlaanderen.be'),
    ('enic-fwb',              'ENIC-NARIC Fédération Wallonie-Bruxelles (Belgium)',                    'https://www.enic-naric.be'),
    ('enic-ostbelgien',       'ENIC German-speaking Community of Belgium',                             'https://www.enic-naric.net'),
    ('cip',                   'CIP Bosnia and Herzegovina (Centre for Information and Recognition)',   'https://www.cip.gov.ba'),
    ('enic-bulgaria',         'ENIC Bulgaria (Ministry of Education and Science)',                     'https://www.enic-naric.net'),
    ('cicic',                 'CICIC (Canadian Information Centre for International Credentials)',     'https://www.cicic.ca'),
    ('enic-croatia',          'ENIC/NARIC Croatia (Agency for Science and Higher Education)',          'https://www.azvo.hr'),
    ('kysats',                'KYSATS (Cyprus Council for Recognition of HE Qualifications)',          'https://www.kysats.ac.cy'),
    ('enic-czech',            'ENIC-NARIC Czech Republic (DZS)',                                       'https://www.dzs.cz'),
    ('enic-denmark',          'ENIC/NARIC Denmark (Agency for Higher Education and Science)',          'https://ufm.dk/en'),
    ('enic-estonia',          'Estonian ENIC/NARIC Centre (HARNO)',                                    'https://harno.ee/en'),
    ('oph-finland',           'Finnish National Agency for Education (ENIC Finland, OPH)',             'https://www.oph.fi/en'),
    ('nceqe',                 'NCEQE Georgia (National Center for Educational Quality Enhancement)',   'https://eqe.ge'),
    ('enic-greece',           'Hellenic NARIC (Greece — ENIC-NARIC centre)',                           'https://www.enic-naric.net'),
    ('enic-holy-see',         'ENIC Holy See',                                                         'https://www.enic-naric.net'),
    ('enic-hungary',          'Hungarian ENIC (Educational Authority)',                                'https://www.enic-naric.net'),
    ('enic-iceland',          'ENIC-NARIC Iceland',                                                    'https://www.enic-naric.net'),
    ('enic-israel',           'ENIC-NARIC Israel (Ministry of Education)',                             'https://www.enic-naric.net'),
    ('enic-kazakhstan',       'ENIC Kazakhstan',                                                       'https://enic-kazakhstan.edu.kz'),
    ('enic-kyrgyzstan',       'ENIC Kyrgyzstan',                                                       'https://www.enic-naric.net'),
    ('aic-latvia',            'Academic Information Centre Latvia (ENIC-NARIC)',                       'https://www.aic.lv'),
    ('aiba-liechtenstein',    'AIBA Liechtenstein (National Agency for International Education)',      'https://www.enic-naric.net'),
    ('skvc',                  'SKVC Lithuania (Centre for Quality Assessment in HE, ENIC-NARIC)',      'https://www.skvc.lt'),
    ('enic-luxembourg',       'ENIC-NARIC Luxembourg (Ministry of Higher Education and Research)',     'https://www.enic-naric.net'),
    ('mqric',                 'MQRIC Malta (Malta Qualifications Recognition Information Centre)',     'https://mfhea.mt'),
    ('enic-moldova',          'ENIC Moldova (Ministry of Education)',                                  'https://www.enic-naric.net'),
    ('enic-monaco',           'ENIC Monaco (Department of Education)',                                 'https://www.enic-naric.net'),
    ('enic-montenegro',       'ENIC Montenegro (Ministry of Education)',                               'https://www.enic-naric.net'),
    ('enic-north-macedonia',  'ENIC North Macedonia (Ministry of Education and Science)',              'https://www.enic-naric.net'),
    ('nawa',                  'NAWA Poland (Polish National Agency for Academic Exchange, ENIC-NARIC)','https://nawa.gov.pl'),
    ('dges',                  'DGES Portugal (Direção-Geral do Ensino Superior, NARIC)',               'https://www.dges.gov.pt'),
    ('cnred',                 'CNRED Romania (National Centre for Recognition of Diplomas)',           'https://www.cnred.edu.ro'),
    ('enic-russia',           'ENIC Russia (National Information Centre)',                             'https://nic.gov.ru'),
    ('enic-san-marino',       'ENIC San Marino',                                                       'https://www.enic-naric.net'),
    ('enic-serbia',           'ENIC Serbia (Ministry of Education)',                                   'https://www.enic-naric.net'),
    ('enic-slovakia',         'ENIC-NARIC Slovakia (Ministry of Education)',                           'https://www.enic-naric.net'),
    ('enic-slovenia',         'ENIC-NARIC Slovenia (Ministry of Education)',                           'https://www.enic-naric.net'),
    ('enic-spain',            'ENIC-NARIC Spain (Ministry of Universities)',                           'https://www.universidades.gob.es'),
    ('enic-switzerland',      'Swiss ENIC (swissuniversities)',                                        'https://www.swissuniversities.ch'),
    ('enic-tajikistan',       'ENIC Tajikistan',                                                       'https://www.enic-naric.net'),
    ('enic-turkey',           'ENIC-NARIC Turkey (Council of Higher Education, YÖK)',                  'https://www.enic-naric.net'),
    ('enic-ukraine',          'ENIC Ukraine (Ministry of Education and Science)',                      'https://enic.in.ua'),
    ('enic-uzbekistan',       'ENIC Uzbekistan',                                                       'https://www.enic-naric.net'),
    -- ── Non-European national equivalency bodies ────────────────────────
    ('nic-japan',             'NIC-Japan (National Information Center for Academic Recognition)',      'https://www.nic-japan.jp/en'),
    ('cscse',                 'CSCSE China (Chinese Service Center for Scholarly Exchange)',           'https://www.cscse.edu.cn'),
    ('saqa',                  'SAQA (South African Qualifications Authority)',                         'https://www.saqa.org.za'),
    ('knqa',                  'KNQA (Kenya National Qualifications Authority)',                        'https://www.knqa.go.ke'),
    ('aiu',                   'AIU (Association of Indian Universities — Equivalence Division)',       'https://www.aiu.ac.in')
) AS v(slug, name, website)
ON CONFLICT (slug) DO UPDATE SET
    name    = EXCLUDED.name,
    website = EXCLUDED.website;

COMMIT;
