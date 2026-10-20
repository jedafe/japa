-- 000909_wiki_entity_content_seed.up.sql
-- JCE ↔ wiki mapping review fix (content depth): visa kinds beyond
-- student/work had no routes or values, and embassies.origin_space_id was
-- NULL (monitored_embassies never recorded origin).
--
--   * visa routes: business + PR pathway (zero routes) plus more
--     tourist/family for kind coverage across real countries
--   * cfv values: overview fields on every seeded route so wiki pages and
--     JCE comparisons render real data
--   * embassies: real rows with host + origin set (the audit test record
--     keeps origin NULL — its origin was never recorded)

BEGIN;

-- ---------------------------------------------------------------------------
-- 1) Visa routes — business, PR pathway, tourist, family (idempotent on slug).
-- ---------------------------------------------------------------------------
INSERT INTO visa_routes (slug, name, visa_category, destination_space_id, is_active, gov_source_url)
SELECT v.slug, v.name, v.cat, s.id, true, 'https://example.gov/' || v.slug
FROM (VALUES
    ('uk-skilled-worker-visa',     'UK Skilled Worker Visa',      'work',     'united-kingdom'),
    ('uk-student-visa',            'UK Student Visa',             'student',  'united-kingdom'),
    ('uk-family-visa',             'UK Family Visa',              'family',   'united-kingdom'),
    ('uk-visitor-visa',            'UK Standard Visitor Visa',    'tourist',  'united-kingdom'),
    ('australia-student-visa',     'Australia Student Visa',      'student',  'australia'),
    ('australia-work-visa',        'Australia Temporary Skill Visa','work',   'australia'),
    ('australia-pr-pathway',       'Australia PR Pathway',        'pr',       'australia'),
    ('canada-pr-pathway',          'Canada Express Entry PR',     'pr',       'canada'),
    ('germany-work-visa',          'Germany EU Blue Card',        'work',     'germany'),
    ('germany-business-visa',      'Germany Business Visa',       'business', 'germany'),
    ('portugal-d7-visa',           'Portugal D7 Passive Income',  'work',     'portugal'),
    ('portugal-golden-visa',       'Portugal Golden Visa',        'business', 'portugal'),
    ('spain-digital-nomad-visa',   'Spain Digital Nomad Visa',    'work',     'spain'),
    ('uae-business-visa',          'UAE Business Visa',           'business', 'uae'),
    ('usa-b1-business-visa',       'USA B1 Business Visa',        'business', 'united-states'),
    ('usa-tourist-visa',           'USA B2 Tourist Visa',         'tourist',  'united-states'),
    ('ireland-family-visa',        'Ireland Family Reunification','family',   'ireland')
) AS v(slug, name, cat, dest_slug)
JOIN spaces s ON s.slug = v.dest_slug AND s.level = 'country'
WHERE NOT EXISTS (SELECT 1 FROM visa_routes r WHERE r.slug = v.slug);

-- ---------------------------------------------------------------------------
-- 2) Overview values on every seeded route — the same fields the wiki page
--    and the JCE kind comparison render.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_values (entity_type, entity_id, field_id, value_text, value_number)
SELECT 'visa_route', r.id, f.id,
    CASE f.key
        WHEN 'route_type'       THEN r.visa_category
        WHEN 'processing_time'  THEN coalesce(v.proc, '6-10 weeks')
        WHEN 'eligibility'      THEN coalesce(v.elig, 'Meet the route requirements; valid passport; supporting documents.')
        WHEN 'official_link'    THEN 'https://example.gov/' || r.slug
        ELSE NULL END,
    CASE WHEN f.key = 'application_fee' THEN coalesce(v.fee, 150) ELSE NULL END
