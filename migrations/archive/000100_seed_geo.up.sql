-- Phase 05: Geo bulk seed — countries and states as Primary Spaces.
-- Ref: japaconnect-engineering/roadmaps/ROADMAP-BACKEND.md Phase 05 (task 05.05)
-- Sources: open "Countries States Cities" dataset (ADR-020).
-- One spaces row per country (level=country) and per state/province (level=state).
-- Cities are NOT bulk-seeded — admin-created on demand per ADR-020.
-- Idempotent: skip if slug already exists.

-- ============================================================
-- Countries (level=country, space_type = country)
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'country'),
    v.name,
    lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'country',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Nigeria', 'NG'),
    ('United Kingdom', 'GB'),
    ('Canada', 'CA'),
    ('United States', 'US'),
    ('Ireland', 'IE'),
    ('Germany', 'DE'),
    ('Netherlands', 'NL'),
    ('South Africa', 'ZA'),
    ('United Arab Emirates', 'AE'),
    ('Ghana', 'GH'),
    ('France', 'FR')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- ============================================================
-- States/Provinces (level=state, parented to country via space_parent_spaces)
-- ============================================================

-- Nigeria states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ng-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abia', 'AB'), ('Adamawa', 'AD'), ('Akwa Ibom', 'AK'), ('Anambra', 'AN'),
    ('Bauchi', 'BA'), ('Bayelsa', 'BY'), ('Benue', 'BE'), ('Borno', 'BO'),
    ('Cross River', 'CR'), ('Delta', 'DE'), ('Ebonyi', 'EB'), ('Edo', 'ED'),
    ('Ekiti', 'EK'), ('Enugu', 'EN'), ('FCT', 'FC'), ('Gombe', 'GO'),
    ('Imo', 'IM'), ('Jigawa', 'JI'), ('Kaduna', 'KD'), ('Kano', 'KN'),
    ('Katsina', 'KT'), ('Kebbi', 'KE'), ('Kogi', 'KO'), ('Kwara', 'KW'),
    ('Lagos', 'LA'), ('Nasarawa', 'NA'), ('Niger', 'NI'), ('Ogun', 'OG'),
    ('Ondo', 'ON'), ('Osun', 'OS'), ('Oyo', 'OY'), ('Plateau', 'PL'),
    ('Rivers', 'RI'), ('Sokoto', 'SO'), ('Taraba', 'TA'), ('Yobe', 'YO'),
    ('Zamfara', 'ZA')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Link Nigeria states to Nigeria country
INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'nigeria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ng-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United Kingdom
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gb-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('England', 'ENG'), ('Scotland', 'SCT'), ('Wales', 'WLS'), ('Northern Ireland', 'NIR')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-kingdom' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gb-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Canada
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ca-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ontario', 'ON'), ('Quebec', 'QC'), ('British Columbia', 'BC'), ('Alberta', 'AB'),
    ('Manitoba', 'MB'), ('Saskatchewan', 'SK'), ('Nova Scotia', 'NS'),
    ('New Brunswick', 'NB'), ('Newfoundland and Labrador', 'NL'),
    ('Prince Edward Island', 'PE'), ('Northwest Territories', 'NT'),
    ('Yukon', 'YT'), ('Nunavut', 'NU')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'canada' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ca-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United States (key states)
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'us-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('California', 'CA'), ('Texas', 'TX'), ('New York', 'NY'), ('Florida', 'FL'),
    ('Illinois', 'IL'), ('Pennsylvania', 'PA'), ('Ohio', 'OH'), ('Georgia', 'GA'),
    ('North Carolina', 'NC'), ('Michigan', 'MI'), ('New Jersey', 'NJ'),
    ('Virginia', 'VA'), ('Washington', 'WA'), ('Arizona', 'AZ'),
    ('Massachusetts', 'MA'), ('Tennessee', 'TN'), ('Indiana', 'IN'),
    ('Maryland', 'MD'), ('Missouri', 'MO'), ('Wisconsin', 'WI'),
    ('Colorado', 'CO'), ('Minnesota', 'MN'), ('South Carolina', 'SC'),
    ('Alabama', 'AL'), ('Louisiana', 'LA'), ('Kentucky', 'KY'),
    ('Oregon', 'OR'), ('Oklahoma', 'OK'), ('Connecticut', 'CT'),
    ('Utah', 'UT'), ('Iowa', 'IA'), ('Nevada', 'NV'), ('Arkansas', 'AR'),
    ('Mississippi', 'MS'), ('Kansas', 'KS'), ('New Mexico', 'NM'),
    ('Nebraska', 'NE'), ('Idaho', 'ID'), ('West Virginia', 'WV'),
    ('Hawaii', 'HI'), ('New Hampshire', 'NH'), ('Maine', 'ME'),
    ('Montana', 'MT'), ('Rhode Island', 'RI'), ('Delaware', 'DE'),
    ('South Dakota', 'SD'), ('North Dakota', 'ND'), ('Alaska', 'AK'),
    ('Vermont', 'VT'), ('Wyoming', 'WY')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-states' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'us-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ireland
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ie-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Leinster', 'L'), ('Munster', 'M'), ('Connacht', 'C'), ('Ulster', 'U')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ireland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ie-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Germany
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'de-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Baden-Württemberg', 'BW'), ('Bavaria', 'BY'), ('Berlin', 'BE'),
    ('Brandenburg', 'BB'), ('Bremen', 'HB'), ('Hamburg', 'HH'),
    ('Hesse', 'HE'), ('Lower Saxony', 'NI'), ('Mecklenburg-Vorpommern', 'MV'),
    ('North Rhine-Westphalia', 'NW'), ('Rhineland-Palatinate', 'RP'),
    ('Saarland', 'SL'), ('Saxony', 'SN'), ('Saxony-Anhalt', 'ST'),
    ('Schleswig-Holstein', 'SH'), ('Thuringia', 'TH')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'germany' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'de-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Netherlands
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nl-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('North Holland', 'NH'), ('South Holland', 'ZH'), ('Utrecht', 'UT'),
    ('North Brabant', 'NB'), ('Gelderland', 'GE'), ('Overijssel', 'OV'),
    ('Limburg', 'LI'), ('Friesland', 'FR'), ('Groningen', 'GR'),
    ('Drenthe', 'DR'), ('Flevoland', 'FL'), ('Zeeland', 'ZE')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'netherlands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- South Africa
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'za-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Gauteng', 'GP'), ('KwaZulu-Natal', 'KZN'), ('Western Cape', 'WC'),
    ('Eastern Cape', 'EC'), ('Free State', 'FS'), ('Limpopo', 'LP'),
    ('Mpumalanga', 'MP'), ('North West', 'NW'), ('Northern Cape', 'NC')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'south-africa' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'za-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United Arab Emirates
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ae-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Dubai', 'DU'), ('Abu Dhabi', 'AZ'), ('Sharjah', 'SH'),
    ('Ajman', 'AJ'), ('Ras Al Khaimah', 'RK'), ('Fujairah', 'FU'),
    ('Umm Al Quwain', 'UQ')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-arab-emirates' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ae-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ghana
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gh-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Greater Accra', 'AA'), ('Ashanti', 'AH'), ('Western', 'WE'),
    ('Central', 'CE'), ('Eastern', 'EP'), ('Northern', 'NO'),
    ('Volta', 'VO'), ('Brong-Ahafo', 'BA'), ('Upper East', 'UE'),
    ('Upper West', 'UW')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ghana' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- France
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fr-' || lower(replace(replace(replace(v.name, ' ', '-'), '''', ''), '.', '')),
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Île-de-France', 'IDF'), ('Auvergne-Rhône-Alpes', 'ARA'),
    ('Hauts-de-France', 'HDF'), ('Nouvelle-Aquitaine', 'NAQ'),
    ('Occitanie', 'OCC'), ('Grand Est', 'GES'), ('Pays de la Loire', 'PDL'),
    ('Bretagne', 'BRE'), ('Normandie', 'NOR'),
    ('Provence-Alpes-Côte d''Azur', 'PAC'), ('Centre-Val de Loire', 'CVL'),
    ('Bourgogne-Franche-Comté', 'BFC'), ('Corse', 'COR')
) AS v(name, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'france' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;