FROM visa_routes r
JOIN custom_fields f ON f.group_id IN (
    SELECT id FROM custom_field_groups
    WHERE slug = 'wiki_' || CASE r.visa_category WHEN 'pr' THEN 'pr_pathway' ELSE r.visa_category || '_visa' END || '_overview'
)
LEFT JOIN (VALUES
    ('uk-skilled-worker-visa',   '3 weeks',      'Job offer from a licensed sponsor; English proficiency.',      719::numeric),
    ('uk-student-visa',          '3 weeks',      'CAS from a licensed student sponsor; proof of funds.',        490::numeric),
    ('uk-family-visa',           '24 weeks',     'UK-settled family member; income threshold.',                 1846::numeric),
    ('uk-visitor-visa',          '3 weeks',      'Tourism or business visits up to 6 months.',                  115::numeric),
    ('australia-student-visa',   '4-8 weeks',    'CoE from an Australian institution; GTE assessment.',         710::numeric),
    ('australia-work-visa',      '4-6 months',   'Skilled occupation list; sponsorship or points test.',        3115::numeric),
    ('australia-pr-pathway',     '8-12 months',  'Points test ≥65; occupation on skilled list.',                4640::numeric),
    ('canada-pr-pathway',        '6 months',     'Express Entry CRS score; education + language tests.',        1525::numeric),
    ('germany-work-visa',        '1-3 months',   'Recognised degree; salary threshold for Blue Card.',          75::numeric),
    ('germany-business-visa',    '2-4 weeks',    'Business purpose documentation; travel insurance.',           80::numeric),
    ('portugal-d7-visa',         '60-90 days',   'Passive income above minimum wage; Portuguese NIF.',          90::numeric),
    ('portugal-golden-visa',     '9-12 months',  'Qualifying investment (fund, job creation).',                 6050::numeric),
    ('spain-digital-nomad-visa', '20 days',      'Remote work proof; income 200% of minimum wage.',             80::numeric),
    ('uae-business-visa',        '5-7 days',     'Business invitation or trade license.',                       100::numeric),
    ('usa-b1-business-visa',     '2-6 weeks',    'Business purpose; strong home-country ties.',                 185::numeric),
    ('usa-tourist-visa',         '2-6 weeks',    'Tourism purpose; strong home-country ties.',                  185::numeric),
    ('ireland-family-visa',      '6-12 months',  'Irish resident family member; relationship proof.',           60::numeric)
) AS v(slug, proc, elig, fee) ON v.slug = r.slug
WHERE r.slug IN (
    'uk-skilled-worker-visa','uk-student-visa','uk-family-visa','uk-visitor-visa',
    'australia-student-visa','australia-work-visa','australia-pr-pathway',
    'canada-pr-pathway','germany-work-visa','germany-business-visa',
    'portugal-d7-visa','portugal-golden-visa','spain-digital-nomad-visa',
    'uae-business-visa','usa-b1-business-visa','usa-tourist-visa','ireland-family-visa',
    'canada-tourist-visa','chile-family-visa'
)
ON CONFLICT DO NOTHING;

-- ---------------------------------------------------------------------------
-- 3) Embassies with real host + origin (dual-country model).
-- ---------------------------------------------------------------------------
INSERT INTO embassies (slug, name, host_space_id, origin_space_id, address, phone, website, booking_url, is_active)
SELECT e.slug, e.name, host.id, orig.id, e.address, e.phone, e.website,
       'https://' || e.slug || '.example.gov/appointments', true
FROM (VALUES
    ('us-embassy-abuja',       'US Embassy Abuja',      'nigeria',       'united-states',  'Plot 1075 Diplomatic Drive, Central District, Abuja', '+234 9 461 4000', 'https://ng.usembassy.gov'),
    ('uk-high-commission-abuja','British High Commission Abuja','nigeria','united-kingdom','19 Torrens Close, Maitama, Abuja',                       '+234 9 462 2200', 'https://www.gov.uk/world/nigeria'),
    ('canada-high-commission-abuja','Canada High Commission Abuja','nigeria','canada',     '13010G, Palm Close, Maitama, Abuja',                     '+234 9 461 2900', 'https://www.international.gc.ca/country-pays/nigeria'),
    ('german-embassy-abuja',   'German Embassy Abuja',  'nigeria',       'germany',        '9 Lake Chad Crescent, Maitama, Abuja',                 '+234 9 461 1010', 'https://nigeria.diplo.de'),
    ('us-embassy-accra',       'US Embassy Accra',      'ghana',         'united-states',  '24 Fourth Circular Rd, Cantonments, Accra',            '+233 30 274 1000', 'https://gh.usembassy.gov'),
    ('uk-high-commission-accra','British High Commission Accra','ghana', 'united-kingdom', 'Osu Link, off Gamel Abdul Nasser Avenue, Accra',         '+233 30 221 3200', 'https://www.gov.uk/world/ghana'),
    ('us-embassy-nairobi',     'US Embassy Nairobi',    'kenya',         'united-states',  'United Nations Avenue, Gigiri, Nairobi',               '+254 20 363 6000', 'https://ke.usembassy.gov'),
    ('nigeria-high-commission-london','Nigeria High Commission London','united-kingdom','nigeria','9 Northumberland Avenue, London WC2N 5BX',       '+44 20 7839 1244', 'https://nigeriahc.org.uk')
) AS e(slug, name, host_slug, orig_slug, address, phone, website)
JOIN spaces host ON host.slug = e.host_slug AND host.level = 'country'
JOIN spaces orig ON orig.slug = e.orig_slug AND orig.level = 'country'
WHERE NOT EXISTS (SELECT 1 FROM embassies x WHERE x.slug = e.slug);

COMMIT;
