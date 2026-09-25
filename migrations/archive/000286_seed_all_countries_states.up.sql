-- 000286_seed_all_countries_states.up.sql
-- Full geo seed: ALL ~250 countries and ~5,300 states/provinces as Primary Spaces.
-- Ref: ADR-020, ROADMAP-BACKEND.md Phase 05 task 05.05.
--
-- Migration 000100 only seeded 11 countries (~175 states) using hardcoded SQL.
-- This migration seeds the remaining countries and states from the open
-- 'Countries States Cities' dataset (dr5hn/countries-states-cities-database).
-- Cities remain admin-created on demand per ADR-020.
-- Idempotent: ON CONFLICT DO NOTHING on (tenant_id, slug).
--
-- After applying, run `go run ./cmd/seed-geo-enqueue` to enqueue
-- space:auto_populate tasks for all country spaces missing wiki_content.

-- ============================================================
-- Countries (level=country) — all 250 from the dataset
-- currency_code is set via a separate UPDATE below because the
-- spaces_currency_code_fkey constraint requires the code to exist
-- in the currencies table (only ~10 currencies are seeded).
-- ============================================================
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'country'),
    v.name,
    v.slug,
    v.code,
    'country',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Afghanistan', 'afghanistan', 'AF'),
    ('Aland Islands', 'aland-islands', 'AX'),
    ('Albania', 'albania', 'AL'),
    ('Algeria', 'algeria', 'DZ'),
    ('American Samoa', 'american-samoa', 'AS'),
    ('Andorra', 'andorra', 'AD'),
    ('Angola', 'angola', 'AO'),
    ('Anguilla', 'anguilla', 'AI'),
    ('Antarctica', 'antarctica', 'AQ'),
    ('Antigua and Barbuda', 'antigua-and-barbuda', 'AG'),
    ('Argentina', 'argentina', 'AR'),
    ('Armenia', 'armenia', 'AM'),
    ('Aruba', 'aruba', 'AW'),
    ('Australia', 'australia', 'AU'),
    ('Austria', 'austria', 'AT'),
    ('Azerbaijan', 'azerbaijan', 'AZ'),
    ('Bahrain', 'bahrain', 'BH'),
    ('Bangladesh', 'bangladesh', 'BD'),
    ('Barbados', 'barbados', 'BB'),
    ('Belarus', 'belarus', 'BY'),
    ('Belgium', 'belgium', 'BE'),
    ('Belize', 'belize', 'BZ'),
    ('Benin', 'benin', 'BJ'),
    ('Bermuda', 'bermuda', 'BM'),
    ('Bhutan', 'bhutan', 'BT'),
    ('Bolivia', 'bolivia', 'BO'),
    ('Bonaire, Sint Eustatius and Saba', 'bonaire-sint-eustatius-and-saba', 'BQ'),
    ('Bosnia and Herzegovina', 'bosnia-and-herzegovina', 'BA'),
    ('Botswana', 'botswana', 'BW'),
    ('Bouvet Island', 'bouvet-island', 'BV'),
    ('Brazil', 'brazil', 'BR'),
    ('British Indian Ocean Territory', 'british-indian-ocean-territory', 'IO'),
    ('Brunei', 'brunei', 'BN'),
    ('Bulgaria', 'bulgaria', 'BG'),
    ('Burkina Faso', 'burkina-faso', 'BF'),
    ('Burundi', 'burundi', 'BI'),
    ('Cambodia', 'cambodia', 'KH'),
    ('Cameroon', 'cameroon', 'CM'),
    ('Canada', 'canada', 'CA'),
    ('Cape Verde', 'cape-verde', 'CV'),
    ('Cayman Islands', 'cayman-islands', 'KY'),
    ('Central African Republic', 'central-african-republic', 'CF'),
    ('Chad', 'chad', 'TD'),
    ('Chile', 'chile', 'CL'),
    ('China', 'china', 'CN'),
    ('Christmas Island', 'christmas-island', 'CX'),
    ('Cocos (Keeling) Islands', 'cocos-keeling-islands', 'CC'),
    ('Colombia', 'colombia', 'CO'),
    ('Comoros', 'comoros', 'KM'),
    ('Congo', 'congo', 'CG'),
    ('Cook Islands', 'cook-islands', 'CK'),
    ('Costa Rica', 'costa-rica', 'CR'),
    ('Croatia', 'croatia', 'HR'),
    ('Cuba', 'cuba', 'CU'),
    ('Curaçao', 'curaçao', 'CW'),
    ('Cyprus', 'cyprus', 'CY'),
    ('Czech Republic', 'czech-republic', 'CZ'),
    ('Democratic Republic of the Congo', 'democratic-republic-of-the-congo', 'CD'),
    ('Denmark', 'denmark', 'DK'),
    ('Djibouti', 'djibouti', 'DJ'),
    ('Dominica', 'dominica', 'DM'),
    ('Dominican Republic', 'dominican-republic', 'DO'),
    ('Ecuador', 'ecuador', 'EC'),
    ('Egypt', 'egypt', 'EG'),
    ('El Salvador', 'el-salvador', 'SV'),
    ('Equatorial Guinea', 'equatorial-guinea', 'GQ'),
    ('Eritrea', 'eritrea', 'ER'),
    ('Estonia', 'estonia', 'EE'),
    ('Eswatini', 'eswatini', 'SZ'),
    ('Ethiopia', 'ethiopia', 'ET'),
    ('Falkland Islands', 'falkland-islands', 'FK'),
    ('Faroe Islands', 'faroe-islands', 'FO'),
    ('Fiji Islands', 'fiji-islands', 'FJ'),
    ('Finland', 'finland', 'FI'),
    ('France', 'france', 'FR'),
    ('French Guiana', 'french-guiana', 'GF'),
    ('French Polynesia', 'french-polynesia', 'PF'),
    ('French Southern Territories', 'french-southern-territories', 'TF'),
    ('Gabon', 'gabon', 'GA'),
    ('Georgia', 'georgia', 'GE'),
    ('Germany', 'germany', 'DE'),
    ('Ghana', 'ghana', 'GH'),
    ('Gibraltar', 'gibraltar', 'GI'),
    ('Greece', 'greece', 'GR'),
    ('Greenland', 'greenland', 'GL'),
    ('Grenada', 'grenada', 'GD'),
    ('Guadeloupe', 'guadeloupe', 'GP'),
    ('Guam', 'guam', 'GU'),
    ('Guatemala', 'guatemala', 'GT'),
    ('Guernsey', 'guernsey', 'GG'),
    ('Guinea', 'guinea', 'GN'),
    ('Guinea-Bissau', 'guinea-bissau', 'GW'),
    ('Guyana', 'guyana', 'GY'),
    ('Haiti', 'haiti', 'HT'),
    ('Heard Island and McDonald Islands', 'heard-island-and-mcdonald-islands', 'HM'),
    ('Honduras', 'honduras', 'HN'),
    ('Hong Kong S.A.R.', 'hong-kong-sar', 'HK'),
    ('Hungary', 'hungary', 'HU'),
    ('Iceland', 'iceland', 'IS'),
    ('India', 'india', 'IN'),
    ('Indonesia', 'indonesia', 'ID'),
    ('Iran', 'iran', 'IR'),
    ('Iraq', 'iraq', 'IQ'),
    ('Ireland', 'ireland', 'IE'),
    ('Israel', 'israel', 'IL'),
    ('Italy', 'italy', 'IT'),
    ('Ivory Coast', 'ivory-coast', 'CI'),
    ('Jamaica', 'jamaica', 'JM'),
    ('Japan', 'japan', 'JP'),
    ('Jersey', 'jersey', 'JE'),
    ('Jordan', 'jordan', 'JO'),
    ('Kazakhstan', 'kazakhstan', 'KZ'),
    ('Kenya', 'kenya', 'KE'),
    ('Kiribati', 'kiribati', 'KI'),
    ('Kosovo', 'kosovo', 'XK'),
    ('Kuwait', 'kuwait', 'KW'),
    ('Kyrgyzstan', 'kyrgyzstan', 'KG'),
    ('Laos', 'laos', 'LA'),
    ('Latvia', 'latvia', 'LV'),
    ('Lebanon', 'lebanon', 'LB'),
    ('Lesotho', 'lesotho', 'LS'),
    ('Liberia', 'liberia', 'LR'),
    ('Libya', 'libya', 'LY'),
    ('Liechtenstein', 'liechtenstein', 'LI'),
    ('Lithuania', 'lithuania', 'LT'),
    ('Luxembourg', 'luxembourg', 'LU'),
    ('Macau S.A.R.', 'macau-sar', 'MO'),
    ('Madagascar', 'madagascar', 'MG'),
    ('Malawi', 'malawi', 'MW'),
    ('Malaysia', 'malaysia', 'MY'),
    ('Maldives', 'maldives', 'MV'),
    ('Mali', 'mali', 'ML'),
    ('Malta', 'malta', 'MT'),
    ('Man (Isle of)', 'man-isle-of', 'IM'),
    ('Marshall Islands', 'marshall-islands', 'MH'),
    ('Martinique', 'martinique', 'MQ'),
    ('Mauritania', 'mauritania', 'MR'),
    ('Mauritius', 'mauritius', 'MU'),
    ('Mayotte', 'mayotte', 'YT'),
    ('Mexico', 'mexico', 'MX'),
    ('Micronesia', 'micronesia', 'FM'),
    ('Moldova', 'moldova', 'MD'),
    ('Monaco', 'monaco', 'MC'),
    ('Mongolia', 'mongolia', 'MN'),
    ('Montenegro', 'montenegro', 'ME'),
    ('Montserrat', 'montserrat', 'MS'),
    ('Morocco', 'morocco', 'MA'),
    ('Mozambique', 'mozambique', 'MZ'),
    ('Myanmar', 'myanmar', 'MM'),
    ('Namibia', 'namibia', 'NA'),
    ('Nauru', 'nauru', 'NR'),
    ('Nepal', 'nepal', 'NP'),
    ('Netherlands', 'netherlands', 'NL'),
    ('New Caledonia', 'new-caledonia', 'NC'),
    ('New Zealand', 'new-zealand', 'NZ'),
    ('Nicaragua', 'nicaragua', 'NI'),
    ('Niger', 'niger', 'NE'),
    ('Nigeria', 'nigeria', 'NG'),
    ('Niue', 'niue', 'NU'),
    ('Norfolk Island', 'norfolk-island', 'NF'),
    ('North Korea', 'north-korea', 'KP'),
    ('North Macedonia', 'north-macedonia', 'MK'),
    ('Northern Mariana Islands', 'northern-mariana-islands', 'MP'),
    ('Norway', 'norway', 'NO'),
    ('Oman', 'oman', 'OM'),
    ('Pakistan', 'pakistan', 'PK'),
    ('Palau', 'palau', 'PW'),
    ('Palestinian Territory Occupied', 'palestinian-territory-occupied', 'PS'),
    ('Panama', 'panama', 'PA'),
    ('Papua New Guinea', 'papua-new-guinea', 'PG'),
    ('Paraguay', 'paraguay', 'PY'),
    ('Peru', 'peru', 'PE'),
    ('Philippines', 'philippines', 'PH'),
    ('Pitcairn Island', 'pitcairn-island', 'PN'),
    ('Poland', 'poland', 'PL'),
    ('Portugal', 'portugal', 'PT'),
    ('Puerto Rico', 'puerto-rico', 'PR'),
    ('Qatar', 'qatar', 'QA'),
    ('Reunion', 'reunion', 'RE'),
    ('Romania', 'romania', 'RO'),
    ('Russia', 'russia', 'RU'),
    ('Rwanda', 'rwanda', 'RW'),
    ('Saint Helena', 'saint-helena', 'SH'),
    ('Saint Kitts and Nevis', 'saint-kitts-and-nevis', 'KN'),
    ('Saint Lucia', 'saint-lucia', 'LC'),
    ('Saint Pierre and Miquelon', 'saint-pierre-and-miquelon', 'PM'),
    ('Saint Vincent and the Grenadines', 'saint-vincent-and-the-grenadines', 'VC'),
    ('Saint-Barthelemy', 'saint-barthelemy', 'BL'),
    ('Saint-Martin (French part)', 'saint-martin-french-part', 'MF'),
    ('Samoa', 'samoa', 'WS'),
    ('San Marino', 'san-marino', 'SM'),
    ('Sao Tome and Principe', 'sao-tome-and-principe', 'ST'),
    ('Saudi Arabia', 'saudi-arabia', 'SA'),
    ('Senegal', 'senegal', 'SN'),
    ('Serbia', 'serbia', 'RS'),
    ('Seychelles', 'seychelles', 'SC'),
    ('Sierra Leone', 'sierra-leone', 'SL'),
    ('Singapore', 'singapore', 'SG'),
    ('Sint Maarten (Dutch part)', 'sint-maarten-dutch-part', 'SX'),
    ('Slovakia', 'slovakia', 'SK'),
    ('Slovenia', 'slovenia', 'SI'),
    ('Solomon Islands', 'solomon-islands', 'SB'),
    ('Somalia', 'somalia', 'SO'),
    ('South Africa', 'south-africa', 'ZA'),
    ('South Georgia', 'south-georgia', 'GS'),
    ('South Korea', 'south-korea', 'KR'),
    ('South Sudan', 'south-sudan', 'SS'),
    ('Spain', 'spain', 'ES'),
    ('Sri Lanka', 'sri-lanka', 'LK'),
    ('Sudan', 'sudan', 'SD'),
    ('Suriname', 'suriname', 'SR'),
    ('Svalbard and Jan Mayen Islands', 'svalbard-and-jan-mayen-islands', 'SJ'),
    ('Sweden', 'sweden', 'SE'),
    ('Switzerland', 'switzerland', 'CH'),
    ('Syria', 'syria', 'SY'),
    ('Taiwan', 'taiwan', 'TW'),
    ('Tajikistan', 'tajikistan', 'TJ'),
    ('Tanzania', 'tanzania', 'TZ'),
    ('Thailand', 'thailand', 'TH'),
    ('The Bahamas', 'the-bahamas', 'BS'),
    ('The Gambia', 'the-gambia', 'GM'),
    ('Timor-Leste', 'timor-leste', 'TL'),
    ('Togo', 'togo', 'TG'),
    ('Tokelau', 'tokelau', 'TK'),
    ('Tonga', 'tonga', 'TO'),
    ('Trinidad and Tobago', 'trinidad-and-tobago', 'TT'),
    ('Tunisia', 'tunisia', 'TN'),
    ('Turkey', 'turkey', 'TR'),
    ('Turkmenistan', 'turkmenistan', 'TM'),
    ('Turks and Caicos Islands', 'turks-and-caicos-islands', 'TC'),
    ('Tuvalu', 'tuvalu', 'TV'),
    ('Uganda', 'uganda', 'UG'),
    ('Ukraine', 'ukraine', 'UA'),
    ('United Arab Emirates', 'united-arab-emirates', 'AE'),
    ('United Kingdom', 'united-kingdom', 'GB'),
    ('United States', 'united-states', 'US'),
    ('United States Minor Outlying Islands', 'united-states-minor-outlying-islands', 'UM'),
    ('Uruguay', 'uruguay', 'UY'),
    ('Uzbekistan', 'uzbekistan', 'UZ'),
    ('Vanuatu', 'vanuatu', 'VU'),
    ('Vatican City State (Holy See)', 'vatican-city-state-holy-see', 'VA'),
    ('Venezuela', 'venezuela', 'VE'),
    ('Vietnam', 'vietnam', 'VN'),
    ('Virgin Islands (British)', 'virgin-islands-british', 'VG'),
    ('Virgin Islands (US)', 'virgin-islands-us', 'VI'),
    ('Wallis and Futuna Islands', 'wallis-and-futuna-islands', 'WF'),
    ('Western Sahara', 'western-sahara', 'EH'),
    ('Yemen', 'yemen', 'YE'),
    ('Zambia', 'zambia', 'ZM'),
    ('Zimbabwe', 'zimbabwe', 'ZW')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

-- Set currency_code only where the currency exists in the currencies table
-- (spaces_currency_code_fkey constraint). Currencies not in the table
-- get NULL — they can be backfilled when new currencies are seeded.
UPDATE spaces s
SET currency_code = c.code
FROM currencies c
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IS NOT NULL
  AND c.code = (SELECT v.currency FROM (VALUES
    ('AF', 'AFN'),
    ('AX', 'EUR'),
    ('AL', 'ALL'),
    ('DZ', 'DZD'),
    ('AS', 'USD'),
    ('AD', 'EUR'),
    ('AO', 'AOA'),
    ('AI', 'XCD'),
    ('AQ', 'AAD'),
    ('AG', 'XCD'),
    ('AR', 'ARS'),
    ('AM', 'AMD'),
    ('AW', 'AWG'),
    ('AU', 'AUD'),
    ('AT', 'EUR'),
    ('AZ', 'AZN'),
    ('BH', 'BHD'),
    ('BD', 'BDT'),
    ('BB', 'BBD'),
    ('BY', 'BYN'),
    ('BE', 'EUR'),
    ('BZ', 'BZD'),
    ('BJ', 'XOF'),
    ('BM', 'BMD'),
    ('BT', 'BTN'),
    ('BO', 'BOB'),
    ('BQ', 'USD'),
    ('BA', 'BAM'),
    ('BW', 'BWP'),
    ('BV', 'NOK'),
    ('BR', 'BRL'),
    ('IO', 'USD'),
    ('BN', 'BND'),
    ('BG', 'EUR'),
    ('BF', 'XOF'),
    ('BI', 'BIF'),
    ('KH', 'KHR'),
    ('CM', 'XAF'),
    ('CA', 'CAD'),
    ('CV', 'CVE'),
    ('KY', 'KYD'),
    ('CF', 'XAF'),
    ('TD', 'XAF'),
    ('CL', 'CLP'),
    ('CN', 'CNY'),
    ('CX', 'AUD'),
    ('CC', 'AUD'),
    ('CO', 'COP'),
    ('KM', 'KMF'),
    ('CG', 'CDF'),
    ('CK', 'NZD'),
    ('CR', 'CRC'),
    ('HR', 'EUR'),
    ('CU', 'CUP'),
    ('CW', 'ANG'),
    ('CY', 'EUR'),
    ('CZ', 'CZK'),
    ('CD', 'CDF'),
    ('DK', 'DKK'),
    ('DJ', 'DJF'),
    ('DM', 'XCD'),
    ('DO', 'DOP'),
    ('EC', 'USD'),
    ('EG', 'EGP'),
    ('SV', 'USD'),
    ('GQ', 'XAF'),
    ('ER', 'ERN'),
    ('EE', 'EUR'),
    ('SZ', 'SZL'),
    ('ET', 'ETB'),
    ('FK', 'FKP'),
    ('FO', 'DKK'),
    ('FJ', 'FJD'),
    ('FI', 'EUR'),
    ('FR', 'EUR'),
    ('GF', 'EUR'),
    ('PF', 'XPF'),
    ('TF', 'EUR'),
    ('GA', 'XAF'),
    ('GE', 'GEL'),
    ('DE', 'EUR'),
    ('GH', 'GHS'),
    ('GI', 'GIP'),
    ('GR', 'EUR'),
    ('GL', 'DKK'),
    ('GD', 'XCD'),
    ('GP', 'EUR'),
    ('GU', 'USD'),
    ('GT', 'GTQ'),
    ('GG', 'GBP'),
    ('GN', 'GNF'),
    ('GW', 'XOF'),
    ('GY', 'GYD'),
    ('HT', 'HTG'),
    ('HM', 'AUD'),
    ('HN', 'HNL'),
    ('HK', 'HKD'),
    ('HU', 'HUF'),
    ('IS', 'ISK'),
    ('IN', 'INR'),
    ('ID', 'IDR'),
    ('IR', 'IRR'),
    ('IQ', 'IQD'),
    ('IE', 'EUR'),
    ('IL', 'ILS'),
    ('IT', 'EUR'),
    ('CI', 'XOF'),
    ('JM', 'JMD'),
    ('JP', 'JPY'),
    ('JE', 'GBP'),
    ('JO', 'JOD'),
    ('KZ', 'KZT'),
    ('KE', 'KES'),
    ('KI', 'AUD'),
    ('XK', 'EUR'),
    ('KW', 'KWD'),
    ('KG', 'KGS'),
    ('LA', 'LAK'),
    ('LV', 'EUR'),
    ('LB', 'LBP'),
    ('LS', 'LSL'),
    ('LR', 'LRD'),
    ('LY', 'LYD'),
    ('LI', 'CHF'),
    ('LT', 'EUR'),
    ('LU', 'EUR'),
    ('MO', 'MOP'),
    ('MG', 'MGA'),
    ('MW', 'MWK'),
    ('MY', 'MYR'),
    ('MV', 'MVR'),
    ('ML', 'XOF'),
    ('MT', 'EUR'),
    ('IM', 'GBP'),
    ('MH', 'USD'),
    ('MQ', 'EUR'),
    ('MR', 'MRU'),
    ('MU', 'MUR'),
    ('YT', 'EUR'),
    ('MX', 'MXN'),
    ('FM', 'USD'),
    ('MD', 'MDL'),
    ('MC', 'EUR'),
    ('MN', 'MNT'),
    ('ME', 'EUR'),
    ('MS', 'XCD'),
    ('MA', 'MAD'),
    ('MZ', 'MZN'),
    ('MM', 'MMK'),
    ('NA', 'NAD'),
    ('NR', 'AUD'),
    ('NP', 'NPR'),
    ('NL', 'EUR'),
    ('NC', 'XPF'),
    ('NZ', 'NZD'),
    ('NI', 'NIO'),
    ('NE', 'XOF'),
    ('NG', 'NGN'),
    ('NU', 'NZD'),
    ('NF', 'AUD'),
    ('KP', 'KPW'),
    ('MK', 'MKD'),
    ('MP', 'USD'),
    ('NO', 'NOK'),
    ('OM', 'OMR'),
    ('PK', 'PKR'),
    ('PW', 'USD'),
    ('PS', 'ILS'),
    ('PA', 'PAB'),
    ('PG', 'PGK'),
    ('PY', 'PYG'),
    ('PE', 'PEN'),
    ('PH', 'PHP'),
    ('PN', 'NZD'),
    ('PL', 'PLN'),
    ('PT', 'EUR'),
    ('PR', 'USD'),
    ('QA', 'QAR'),
    ('RE', 'EUR'),
    ('RO', 'RON'),
    ('RU', 'RUB'),
    ('RW', 'RWF'),
    ('SH', 'SHP'),
    ('KN', 'XCD'),
    ('LC', 'XCD'),
    ('PM', 'EUR'),
    ('VC', 'XCD'),
    ('BL', 'EUR'),
    ('MF', 'EUR'),
    ('WS', 'WST'),
    ('SM', 'EUR'),
    ('ST', 'STN'),
    ('SA', 'SAR'),
    ('SN', 'XOF'),
    ('RS', 'RSD'),
    ('SC', 'SCR'),
    ('SL', 'SLL'),
    ('SG', 'SGD'),
    ('SX', 'ANG'),
    ('SK', 'EUR'),
    ('SI', 'EUR'),
    ('SB', 'SBD'),
    ('SO', 'SOS'),
    ('ZA', 'ZAR'),
    ('GS', 'GBP'),
    ('KR', 'KRW'),
    ('SS', 'SSP'),
    ('ES', 'EUR'),
    ('LK', 'LKR'),
    ('SD', 'SDG'),
    ('SR', 'SRD'),
    ('SJ', 'NOK'),
    ('SE', 'SEK'),
    ('CH', 'CHF'),
    ('SY', 'SYP'),
    ('TW', 'TWD'),
    ('TJ', 'TJS'),
    ('TZ', 'TZS'),
    ('TH', 'THB'),
    ('BS', 'BSD'),
    ('GM', 'GMD'),
    ('TL', 'USD'),
    ('TG', 'XOF'),
    ('TK', 'NZD'),
    ('TO', 'TOP'),
    ('TT', 'TTD'),
    ('TN', 'TND'),
    ('TR', 'TRY'),
    ('TM', 'TMT'),
    ('TC', 'USD'),
    ('TV', 'AUD'),
    ('UG', 'UGX'),
    ('UA', 'UAH'),
    ('AE', 'AED'),
    ('GB', 'GBP'),
    ('US', 'USD'),
    ('UM', 'USD'),
    ('UY', 'UYU'),
    ('UZ', 'UZS'),
    ('VU', 'VUV'),
    ('VA', 'EUR'),
    ('VE', 'VES'),
    ('VN', 'VND'),
    ('VG', 'USD'),
    ('VI', 'USD'),
    ('WF', 'XPF'),
    ('EH', 'MAD'),
    ('YE', 'YER'),
    ('ZM', 'ZMW'),
    ('ZW', 'ZWL')
) AS v(country_code, currency) WHERE v.country_code = s.geo_code)
  AND (s.currency_code IS NULL OR s.currency_code = '');

-- Backfill geo_code for countries created without it (e.g. Liberia)
UPDATE spaces s
SET geo_code = v.code
FROM (VALUES
    ('afghanistan', 'AF'),
    ('aland-islands', 'AX'),
    ('albania', 'AL'),
    ('algeria', 'DZ'),
    ('american-samoa', 'AS'),
    ('andorra', 'AD'),
    ('angola', 'AO'),
    ('anguilla', 'AI'),
    ('antarctica', 'AQ'),
    ('antigua-and-barbuda', 'AG'),
    ('argentina', 'AR'),
    ('armenia', 'AM'),
    ('aruba', 'AW'),
    ('australia', 'AU'),
    ('austria', 'AT'),
    ('azerbaijan', 'AZ'),
    ('bahrain', 'BH'),
    ('bangladesh', 'BD'),
    ('barbados', 'BB'),
    ('belarus', 'BY'),
    ('belgium', 'BE'),
    ('belize', 'BZ'),
    ('benin', 'BJ'),
    ('bermuda', 'BM'),
    ('bhutan', 'BT'),
    ('bolivia', 'BO'),
    ('bonaire-sint-eustatius-and-saba', 'BQ'),
    ('bosnia-and-herzegovina', 'BA'),
    ('botswana', 'BW'),
    ('bouvet-island', 'BV'),
    ('brazil', 'BR'),
    ('british-indian-ocean-territory', 'IO'),
    ('brunei', 'BN'),
    ('bulgaria', 'BG'),
    ('burkina-faso', 'BF'),
    ('burundi', 'BI'),
    ('cambodia', 'KH'),
    ('cameroon', 'CM'),
    ('canada', 'CA'),
    ('cape-verde', 'CV'),
    ('cayman-islands', 'KY'),
    ('central-african-republic', 'CF'),
    ('chad', 'TD'),
    ('chile', 'CL'),
    ('china', 'CN'),
    ('christmas-island', 'CX'),
    ('cocos-keeling-islands', 'CC'),
    ('colombia', 'CO'),
    ('comoros', 'KM'),
    ('congo', 'CG'),
    ('cook-islands', 'CK'),
    ('costa-rica', 'CR'),
    ('croatia', 'HR'),
    ('cuba', 'CU'),
    ('curaçao', 'CW'),
    ('cyprus', 'CY'),
    ('czech-republic', 'CZ'),
    ('democratic-republic-of-the-congo', 'CD'),
    ('denmark', 'DK'),
    ('djibouti', 'DJ'),
    ('dominica', 'DM'),
    ('dominican-republic', 'DO'),
    ('ecuador', 'EC'),
    ('egypt', 'EG'),
    ('el-salvador', 'SV'),
    ('equatorial-guinea', 'GQ'),
    ('eritrea', 'ER'),
    ('estonia', 'EE'),
    ('eswatini', 'SZ'),
    ('ethiopia', 'ET'),
    ('falkland-islands', 'FK'),
    ('faroe-islands', 'FO'),
    ('fiji-islands', 'FJ'),
    ('finland', 'FI'),
    ('france', 'FR'),
    ('french-guiana', 'GF'),
    ('french-polynesia', 'PF'),
    ('french-southern-territories', 'TF'),
    ('gabon', 'GA'),
    ('georgia', 'GE'),
    ('germany', 'DE'),
    ('ghana', 'GH'),
    ('gibraltar', 'GI'),
    ('greece', 'GR'),
    ('greenland', 'GL'),
    ('grenada', 'GD'),
    ('guadeloupe', 'GP'),
    ('guam', 'GU'),
    ('guatemala', 'GT'),
    ('guernsey', 'GG'),
    ('guinea', 'GN'),
    ('guinea-bissau', 'GW'),
    ('guyana', 'GY'),
    ('haiti', 'HT'),
    ('heard-island-and-mcdonald-islands', 'HM'),
    ('honduras', 'HN'),
    ('hong-kong-sar', 'HK'),
    ('hungary', 'HU'),
    ('iceland', 'IS'),
    ('india', 'IN'),
    ('indonesia', 'ID'),
    ('iran', 'IR'),
    ('iraq', 'IQ'),
    ('ireland', 'IE'),
    ('israel', 'IL'),
    ('italy', 'IT'),
    ('ivory-coast', 'CI'),
    ('jamaica', 'JM'),
    ('japan', 'JP'),
    ('jersey', 'JE'),
    ('jordan', 'JO'),
    ('kazakhstan', 'KZ'),
    ('kenya', 'KE'),
    ('kiribati', 'KI'),
    ('kosovo', 'XK'),
    ('kuwait', 'KW'),
    ('kyrgyzstan', 'KG'),
    ('laos', 'LA'),
    ('latvia', 'LV'),
    ('lebanon', 'LB'),
    ('lesotho', 'LS'),
    ('liberia', 'LR'),
    ('libya', 'LY'),
    ('liechtenstein', 'LI'),
    ('lithuania', 'LT'),
    ('luxembourg', 'LU'),
    ('macau-sar', 'MO'),
    ('madagascar', 'MG'),
    ('malawi', 'MW'),
    ('malaysia', 'MY'),
    ('maldives', 'MV'),
    ('mali', 'ML'),
    ('malta', 'MT'),
    ('man-isle-of', 'IM'),
    ('marshall-islands', 'MH'),
    ('martinique', 'MQ'),
    ('mauritania', 'MR'),
    ('mauritius', 'MU'),
    ('mayotte', 'YT'),
    ('mexico', 'MX'),
    ('micronesia', 'FM'),
    ('moldova', 'MD'),
    ('monaco', 'MC'),
    ('mongolia', 'MN'),
    ('montenegro', 'ME'),
    ('montserrat', 'MS'),
    ('morocco', 'MA'),
    ('mozambique', 'MZ'),
    ('myanmar', 'MM'),
    ('namibia', 'NA'),
    ('nauru', 'NR'),
    ('nepal', 'NP'),
    ('netherlands', 'NL'),
    ('new-caledonia', 'NC'),
    ('new-zealand', 'NZ'),
    ('nicaragua', 'NI'),
    ('niger', 'NE'),
    ('nigeria', 'NG'),
    ('niue', 'NU'),
    ('norfolk-island', 'NF'),
    ('north-korea', 'KP'),
    ('north-macedonia', 'MK'),
    ('northern-mariana-islands', 'MP'),
    ('norway', 'NO'),
    ('oman', 'OM'),
    ('pakistan', 'PK'),
    ('palau', 'PW'),
    ('palestinian-territory-occupied', 'PS'),
    ('panama', 'PA'),
    ('papua-new-guinea', 'PG'),
    ('paraguay', 'PY'),
    ('peru', 'PE'),
    ('philippines', 'PH'),
    ('pitcairn-island', 'PN'),
    ('poland', 'PL'),
    ('portugal', 'PT'),
    ('puerto-rico', 'PR'),
    ('qatar', 'QA'),
    ('reunion', 'RE'),
    ('romania', 'RO'),
    ('russia', 'RU'),
    ('rwanda', 'RW'),
    ('saint-helena', 'SH'),
    ('saint-kitts-and-nevis', 'KN'),
    ('saint-lucia', 'LC'),
    ('saint-pierre-and-miquelon', 'PM'),
    ('saint-vincent-and-the-grenadines', 'VC'),
    ('saint-barthelemy', 'BL'),
    ('saint-martin-french-part', 'MF'),
    ('samoa', 'WS'),
    ('san-marino', 'SM'),
    ('sao-tome-and-principe', 'ST'),
    ('saudi-arabia', 'SA'),
    ('senegal', 'SN'),
    ('serbia', 'RS'),
    ('seychelles', 'SC'),
    ('sierra-leone', 'SL'),
    ('singapore', 'SG'),
    ('sint-maarten-dutch-part', 'SX'),
    ('slovakia', 'SK'),
    ('slovenia', 'SI'),
    ('solomon-islands', 'SB'),
    ('somalia', 'SO'),
    ('south-africa', 'ZA'),
    ('south-georgia', 'GS'),
    ('south-korea', 'KR'),
    ('south-sudan', 'SS'),
    ('spain', 'ES'),
    ('sri-lanka', 'LK'),
    ('sudan', 'SD'),
    ('suriname', 'SR'),
    ('svalbard-and-jan-mayen-islands', 'SJ'),
    ('sweden', 'SE'),
    ('switzerland', 'CH'),
    ('syria', 'SY'),
    ('taiwan', 'TW'),
    ('tajikistan', 'TJ'),
    ('tanzania', 'TZ'),
    ('thailand', 'TH'),
    ('the-bahamas', 'BS'),
    ('the-gambia', 'GM'),
    ('timor-leste', 'TL'),
    ('togo', 'TG'),
    ('tokelau', 'TK'),
    ('tonga', 'TO'),
    ('trinidad-and-tobago', 'TT'),
    ('tunisia', 'TN'),
    ('turkey', 'TR'),
    ('turkmenistan', 'TM'),
    ('turks-and-caicos-islands', 'TC'),
    ('tuvalu', 'TV'),
    ('uganda', 'UG'),
    ('ukraine', 'UA'),
    ('united-arab-emirates', 'AE'),
    ('united-kingdom', 'GB'),
    ('united-states', 'US'),
    ('united-states-minor-outlying-islands', 'UM'),
    ('uruguay', 'UY'),
    ('uzbekistan', 'UZ'),
    ('vanuatu', 'VU'),
    ('vatican-city-state-holy-see', 'VA'),
    ('venezuela', 'VE'),
    ('vietnam', 'VN'),
    ('virgin-islands-british', 'VG'),
    ('virgin-islands-us', 'VI'),
    ('wallis-and-futuna-islands', 'WF'),
    ('western-sahara', 'EH'),
    ('yemen', 'YE'),
    ('zambia', 'ZM'),
    ('zimbabwe', 'ZW')
) AS v(slug, code)
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001' AND s.level = 'country' AND s.slug = v.slug
  AND (s.geo_code IS NULL OR s.geo_code = '');

-- ============================================================
-- States/Provinces (level=state) — all ~5,300 from the dataset
-- ============================================================
-- Afghanistan (AF) — 34 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'af-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Badakhshan', 'badakhshan', 'BDS'),
    ('Badghis', 'badghis', 'BDG'),
    ('Baghlan', 'baghlan', 'BGL'),
    ('Balkh', 'balkh', 'BAL'),
    ('Bamyan', 'bamyan', 'BAM'),
    ('Daykundi', 'daykundi', 'DAY'),
    ('Farah', 'farah', 'FRA'),
    ('Faryab', 'faryab', 'FYB'),
    ('Ghazni', 'ghazni', 'GHA'),
    ('Ghōr', 'ghōr', 'GHO'),
    ('Helmand', 'helmand', 'HEL'),
    ('Herat', 'herat', 'HER'),
    ('Jowzjan', 'jowzjan', 'JOW'),
    ('Kabul', 'kabul', 'KAB'),
    ('Kandahar', 'kandahar', 'KAN'),
    ('Kapisa', 'kapisa', 'KAP'),
    ('Khost', 'khost', 'KHO'),
    ('Kunar', 'kunar', 'KNR'),
    ('Kunduz Province', 'kunduz-province', 'KDZ'),
    ('Laghman', 'laghman', 'LAG'),
    ('Logar', 'logar', 'LOG'),
    ('Nangarhar', 'nangarhar', 'NAN'),
    ('Nimruz', 'nimruz', 'NIM'),
    ('Nuristan', 'nuristan', 'NUR'),
    ('Paktia', 'paktia', 'PIA'),
    ('Paktika', 'paktika', 'PKA'),
    ('Panjshir', 'panjshir', 'PAN'),
    ('Parwan', 'parwan', 'PAR'),
    ('Samangan', 'samangan', 'SAM'),
    ('Sar-e Pol', 'sar-e-pol', 'SAR'),
    ('Takhar', 'takhar', 'TAK'),
    ('Urozgan', 'urozgan', 'URU'),
    ('Wardak', 'wardak', 'WAR'),
    ('Zabul', 'zabul', 'ZAB')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'afghanistan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'af-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Aland Islands (AX) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ax-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Brändö', 'brändö', '12'),
    ('Eckerö', 'eckerö', '08'),
    ('Finström', 'finström', '03'),
    ('Föglö', 'föglö', '09'),
    ('Geta', 'geta', '10'),
    ('Hammarland', 'hammarland', '06'),
    ('Jomala', 'jomala', '02'),
    ('Kökar', 'kökar', '15'),
    ('Kumlinge', 'kumlinge', '14'),
    ('Lemland', 'lemland', '04'),
    ('Lumparland', 'lumparland', '13'),
    ('Mariehamn', 'mariehamn', '01'),
    ('Saltvik', 'saltvik', '05'),
    ('Sottunga', 'sottunga', '16'),
    ('Sund', 'sund', '07'),
    ('Vårdö', 'vårdö', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'aland-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ax-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Albania (AL) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'al-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Berat', 'berat', '01'),
    ('Dibër', 'dibër', '09'),
    ('Durrës', 'durrës', '02'),
    ('Elbasan', 'elbasan', '03'),
    ('Fier', 'fier', '04'),
    ('Gjirokastër', 'gjirokastër', '05'),
    ('Korçë', 'korçë', '06'),
    ('Kukës', 'kukës', '07'),
    ('Lezhë', 'lezhë', '08'),
    ('Shkodër', 'shkodër', '10'),
    ('Tirana', 'tirana', '11'),
    ('Vlorë', 'vlorë', '12')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'albania' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'al-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Algeria (DZ) — 58 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'dz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adrar', 'adrar', '01'),
    ('Aïn Defla', 'aïn-defla', '44'),
    ('Aïn Témouchent', 'aïn-témouchent', '46'),
    ('Algiers', 'algiers', '16'),
    ('Annaba', 'annaba', '23'),
    ('Batna', 'batna', '05'),
    ('Béchar', 'béchar', '08'),
    ('Béjaïa', 'béjaïa', '06'),
    ('Béni Abbès', 'béni-abbès', '53'),
    ('Biskra', 'biskra', '07'),
    ('Blida', 'blida', '09'),
    ('Bordj Baji Mokhtar', 'bordj-baji-mokhtar', '52'),
    ('Bordj Bou Arréridj', 'bordj-bou-arréridj', '34'),
    ('Bouïra', 'bouïra', '10'),
    ('Boumerdès', 'boumerdès', '35'),
    ('Chlef', 'chlef', '02'),
    ('Constantine', 'constantine', '25'),
    ('Djanet', 'djanet', '56'),
    ('Djelfa', 'djelfa', '17'),
    ('El Bayadh', 'el-bayadh', '32'),
    ('El M''ghair', 'el-mghair', '49'),
    ('El Menia', 'el-menia', '50'),
    ('El Oued', 'el-oued', '39'),
    ('El Tarf', 'el-tarf', '36'),
    ('Ghardaïa', 'ghardaïa', '47'),
    ('Guelma', 'guelma', '24'),
    ('Illizi', 'illizi', '33'),
    ('In Guezzam', 'in-guezzam', '58'),
    ('In Salah', 'in-salah', '57'),
    ('Jijel', 'jijel', '18'),
    ('Khenchela', 'khenchela', '40'),
    ('Laghouat', 'laghouat', '03'),
    ('M''Sila', 'msila', '28'),
    ('Mascara', 'mascara', '29'),
    ('Médéa', 'médéa', '26'),
    ('Mila', 'mila', '43'),
    ('Mostaganem', 'mostaganem', '27'),
    ('Naama', 'naama', '45'),
    ('Oran', 'oran', '31'),
    ('Ouargla', 'ouargla', '30'),
    ('Ouled Djellal', 'ouled-djellal', '51'),
    ('Oum El Bouaghi', 'oum-el-bouaghi', '04'),
    ('Relizane', 'relizane', '48'),
    ('Saïda', 'saïda', '20'),
    ('Sétif', 'sétif', '19'),
    ('Sidi Bel Abbès', 'sidi-bel-abbès', '22'),
    ('Skikda', 'skikda', '21'),
    ('Souk Ahras', 'souk-ahras', '41'),
    ('Tamanghasset', 'tamanghasset', '11'),
    ('Tébessa', 'tébessa', '12'),
    ('Tiaret', 'tiaret', '14'),
    ('Timimoun', 'timimoun', '54'),
    ('Tindouf', 'tindouf', '37'),
    ('Tipasa', 'tipasa', '42'),
    ('Tissemsilt', 'tissemsilt', '38'),
    ('Tizi Ouzou', 'tizi-ouzou', '15'),
    ('Tlemcen', 'tlemcen', '13'),
    ('Touggourt', 'touggourt', '55')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'algeria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'dz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- American Samoa (AS) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'as-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Eastern', 'eastern', '02'),
    ('Manuʻa', 'manuʻa', '03'),
    ('Rose', 'rose', '05'),
    ('Swains', 'swains', '04'),
    ('Western', 'western', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'american-samoa' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'as-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Andorra (AD) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ad-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Andorra la Vella', 'andorra-la-vella', '07'),
    ('Canillo', 'canillo', '02'),
    ('Encamp', 'encamp', '03'),
    ('Escaldes-Engordany', 'escaldes-engordany', '08'),
    ('La Massana', 'la-massana', '04'),
    ('Ordino', 'ordino', '05'),
    ('Sant Julià de Lòria', 'sant-julià-de-lòria', '06')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'andorra' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ad-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Angola (AO) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ao-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bengo', 'bengo', 'BGO'),
    ('Benguela', 'benguela', 'BGU'),
    ('Bié', 'bié', 'BIE'),
    ('Cabinda', 'cabinda', 'CAB'),
    ('Cuando Cubango', 'cuando-cubango', 'CCU'),
    ('Cuanza', 'cuanza', 'CUS'),
    ('Cuanza Norte', 'cuanza-norte', 'CNO'),
    ('Cunene', 'cunene', 'CNN'),
    ('Huambo', 'huambo', 'HUA'),
    ('Huíla', 'huíla', 'HUI'),
    ('Luanda', 'luanda', 'LUA'),
    ('Lunda Norte', 'lunda-norte', 'LNO'),
    ('Lunda Sul', 'lunda-sul', 'LSU'),
    ('Malanje', 'malanje', 'MAL'),
    ('Moxico', 'moxico', 'MOX'),
    ('Namibe', 'namibe', 'NAM'),
    ('Uíge', 'uíge', 'UIG'),
    ('Zaire', 'zaire', 'ZAI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'angola' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ao-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Anguilla (AI) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ai-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Blowing Point', 'blowing-point', '01'),
    ('East End', 'east-end', '02'),
    ('George Hill', 'george-hill', '03'),
    ('Island Harbour', 'island-harbour', '04'),
    ('North Hill', 'north-hill', '05'),
    ('North Side', 'north-side', '06'),
    ('Sandy Ground', 'sandy-ground', '07'),
    ('Sandy Hill', 'sandy-hill', '08'),
    ('South Hill', 'south-hill', '09'),
    ('Stoney Ground', 'stoney-ground', '10'),
    ('The Farrington', 'the-farrington', '11'),
    ('The Quarter', 'the-quarter', '12'),
    ('The Valley', 'the-valley', '13'),
    ('West End', 'west-end', '14')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'anguilla' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ai-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Antigua and Barbuda (AG) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ag-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Barbuda', 'barbuda', '10'),
    ('Redonda', 'redonda', '11'),
    ('Saint George', 'saint-george', '03'),
    ('Saint John', 'saint-john', '04'),
    ('Saint Mary', 'saint-mary', '05'),
    ('Saint Paul', 'saint-paul', '06'),
    ('Saint Peter', 'saint-peter', '07'),
    ('Saint Philip', 'saint-philip', '08')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'antigua-and-barbuda' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ag-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Argentina (AR) — 24 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ar-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Autonomous City of Buenos Aires', 'autonomous-city-of-buenos-aires', 'C'),
    ('Buenos Aires', 'buenos-aires', 'B'),
    ('Catamarca', 'catamarca', 'K'),
    ('Chaco', 'chaco', 'H'),
    ('Chubut', 'chubut', 'U'),
    ('Córdoba', 'córdoba', 'X'),
    ('Corrientes', 'corrientes', 'W'),
    ('Entre Ríos', 'entre-ríos', 'E'),
    ('Formosa', 'formosa', 'P'),
    ('Jujuy', 'jujuy', 'Y'),
    ('La Pampa', 'la-pampa', 'L'),
    ('La Rioja', 'la-rioja', 'F'),
    ('Mendoza', 'mendoza', 'M'),
    ('Misiones', 'misiones', 'N'),
    ('Neuquén', 'neuquén', 'Q'),
    ('Río Negro', 'río-negro', 'R'),
    ('Salta', 'salta', 'A'),
    ('San Juan', 'san-juan', 'J'),
    ('San Luis', 'san-luis', 'D'),
    ('Santa Cruz', 'santa-cruz', 'Z'),
    ('Santa Fe', 'santa-fe', 'S'),
    ('Santiago del Estero', 'santiago-del-estero', 'G'),
    ('Tierra del Fuego', 'tierra-del-fuego', 'V'),
    ('Tucumán', 'tucumán', 'T')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'argentina' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ar-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Armenia (AM) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'am-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aragatsotn', 'aragatsotn', 'AG'),
    ('Ararat', 'ararat', 'AR'),
    ('Armavir', 'armavir', 'AV'),
    ('Gegharkunik', 'gegharkunik', 'GR'),
    ('Kotayk', 'kotayk', 'KT'),
    ('Lori', 'lori', 'LO'),
    ('Shirak', 'shirak', 'SH'),
    ('Syunik', 'syunik', 'SU'),
    ('Tavush', 'tavush', 'TV'),
    ('Vayots Dzor', 'vayots-dzor', 'VD'),
    ('Yerevan', 'yerevan', 'ER')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'armenia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'am-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Aruba (AW) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'aw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Noord', 'noord', '01'),
    ('Oranjestad', 'oranjestad', '09'),
    ('Oranjestad East', 'oranjestad-east', '03'),
    ('Oranjestad West', 'oranjestad-west', '02'),
    ('Paradera', 'paradera', '04'),
    ('San Nicolaas Noord', 'san-nicolaas-noord', '05'),
    ('San Nicolaas Zuid', 'san-nicolaas-zuid', '06'),
    ('Santa Cruz', 'santa-cruz', '07'),
    ('Savaneta', 'savaneta', '08')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'aruba' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'aw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Australia (AU) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'au-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Australian Capital Territory', 'australian-capital-territory', 'ACT'),
    ('New South Wales', 'new-south-wales', 'NSW'),
    ('Northern Territory', 'northern-territory', 'NT'),
    ('Queensland', 'queensland', 'QLD'),
    ('South Australia', 'south-australia', 'SA'),
    ('Tasmania', 'tasmania', 'TAS'),
    ('Victoria', 'victoria', 'VIC'),
    ('Western Australia', 'western-australia', 'WA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'australia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'au-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Austria (AT) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'at-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Burgenland', 'burgenland', '1'),
    ('Carinthia', 'carinthia', '2'),
    ('Lower Austria', 'lower-austria', '3'),
    ('Salzburg', 'salzburg', '5'),
    ('Styria', 'styria', '6'),
    ('Tyrol', 'tyrol', '7'),
    ('Upper Austria', 'upper-austria', '4'),
    ('Vienna', 'vienna', '9'),
    ('Vorarlberg', 'vorarlberg', '8')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'austria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'at-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Azerbaijan (AZ) — 78 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'az-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Absheron', 'absheron', 'ABS'),
    ('Agdam', 'agdam', 'AGM'),
    ('Agdash', 'agdash', 'AGS'),
    ('Aghjabadi', 'aghjabadi', 'AGC'),
    ('Agstafa', 'agstafa', 'AGA'),
    ('Agsu', 'agsu', 'AGU'),
    ('Astara', 'astara', 'AST'),
    ('Babek', 'babek', 'BAB'),
    ('Baku', 'baku', 'BA'),
    ('Balakan', 'balakan', 'BAL'),
    ('Barda', 'barda', 'BAR'),
    ('Beylagan', 'beylagan', 'BEY'),
    ('Bilasuvar', 'bilasuvar', 'BIL'),
    ('Dashkasan', 'dashkasan', 'DAS'),
    ('Fizuli', 'fizuli', 'FUZ'),
    ('Ganja', 'ganja', 'GA'),
    ('Gədəbəy', 'gədəbəy', 'GAD'),
    ('Gobustan', 'gobustan', 'QOB'),
    ('Goranboy', 'goranboy', 'GOR'),
    ('Goychay', 'goychay', 'GOY'),
    ('Goygol', 'goygol', 'GYG'),
    ('Hajigabul', 'hajigabul', 'HAC'),
    ('Imishli', 'imishli', 'IMI'),
    ('Ismailli', 'ismailli', 'ISM'),
    ('Jabrayil', 'jabrayil', 'CAB'),
    ('Jalilabad', 'jalilabad', 'CAL'),
    ('Julfa', 'julfa', 'CUL'),
    ('Kalbajar', 'kalbajar', 'KAL'),
    ('Kangarli', 'kangarli', 'KAN'),
    ('Khachmaz', 'khachmaz', 'XAC'),
    ('Khankendi', 'khankendi', 'XA'),
    ('Khizi', 'khizi', 'XIZ'),
    ('Khojali', 'khojali', 'XCI'),
    ('Kurdamir', 'kurdamir', 'KUR'),
    ('Lachin', 'lachin', 'LAC'),
    ('Lankaran', 'lankaran', 'LAN'),
    ('Lankaran', 'lankaran', 'LA'),
    ('Lerik', 'lerik', 'LER'),
    ('Martuni', 'martuni', 'XVD'),
    ('Masally', 'masally', 'MAS'),
    ('Mingachevir', 'mingachevir', 'MI'),
    ('Naftalan', 'naftalan', 'NA'),
    ('Nakhchivan', 'nakhchivan', 'NV'),
    ('Nakhchivan', 'nakhchivan', 'NX'),
    ('Neftchala', 'neftchala', 'NEF'),
    ('Oghuz', 'oghuz', 'OGU'),
    ('Ordubad', 'ordubad', 'ORD'),
    ('Qabala', 'qabala', 'QAB'),
    ('Qakh', 'qakh', 'QAX'),
    ('Qazakh', 'qazakh', 'QAZ'),
    ('Quba', 'quba', 'QBA'),
    ('Qubadli', 'qubadli', 'QBI'),
    ('Qusar', 'qusar', 'QUS'),
    ('Saatly', 'saatly', 'SAT'),
    ('Sabirabad', 'sabirabad', 'SAB'),
    ('Sadarak', 'sadarak', 'SAD'),
    ('Salyan', 'salyan', 'SAL'),
    ('Samukh', 'samukh', 'SMX'),
    ('Shabran', 'shabran', 'SBN'),
    ('Shahbuz', 'shahbuz', 'SAH'),
    ('Shaki', 'shaki', 'SA'),
    ('Shaki', 'shaki', 'SAK'),
    ('Shamakhi', 'shamakhi', 'SMI'),
    ('Shamkir', 'shamkir', 'SKR'),
    ('Sharur', 'sharur', 'SAR'),
    ('Shirvan', 'shirvan', 'SR'),
    ('Shusha', 'shusha', 'SUS'),
    ('Siazan', 'siazan', 'SIY'),
    ('Sumqayit', 'sumqayit', 'SM'),
    ('Tartar', 'tartar', 'TAR'),
    ('Tovuz', 'tovuz', 'TOV'),
    ('Ujar', 'ujar', 'UCA'),
    ('Yardymli', 'yardymli', 'YAR'),
    ('Yevlakh', 'yevlakh', 'YEV'),
    ('Yevlakh', 'yevlakh', 'YE'),
    ('Zangilan', 'zangilan', 'ZAN'),
    ('Zaqatala', 'zaqatala', 'ZAQ'),
    ('Zardab', 'zardab', 'ZAR')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'azerbaijan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'az-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bahrain (BH) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bh-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Capital', 'capital', '13'),
    ('Muharraq', 'muharraq', '15'),
    ('Northern', 'northern', '17'),
    ('Southern', 'southern', '14')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bahrain' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bangladesh (BD) — 72 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bd-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bagerhat', 'bagerhat', '05'),
    ('Bandarban', 'bandarban', '01'),
    ('Barguna', 'barguna', '02'),
    ('Barisal ', 'barisal', 'A'),
    ('Barishal', 'barishal', '06'),
    ('Bhola', 'bhola', '07'),
    ('Bogura', 'bogura', '03'),
    ('Brahmanbaria', 'brahmanbaria', '04'),
    ('Chandpur', 'chandpur', '09'),
    ('Chapai Nawabganj', 'chapai-nawabganj', '45'),
    ('Chattogram', 'chattogram', '10'),
    ('Chittagong ', 'chittagong', 'B'),
    ('Chuadanga', 'chuadanga', '12'),
    ('Cox''s Bazar', 'coxs-bazar', '11'),
    ('Cumilla', 'cumilla', '08'),
    ('Dhaka', 'dhaka', '13'),
    ('Dhaka ', 'dhaka', 'C'),
    ('Dinajpur', 'dinajpur', '14'),
    ('Faridpur', 'faridpur', '15'),
    ('Feni', 'feni', '16'),
    ('Gaibandha', 'gaibandha', '19'),
    ('Gazipur', 'gazipur', '18'),
    ('Gopalganj', 'gopalganj', '17'),
    ('Habiganj', 'habiganj', '20'),
    ('Jamalpur', 'jamalpur', '21'),
    ('Jashore', 'jashore', '22'),
    ('Jhalakathi', 'jhalakathi', '25'),
    ('Jhenaidah', 'jhenaidah', '23'),
    ('Joypurhat', 'joypurhat', '24'),
    ('Khagrachhari', 'khagrachhari', '29'),
    ('Khulna', 'khulna', '27'),
    ('Khulna ', 'khulna', 'D'),
    ('Kishoreganj', 'kishoreganj', '26'),
    ('Kurigram', 'kurigram', '28'),
    ('Kushtia', 'kushtia', '30'),
    ('Lakshmipur', 'lakshmipur', '31'),
    ('Lalmonirhat', 'lalmonirhat', '32'),
    ('Madaripur', 'madaripur', '36'),
    ('Magura', 'magura', '37'),
    ('Manikganj', 'manikganj', '33'),
    ('Meherpur', 'meherpur', '39'),
    ('Moulvibazar', 'moulvibazar', '38'),
    ('Munshiganj', 'munshiganj', '35'),
    ('Mymensingh', 'mymensingh', '34'),
    ('Mymensingh ', 'mymensingh', 'H'),
    ('Naogaon', 'naogaon', '48'),
    ('Narail', 'narail', '43'),
    ('Narayanganj', 'narayanganj', '40'),
    ('Narsingdi', 'narsingdi', '42'),
    ('Natore', 'natore', '44'),
    ('Netrakona', 'netrakona', '41'),
    ('Nilphamari', 'nilphamari', '46'),
    ('Noakhali', 'noakhali', '47'),
    ('Pabna', 'pabna', '49'),
    ('Panchagarh', 'panchagarh', '52'),
    ('Patuakhali', 'patuakhali', '51'),
    ('Pirojpur', 'pirojpur', '50'),
    ('Rajbari', 'rajbari', '53'),
    ('Rajshahi', 'rajshahi', '54'),
    ('Rajshahi ', 'rajshahi', 'E'),
    ('Rangamati', 'rangamati', '56'),
    ('Rangpur ', 'rangpur', 'F'),
    ('Rangpur', 'rangpur', '55'),
    ('Satkhira', 'satkhira', '58'),
    ('Shariatpur', 'shariatpur', '62'),
    ('Sherpur', 'sherpur', '57'),
    ('Sirajganj', 'sirajganj', '59'),
    ('Sunamganj', 'sunamganj', '61'),
    ('Sylhet ', 'sylhet', 'G'),
    ('Sylhet', 'sylhet', '60'),
    ('Tangail', 'tangail', '63'),
    ('Thakurgaon', 'thakurgaon', '64')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bangladesh' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bd-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Barbados (BB) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bb-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Christ Church', 'christ-church', '01'),
    ('Saint Andrew', 'saint-andrew', '02'),
    ('Saint George', 'saint-george', '03'),
    ('Saint James', 'saint-james', '04'),
    ('Saint John', 'saint-john', '05'),
    ('Saint Joseph', 'saint-joseph', '06'),
    ('Saint Lucy', 'saint-lucy', '07'),
    ('Saint Michael', 'saint-michael', '08'),
    ('Saint Peter', 'saint-peter', '09'),
    ('Saint Philip', 'saint-philip', '10'),
    ('Saint Thomas', 'saint-thomas', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'barbados' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bb-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Belarus (BY) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'by-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Brest', 'brest', 'BR'),
    ('Gomel', 'gomel', 'HO'),
    ('Grodno', 'grodno', 'HR'),
    ('Minsk', 'minsk', 'MI'),
    ('Minsk', 'minsk', 'HM'),
    ('Mogilev', 'mogilev', 'MA'),
    ('Vitebsk', 'vitebsk', 'VI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'belarus' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'by-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Belgium (BE) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'be-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Antwerp', 'antwerp', 'VAN'),
    ('Brussels-Capital ', 'brussels-capital', 'BRU'),
    ('East Flanders', 'east-flanders', 'VOV'),
    ('Flanders', 'flanders', 'VLG'),
    ('Flemish Brabant', 'flemish-brabant', 'VBR'),
    ('Hainaut', 'hainaut', 'WHT'),
    ('Liège', 'liège', 'WLG'),
    ('Limburg', 'limburg', 'VLI'),
    ('Luxembourg', 'luxembourg', 'WLX'),
    ('Namur', 'namur', 'WNA'),
    ('Wallonia', 'wallonia', 'WAL'),
    ('Walloon Brabant', 'walloon-brabant', 'WBR'),
    ('West Flanders', 'west-flanders', 'VWV')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'belgium' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'be-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Belize (BZ) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Belize', 'belize', 'BZ'),
    ('Cayo', 'cayo', 'CY'),
    ('Corozal', 'corozal', 'CZL'),
    ('Orange Walk', 'orange-walk', 'OW'),
    ('Stann Creek', 'stann-creek', 'SC'),
    ('Toledo', 'toledo', 'TOL')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'belize' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Benin (BJ) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bj-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alibori', 'alibori', 'AL'),
    ('Atakora', 'atakora', 'AK'),
    ('Atlantique', 'atlantique', 'AQ'),
    ('Borgou', 'borgou', 'BO'),
    ('Collines', 'collines', 'CO'),
    ('Donga', 'donga', 'DO'),
    ('Kouffo', 'kouffo', 'KO'),
    ('Littoral', 'littoral', 'LI'),
    ('Mono', 'mono', 'MO'),
    ('Ouémé', 'ouémé', 'OU'),
    ('Plateau', 'plateau', 'PL'),
    ('Zou', 'zou', 'ZO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'benin' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bj-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bermuda (BM) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Devonshire', 'devonshire', 'DEV'),
    ('Hamilton', 'hamilton', 'HA'),
    ('Paget', 'paget', 'PAG'),
    ('Pembroke', 'pembroke', 'PEM'),
    ('Saint George''s', 'saint-georges', 'SGE'),
    ('Sandys', 'sandys', 'SAN'),
    ('Smith''s', 'smiths', 'SMI'),
    ('Southampton', 'southampton', 'SOU'),
    ('Warwick', 'warwick', 'WAR')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bermuda' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bhutan (BT) — 20 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bumthang ', 'bumthang', '33'),
    ('Chukha ', 'chukha', '12'),
    ('Dagana ', 'dagana', '22'),
    ('Gasa ', 'gasa', 'GA'),
    ('Haa ', 'haa', '13'),
    ('Lhuntse ', 'lhuntse', '44'),
    ('Mongar ', 'mongar', '42'),
    ('Paro ', 'paro', '11'),
    ('Pemagatshel ', 'pemagatshel', '43'),
    ('Punakha ', 'punakha', '23'),
    ('Samdrup Jongkhar ', 'samdrup-jongkhar', '45'),
    ('Samtse ', 'samtse', '14'),
    ('Sarpang ', 'sarpang', '31'),
    ('Thimphu ', 'thimphu', '15'),
    ('Trashi Yangtse	', 'trashi-yangtse', 'TY'),
    ('Trashigang ', 'trashigang', '41'),
    ('Trongsa ', 'trongsa', '32'),
    ('Tsirang ', 'tsirang', '21'),
    ('Wangdue Phodrang ', 'wangdue-phodrang', '24'),
    ('Zhemgang ', 'zhemgang', '34')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bhutan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bolivia (BO) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bo-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Beni', 'beni', 'B'),
    ('Chuquisaca', 'chuquisaca', 'H'),
    ('Cochabamba', 'cochabamba', 'C'),
    ('La Paz', 'la-paz', 'L'),
    ('Oruro', 'oruro', 'O'),
    ('Pando', 'pando', 'N'),
    ('Potosí', 'potosí', 'P'),
    ('Santa Cruz', 'santa-cruz', 'S'),
    ('Tarija', 'tarija', 'T')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bolivia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bo-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bonaire, Sint Eustatius and Saba (BQ) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bq-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bonaire', 'bonaire', 'BQ1'),
    ('Saba', 'saba', 'BQ2'),
    ('Sint Eustatius', 'sint-eustatius', 'BQ3')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bonaire-sint-eustatius-and-saba' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bq-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bosnia and Herzegovina (BA) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ba-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Brčko', 'brčko', 'BRC'),
    ('Federation of Bosnia and Herzegovina', 'federation-of-bosnia-and-herzegovina', 'BIH'),
    ('Republika Srpska', 'republika-srpska', 'SRP')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bosnia-and-herzegovina' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ba-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Botswana (BW) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central', 'central', 'CE'),
    ('Chobe', 'chobe', 'CH'),
    ('Francistown', 'francistown', 'FR'),
    ('Gaborone', 'gaborone', 'GA'),
    ('Ghanzi', 'ghanzi', 'GH'),
    ('Jwaneng', 'jwaneng', 'JW'),
    ('Kgalagadi', 'kgalagadi', 'KG'),
    ('Kgatleng', 'kgatleng', 'KL'),
    ('Kweneng', 'kweneng', 'KW'),
    ('Lobatse', 'lobatse', 'LO'),
    ('North-East', 'north-east', 'NE'),
    ('North-West', 'north-west', 'NW'),
    ('Selibe Phikwe', 'selibe-phikwe', 'SP'),
    ('South-East', 'south-east', 'SE'),
    ('Southern', 'southern', 'SO'),
    ('Sowa Town', 'sowa-town', 'ST')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'botswana' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Brazil (BR) — 27 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'br-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Acre', 'acre', 'AC'),
    ('Alagoas', 'alagoas', 'AL'),
    ('Amapá', 'amapá', 'AP'),
    ('Amazonas', 'amazonas', 'AM'),
    ('Bahia', 'bahia', 'BA'),
    ('Ceará', 'ceará', 'CE'),
    ('Distrito Federal', 'distrito-federal', 'DF'),
    ('Espírito Santo', 'espírito-santo', 'ES'),
    ('Goiás', 'goiás', 'GO'),
    ('Maranhão', 'maranhão', 'MA'),
    ('Mato Grosso', 'mato-grosso', 'MT'),
    ('Mato Grosso do Sul', 'mato-grosso-do-sul', 'MS'),
    ('Minas Gerais', 'minas-gerais', 'MG'),
    ('Pará', 'pará', 'PA'),
    ('Paraíba', 'paraíba', 'PB'),
    ('Paraná', 'paraná', 'PR'),
    ('Pernambuco', 'pernambuco', 'PE'),
    ('Piauí', 'piauí', 'PI'),
    ('Rio de Janeiro', 'rio-de-janeiro', 'RJ'),
    ('Rio Grande do Norte', 'rio-grande-do-norte', 'RN'),
    ('Rio Grande do Sul', 'rio-grande-do-sul', 'RS'),
    ('Rondônia', 'rondônia', 'RO'),
    ('Roraima', 'roraima', 'RR'),
    ('Santa Catarina', 'santa-catarina', 'SC'),
    ('São Paulo', 'são-paulo', 'SP'),
    ('Sergipe', 'sergipe', 'SE'),
    ('Tocantins', 'tocantins', 'TO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'brazil' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'br-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Brunei (BN) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Belait', 'belait', 'BE'),
    ('Brunei-Muara', 'brunei-muara', 'BM'),
    ('Temburong', 'temburong', 'TE'),
    ('Tutong', 'tutong', 'TU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'brunei' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Bulgaria (BG) — 28 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Blagoevgrad', 'blagoevgrad', '01'),
    ('Burgas', 'burgas', '02'),
    ('Dobrich', 'dobrich', '08'),
    ('Gabrovo', 'gabrovo', '07'),
    ('Haskovo', 'haskovo', '26'),
    ('Kardzhali', 'kardzhali', '09'),
    ('Kyustendil', 'kyustendil', '10'),
    ('Lovech', 'lovech', '11'),
    ('Montana', 'montana', '12'),
    ('Pazardzhik', 'pazardzhik', '13'),
    ('Pernik', 'pernik', '14'),
    ('Pleven', 'pleven', '15'),
    ('Plovdiv', 'plovdiv', '16'),
    ('Razgrad', 'razgrad', '17'),
    ('Ruse', 'ruse', '18'),
    ('Shumen', 'shumen', '27'),
    ('Silistra', 'silistra', '19'),
    ('Sliven', 'sliven', '20'),
    ('Smolyan', 'smolyan', '21'),
    ('Sofia', 'sofia', '23'),
    ('Sofia City', 'sofia-city', '22'),
    ('Stara Zagora', 'stara-zagora', '24'),
    ('Targovishte', 'targovishte', '25'),
    ('Varna', 'varna', '03'),
    ('Veliko Tarnovo', 'veliko-tarnovo', '04'),
    ('Vidin', 'vidin', '05'),
    ('Vratsa', 'vratsa', '06'),
    ('Yambol', 'yambol', '28')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'bulgaria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Burkina Faso (BF) — 58 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Balé', 'balé', 'BAL'),
    ('Bam', 'bam', 'BAM'),
    ('Banwa', 'banwa', 'BAN'),
    ('Bazèga', 'bazèga', 'BAZ'),
    ('Boucle du Mouhoun', 'boucle-du-mouhoun', '01'),
    ('Bougouriba', 'bougouriba', 'BGR'),
    ('Boulgou', 'boulgou', 'BLG'),
    ('Boulkiemde', 'boulkiemde', 'BLK'),
    ('Cascades', 'cascades', '02'),
    ('Centre', 'centre', '03'),
    ('Centre-Est', 'centre-est', '04'),
    ('Centre-Nord', 'centre-nord', '05'),
    ('Centre-Ouest', 'centre-ouest', '06'),
    ('Centre-Sud', 'centre-sud', '07'),
    ('Comoé', 'comoé', 'COM'),
    ('Est', 'est', '08'),
    ('Ganzourgou', 'ganzourgou', 'GAN'),
    ('Gnagna', 'gnagna', 'GNA'),
    ('Gourma', 'gourma', 'GOU'),
    ('Hauts-Bassins', 'hauts-bassins', '09'),
    ('Houet', 'houet', 'HOU'),
    ('Ioba', 'ioba', 'IOB'),
    ('Kadiogo', 'kadiogo', 'KAD'),
    ('Kénédougou', 'kénédougou', 'KEN'),
    ('Komondjari', 'komondjari', 'KMD'),
    ('Kompienga', 'kompienga', 'KMP'),
    ('Kossi', 'kossi', 'KOS'),
    ('Koulpélogo', 'koulpélogo', 'KOP'),
    ('Kouritenga', 'kouritenga', 'KOT'),
    ('Kourwéogo', 'kourwéogo', 'KOW'),
    ('Léraba', 'léraba', 'LER'),
    ('Loroum', 'loroum', 'LOR'),
    ('Mouhoun', 'mouhoun', 'MOU'),
    ('Nahouri', 'nahouri', 'NAO'),
    ('Namentenga', 'namentenga', 'NAM'),
    ('Nayala', 'nayala', 'NAY'),
    ('Nord', 'nord', '10'),
    ('Noumbiel', 'noumbiel', 'NOU'),
    ('Oubritenga', 'oubritenga', 'OUB'),
    ('Oudalan', 'oudalan', 'OUD'),
    ('Passoré', 'passoré', 'PAS'),
    ('Plateau-Central', 'plateau-central', '11'),
    ('Poni', 'poni', 'PON'),
    ('Sahel', 'sahel', '12'),
    ('Sanguié', 'sanguié', 'SNG'),
    ('Sanmatenga', 'sanmatenga', 'SMT'),
    ('Séno', 'séno', 'SEN'),
    ('Sissili', 'sissili', 'SIS'),
    ('Soum', 'soum', 'SOM'),
    ('Sourou', 'sourou', 'SOR'),
    ('Sud-Ouest', 'sud-ouest', '13'),
    ('Tapoa', 'tapoa', 'TAP'),
    ('Tuy', 'tuy', 'TUI'),
    ('Yagha', 'yagha', 'YAG'),
    ('Yatenga', 'yatenga', 'YAT'),
    ('Ziro', 'ziro', 'ZIR'),
    ('Zondoma', 'zondoma', 'ZON'),
    ('Zoundwéogo', 'zoundwéogo', 'ZOU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'burkina-faso' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Burundi (BI) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bi-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bubanza', 'bubanza', 'BB'),
    ('Bujumbura Mairie', 'bujumbura-mairie', 'BM'),
    ('Bujumbura Rural', 'bujumbura-rural', 'BL'),
    ('Bururi', 'bururi', 'BR'),
    ('Cankuzo', 'cankuzo', 'CA'),
    ('Cibitoke', 'cibitoke', 'CI'),
    ('Gitega', 'gitega', 'GI'),
    ('Karuzi', 'karuzi', 'KR'),
    ('Kayanza', 'kayanza', 'KY'),
    ('Kirundo', 'kirundo', 'KI'),
    ('Makamba', 'makamba', 'MA'),
    ('Muramvya', 'muramvya', 'MU'),
    ('Muyinga', 'muyinga', 'MY'),
    ('Mwaro', 'mwaro', 'MW'),
    ('Ngozi', 'ngozi', 'NG'),
    ('Rumonge', 'rumonge', 'RM'),
    ('Rutana', 'rutana', 'RT'),
    ('Ruyigi', 'ruyigi', 'RY')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'burundi' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bi-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cambodia (KH) — 25 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kh-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Banteay Meanchey', 'banteay-meanchey', '1'),
    ('Battambang', 'battambang', '2'),
    ('Kampong Cham', 'kampong-cham', '3'),
    ('Kampong Chhnang', 'kampong-chhnang', '4'),
    ('Kampong Speu', 'kampong-speu', '5'),
    ('Kampong Thom', 'kampong-thom', '6'),
    ('Kampot', 'kampot', '7'),
    ('Kandal', 'kandal', '8'),
    ('Kep', 'kep', '23'),
    ('Koh Kong', 'koh-kong', '9'),
    ('Kratie', 'kratie', '10'),
    ('Mondulkiri', 'mondulkiri', '11'),
    ('Oddar Meanchey', 'oddar-meanchey', '22'),
    ('Pailin', 'pailin', '24'),
    ('Phnom Penh', 'phnom-penh', '12'),
    ('Preah Vihear', 'preah-vihear', '13'),
    ('Prey Veng', 'prey-veng', '14'),
    ('Pursat', 'pursat', '15'),
    ('Ratanakiri', 'ratanakiri', '16'),
    ('Siem Reap', 'siem-reap', '17'),
    ('Sihanoukville', 'sihanoukville', '18'),
    ('Stung Treng', 'stung-treng', '19'),
    ('Svay Rieng', 'svay-rieng', '20'),
    ('Takeo', 'takeo', '21'),
    ('Tboung Khmum', 'tboung-khmum', '25')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cambodia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cameroon (CM) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adamawa', 'adamawa', 'AD'),
    ('Centre', 'centre', 'CE'),
    ('East', 'east', 'ES'),
    ('Far North', 'far-north', 'EN'),
    ('Littoral', 'littoral', 'LT'),
    ('North', 'north', 'NO'),
    ('Northwest', 'northwest', 'NW'),
    ('South', 'south', 'SU'),
    ('Southwest', 'southwest', 'SW'),
    ('West', 'west', 'OU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cameroon' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Canada (CA) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ca-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alberta', 'alberta', 'AB'),
    ('British Columbia', 'british-columbia', 'BC'),
    ('Manitoba', 'manitoba', 'MB'),
    ('New Brunswick', 'new-brunswick', 'NB'),
    ('Newfoundland and Labrador', 'newfoundland-and-labrador', 'NL'),
    ('Northwest Territories', 'northwest-territories', 'NT'),
    ('Nova Scotia', 'nova-scotia', 'NS'),
    ('Nunavut', 'nunavut', 'NU'),
    ('Ontario', 'ontario', 'ON'),
    ('Prince Edward Island', 'prince-edward-island', 'PE'),
    ('Quebec', 'quebec', 'QC'),
    ('Saskatchewan', 'saskatchewan', 'SK'),
    ('Yukon', 'yukon', 'YT')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'canada' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ca-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cape Verde (CV) — 24 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cv-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Barlavento Islands', 'barlavento-islands', 'B'),
    ('Boa Vista', 'boa-vista', 'BV'),
    ('Brava', 'brava', 'BR'),
    ('Maio', 'maio', 'MA'),
    ('Mosteiros', 'mosteiros', 'MO'),
    ('Paul', 'paul', 'PA'),
    ('Porto Novo', 'porto-novo', 'PN'),
    ('Praia', 'praia', 'PR'),
    ('Ribeira Brava', 'ribeira-brava', 'RB'),
    ('Ribeira Grande', 'ribeira-grande', 'RG'),
    ('Ribeira Grande de Santiago', 'ribeira-grande-de-santiago', 'RS'),
    ('Sal', 'sal', 'SL'),
    ('Santa Catarina', 'santa-catarina', 'CA'),
    ('Santa Catarina do Fogo', 'santa-catarina-do-fogo', 'CF'),
    ('Santa Cruz', 'santa-cruz', 'CR'),
    ('São Domingos', 'são-domingos', 'SD'),
    ('São Filipe', 'são-filipe', 'SF'),
    ('São Lourenço dos Órgãos', 'são-lourenço-dos-órgãos', 'SO'),
    ('São Miguel', 'são-miguel', 'SM'),
    ('São Salvador do Mundo', 'são-salvador-do-mundo', 'SS'),
    ('São Vicente', 'são-vicente', 'SV'),
    ('Sotavento Islands', 'sotavento-islands', 'S'),
    ('Tarrafal', 'tarrafal', 'TA'),
    ('Tarrafal de São Nicolau', 'tarrafal-de-são-nicolau', 'TS')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cape-verde' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cv-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cayman Islands (KY) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ky-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Cayman Brac', 'cayman-brac', '02'),
    ('Grand Cayman', 'grand-cayman', '01'),
    ('Little Cayman', 'little-cayman', '03')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cayman-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ky-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Central African Republic (CF) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bamingui-Bangoran', 'bamingui-bangoran', 'BB'),
    ('Bangui', 'bangui', 'BGF'),
    ('Basse-Kotto', 'basse-kotto', 'BK'),
    ('Haut-Mbomou', 'haut-mbomou', 'HM'),
    ('Haute-Kotto', 'haute-kotto', 'HK'),
    ('Kémo', 'kémo', 'KG'),
    ('Lobaye', 'lobaye', 'LB'),
    ('Mambéré-Kadéï', 'mambéré-kadéï', 'HS'),
    ('Mbomou', 'mbomou', 'MB'),
    ('Nana-Grébizi', 'nana-grébizi', 'KB'),
    ('Nana-Mambéré', 'nana-mambéré', 'NM'),
    ('Ombella-M''Poko', 'ombella-mpoko', 'MP'),
    ('Ouaka', 'ouaka', 'UK'),
    ('Ouham', 'ouham', 'AC'),
    ('Ouham-Pendé', 'ouham-pendé', 'OP'),
    ('Sangha-Mbaéré', 'sangha-mbaéré', 'SE'),
    ('Vakaga', 'vakaga', 'VK')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'central-african-republic' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Chad (TD) — 23 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'td-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bahr el Gazel', 'bahr-el-gazel', 'BG'),
    ('Batha', 'batha', 'BA'),
    ('Borkou', 'borkou', 'BO'),
    ('Chari-Baguirmi', 'chari-baguirmi', 'CB'),
    ('Ennedi-Est', 'ennedi-est', 'EE'),
    ('Ennedi-Ouest', 'ennedi-ouest', 'EO'),
    ('Guéra', 'guéra', 'GR'),
    ('Hadjer-Lamis', 'hadjer-lamis', 'HL'),
    ('Kanem', 'kanem', 'KA'),
    ('Lac', 'lac', 'LC'),
    ('Logone Occidental', 'logone-occidental', 'LO'),
    ('Logone Oriental', 'logone-oriental', 'LR'),
    ('Mandoul', 'mandoul', 'MA'),
    ('Mayo-Kebbi Est', 'mayo-kebbi-est', 'ME'),
    ('Mayo-Kebbi Ouest', 'mayo-kebbi-ouest', 'MO'),
    ('Moyen-Chari', 'moyen-chari', 'MC'),
    ('N''Djamena', 'ndjamena', 'ND'),
    ('Ouaddaï', 'ouaddaï', 'OD'),
    ('Salamat', 'salamat', 'SA'),
    ('Sila', 'sila', 'SI'),
    ('Tandjilé', 'tandjilé', 'TA'),
    ('Tibesti', 'tibesti', 'TI'),
    ('Wadi Fira', 'wadi-fira', 'WF')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'chad' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'td-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Chile (CL) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aisén del General Carlos Ibañez del Campo', 'aisén-del-general-carlos-ibañez-del-campo', 'AI'),
    ('Antofagasta', 'antofagasta', 'AN'),
    ('Arica y Parinacota', 'arica-y-parinacota', 'AP'),
    ('Atacama', 'atacama', 'AT'),
    ('Biobío', 'biobío', 'BI'),
    ('Coquimbo', 'coquimbo', 'CO'),
    ('La Araucanía', 'la-araucanía', 'AR'),
    ('Libertador General Bernardo O''Higgins', 'libertador-general-bernardo-ohiggins', 'LI'),
    ('Los Lagos', 'los-lagos', 'LL'),
    ('Los Ríos', 'los-ríos', 'LR'),
    ('Magallanes y de la Antártica Chilena', 'magallanes-y-de-la-antártica-chilena', 'MA'),
    ('Maule', 'maule', 'ML'),
    ('Ñuble', 'ñuble', 'NB'),
    ('Región Metropolitana de Santiago', 'región-metropolitana-de-santiago', 'RM'),
    ('Tarapacá', 'tarapacá', 'TA'),
    ('Valparaíso', 'valparaíso', 'VS')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'chile' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- China (CN) — 34 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anhui', 'anhui', 'AH'),
    ('Beijing', 'beijing', 'BJ'),
    ('Chongqing', 'chongqing', 'CQ'),
    ('Fujian', 'fujian', 'FJ'),
    ('Gansu', 'gansu', 'GS'),
    ('Guangdong', 'guangdong', 'GD'),
    ('Guangxi', 'guangxi', 'GX'),
    ('Guizhou', 'guizhou', 'GZ'),
    ('Hainan', 'hainan', 'HI'),
    ('Hebei', 'hebei', 'HE'),
    ('Heilongjiang', 'heilongjiang', 'HL'),
    ('Henan', 'henan', 'HA'),
    ('Hong Kong SAR', 'hong-kong-sar', 'HK'),
    ('Hubei', 'hubei', 'HB'),
    ('Hunan', 'hunan', 'HN'),
    ('Inner Mongolia', 'inner-mongolia', 'NM'),
    ('Jiangsu', 'jiangsu', 'JS'),
    ('Jiangxi', 'jiangxi', 'JX'),
    ('Jilin', 'jilin', 'JL'),
    ('Liaoning', 'liaoning', 'LN'),
    ('Macau SAR', 'macau-sar', 'MO'),
    ('Ningxia', 'ningxia', 'NX'),
    ('Qinghai', 'qinghai', 'QH'),
    ('Shaanxi', 'shaanxi', 'SN'),
    ('Shandong', 'shandong', 'SD'),
    ('Shanghai', 'shanghai', 'SH'),
    ('Shanxi', 'shanxi', 'SX'),
    ('Sichuan', 'sichuan', 'SC'),
    ('Taiwan', 'taiwan', 'TW'),
    ('Tianjin', 'tianjin', 'TJ'),
    ('Tibet', 'tibet', 'XZ'),
    ('Xinjiang', 'xinjiang', 'XJ'),
    ('Yunnan', 'yunnan', 'YN'),
    ('Zhejiang', 'zhejiang', 'ZJ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'china' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Colombia (CO) — 33 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'co-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Amazonas', 'amazonas', 'AMA'),
    ('Antioquia', 'antioquia', 'ANT'),
    ('Arauca', 'arauca', 'ARA'),
    ('Atlántico', 'atlántico', 'ATL'),
    ('Bogotá D.C.', 'bogotá-dc', 'DC'),
    ('Bolívar', 'bolívar', 'BOL'),
    ('Boyacá', 'boyacá', 'BOY'),
    ('Caldas', 'caldas', 'CAL'),
    ('Caquetá', 'caquetá', 'CAQ'),
    ('Casanare', 'casanare', 'CAS'),
    ('Cauca', 'cauca', 'CAU'),
    ('Cesar', 'cesar', 'CES'),
    ('Chocó', 'chocó', 'CHO'),
    ('Córdoba', 'córdoba', 'COR'),
    ('Cundinamarca', 'cundinamarca', 'CUN'),
    ('Guainía', 'guainía', 'GUA'),
    ('Guaviare', 'guaviare', 'GUV'),
    ('Huila', 'huila', 'HUI'),
    ('La Guajira', 'la-guajira', 'LAG'),
    ('Magdalena', 'magdalena', 'MAG'),
    ('Meta', 'meta', 'MET'),
    ('Nariño', 'nariño', 'NAR'),
    ('Norte de Santander', 'norte-de-santander', 'NSA'),
    ('Putumayo', 'putumayo', 'PUT'),
    ('Quindío', 'quindío', 'QUI'),
    ('Risaralda', 'risaralda', 'RIS'),
    ('San Andrés, Providencia y Santa Catalina', 'san-andrés-providencia-y-santa-catalina', 'SAP'),
    ('Santander', 'santander', 'SAN'),
    ('Sucre', 'sucre', 'SUC'),
    ('Tolima', 'tolima', 'TOL'),
    ('Valle del Cauca', 'valle-del-cauca', 'VAC'),
    ('Vaupés', 'vaupés', 'VAU'),
    ('Vichada', 'vichada', 'VID')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'colombia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'co-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Comoros (KM) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'km-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anjouan', 'anjouan', 'A'),
    ('Grande Comore', 'grande-comore', 'G'),
    ('Mohéli', 'mohéli', 'M')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'comoros' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'km-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Congo (CG) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bouenza', 'bouenza', '11'),
    ('Brazzaville', 'brazzaville', 'BZV'),
    ('Cuvette', 'cuvette', '8'),
    ('Cuvette-Ouest', 'cuvette-ouest', '15'),
    ('Kouilou', 'kouilou', '5'),
    ('Lékoumou', 'lékoumou', '2'),
    ('Likouala', 'likouala', '7'),
    ('Niari', 'niari', '9'),
    ('Plateaux', 'plateaux', '14'),
    ('Pointe-Noire', 'pointe-noire', '16'),
    ('Pool', 'pool', '12'),
    ('Sangha', 'sangha', '13')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'congo' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Costa Rica (CR) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alajuela', 'alajuela', 'A'),
    ('Cartago', 'cartago', 'C'),
    ('Guanacaste', 'guanacaste', 'G'),
    ('Heredia', 'heredia', 'H'),
    ('Limón', 'limón', 'L'),
    ('Puntarenas', 'puntarenas', 'P'),
    ('San José', 'san-josé', 'SJ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'costa-rica' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Croatia (HR) — 20 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'hr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bjelovar-Bilogora', 'bjelovar-bilogora', '07'),
    ('Brod-Posavina', 'brod-posavina', '12'),
    ('Dubrovnik-Neretva', 'dubrovnik-neretva', '19'),
    ('Istria', 'istria', '18'),
    ('Karlovac', 'karlovac', '04'),
    ('Koprivnica-Križevci', 'koprivnica-križevci', '06'),
    ('Krapina-Zagorje', 'krapina-zagorje', '02'),
    ('Lika-Senj', 'lika-senj', '09'),
    ('Međimurje', 'međimurje', '20'),
    ('Osijek-Baranja', 'osijek-baranja', '14'),
    ('Požega-Slavonia', 'požega-slavonia', '11'),
    ('Primorje-Gorski Kotar', 'primorje-gorski-kotar', '08'),
    ('Šibenik-Knin', 'šibenik-knin', '15'),
    ('Sisak-Moslavina', 'sisak-moslavina', '03'),
    ('Split-Dalmatia', 'split-dalmatia', '17'),
    ('Varaždin', 'varaždin', '05'),
    ('Virovitica-Podravina', 'virovitica-podravina', '10'),
    ('Vukovar-Syrmia', 'vukovar-syrmia', '16'),
    ('Zadar', 'zadar', '13'),
    ('Zagreb', 'zagreb', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'croatia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'hr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cuba (CU) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Artemisa', 'artemisa', '15'),
    ('Camagüey', 'camagüey', '09'),
    ('Ciego de Ávila', 'ciego-de-ávila', '08'),
    ('Cienfuegos', 'cienfuegos', '06'),
    ('Granma', 'granma', '12'),
    ('Guantánamo', 'guantánamo', '14'),
    ('Havana', 'havana', '03'),
    ('Holguín', 'holguín', '11'),
    ('Isla de la Juventud', 'isla-de-la-juventud', '99'),
    ('Las Tunas', 'las-tunas', '10'),
    ('Matanzas', 'matanzas', '04'),
    ('Mayabeque', 'mayabeque', '16'),
    ('Pinar del Río', 'pinar-del-río', '01'),
    ('Sancti Spíritus', 'sancti-spíritus', '07'),
    ('Santiago de Cuba', 'santiago-de-cuba', '13'),
    ('Villa Clara', 'villa-clara', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cuba' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Cyprus (CY) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cy-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Famagusta (Mağusa)', 'famagusta-mağusa', '04'),
    ('Kyrenia (Keryneia)', 'kyrenia-keryneia', '06'),
    ('Larnaca (Larnaka)', 'larnaca-larnaka', '03'),
    ('Limassol (Leymasun)', 'limassol-leymasun', '02'),
    ('Nicosia (Lefkoşa)', 'nicosia-lefkoşa', '01'),
    ('Paphos (Pafos)', 'paphos-pafos', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'cyprus' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cy-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Czech Republic (CZ) — 90 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Benešov', 'benešov', '201'),
    ('Beroun', 'beroun', '202'),
    ('Blansko', 'blansko', '641'),
    ('Břeclav', 'břeclav', '644'),
    ('Brno-město', 'brno-město', '642'),
    ('Brno-venkov', 'brno-venkov', '643'),
    ('Bruntál', 'bruntál', '801'),
    ('Česká Lípa', 'česká-lípa', '511'),
    ('České Budějovice', 'české-budějovice', '311'),
    ('Český Krumlov', 'český-krumlov', '312'),
    ('Cheb', 'cheb', '411'),
    ('Chomutov', 'chomutov', '422'),
    ('Chrudim', 'chrudim', '531'),
    ('Děčín', 'děčín', '421'),
    ('Domažlice', 'domažlice', '321'),
    ('Frýdek-Místek', 'frýdek-místek', '802'),
    ('Havlíčkův Brod', 'havlíčkův-brod', '631'),
    ('Hodonín', 'hodonín', '645'),
    ('Hradec Králové', 'hradec-králové', '521'),
    ('Jablonec nad Nisou', 'jablonec-nad-nisou', '512'),
    ('Jeseník', 'jeseník', '711'),
    ('Jičín', 'jičín', '522'),
    ('Jihlava', 'jihlava', '632'),
    ('Jihočeský kraj', 'jihočeský-kraj', '31'),
    ('Jihomoravský kraj', 'jihomoravský-kraj', '64'),
    ('Jindřichův Hradec', 'jindřichův-hradec', '313'),
    ('Karlovarský kraj', 'karlovarský-kraj', '41'),
    ('Karlovy Vary', 'karlovy-vary', '412'),
    ('Karviná', 'karviná', '803'),
    ('Kladno', 'kladno', '203'),
    ('Klatovy', 'klatovy', '322'),
    ('Kolín', 'kolín', '204'),
    ('Kraj Vysočina', 'kraj-vysočina', '63'),
    ('Královéhradecký kraj', 'královéhradecký-kraj', '52'),
    ('Kroměříž', 'kroměříž', '721'),
    ('Kutná Hora', 'kutná-hora', '205'),
    ('Liberec', 'liberec', '513'),
    ('Liberecký kraj', 'liberecký-kraj', '51'),
    ('Litoměřice', 'litoměřice', '423'),
    ('Louny', 'louny', '424'),
    ('Mělník', 'mělník', '206'),
    ('Mladá Boleslav', 'mladá-boleslav', '207'),
    ('Moravskoslezský kraj', 'moravskoslezský-kraj', '80'),
    ('Most', 'most', '425'),
    ('Náchod', 'náchod', '523'),
    ('Nový Jičín', 'nový-jičín', '804'),
    ('Nymburk', 'nymburk', '208'),
    ('Olomouc', 'olomouc', '712'),
    ('Olomoucký kraj', 'olomoucký-kraj', '71'),
    ('Opava', 'opava', '805'),
    ('Ostrava-město', 'ostrava-město', '806'),
    ('Pardubice', 'pardubice', '532'),
    ('Pardubický kraj', 'pardubický-kraj', '53'),
    ('Pelhřimov', 'pelhřimov', '633'),
    ('Písek', 'písek', '314'),
    ('Plzeň-jih', 'plzeň-jih', '324'),
    ('Plzeň-město', 'plzeň-město', '323'),
    ('Plzeň-sever', 'plzeň-sever', '325'),
    ('Plzeňský kraj', 'plzeňský-kraj', '32'),
    ('Prachatice', 'prachatice', '315'),
    ('Praha-východ', 'praha-východ', '209'),
    ('Praha-západ', 'praha-západ', '20A'),
    ('Praha, Hlavní město', 'praha-hlavní-město', '10'),
    ('Přerov', 'přerov', '714'),
    ('Příbram', 'příbram', '20B'),
    ('Prostějov', 'prostějov', '713'),
    ('Rakovník', 'rakovník', '20C'),
    ('Rokycany', 'rokycany', '326'),
    ('Rychnov nad Kněžnou', 'rychnov-nad-kněžnou', '524'),
    ('Semily', 'semily', '514'),
    ('Sokolov', 'sokolov', '413'),
    ('Strakonice', 'strakonice', '316'),
    ('Středočeský kraj', 'středočeský-kraj', '20'),
    ('Šumperk', 'šumperk', '715'),
    ('Svitavy', 'svitavy', '533'),
    ('Tábor', 'tábor', '317'),
    ('Tachov', 'tachov', '327'),
    ('Teplice', 'teplice', '426'),
    ('Třebíč', 'třebíč', '634'),
    ('Trutnov', 'trutnov', '525'),
    ('Uherské Hradiště', 'uherské-hradiště', '722'),
    ('Ústecký kraj', 'ústecký-kraj', '42'),
    ('Ústí nad Labem', 'ústí-nad-labem', '427'),
    ('Ústí nad Orlicí', 'ústí-nad-orlicí', '534'),
    ('Vsetín', 'vsetín', '723'),
    ('Vyškov', 'vyškov', '646'),
    ('Žďár nad Sázavou', 'žďár-nad-sázavou', '635'),
    ('Zlín', 'zlín', '724'),
    ('Zlínský kraj', 'zlínský-kraj', '72'),
    ('Znojmo', 'znojmo', '647')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'czech-republic' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Democratic Republic of the Congo (CD) — 26 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'cd-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bas-Uélé', 'bas-uélé', 'BU'),
    ('Équateur', 'équateur', 'EQ'),
    ('Haut-Katanga', 'haut-katanga', 'HK'),
    ('Haut-Lomami', 'haut-lomami', 'HL'),
    ('Haut-Uélé', 'haut-uélé', 'HU'),
    ('Ituri', 'ituri', 'IT'),
    ('Kasaï', 'kasaï', 'KS'),
    ('Kasaï Central', 'kasaï-central', 'KC'),
    ('Kasaï Oriental', 'kasaï-oriental', 'KE'),
    ('Kinshasa', 'kinshasa', 'KN'),
    ('Kongo Central', 'kongo-central', 'BC'),
    ('Kwango', 'kwango', 'KG'),
    ('Kwilu', 'kwilu', 'KL'),
    ('Lomami', 'lomami', 'LO'),
    ('Lualaba', 'lualaba', 'LU'),
    ('Mai-Ndombe', 'mai-ndombe', 'MN'),
    ('Maniema', 'maniema', 'MA'),
    ('Mongala', 'mongala', 'MO'),
    ('Nord-Kivu', 'nord-kivu', 'NK'),
    ('Nord-Ubangi', 'nord-ubangi', 'NU'),
    ('Sankuru', 'sankuru', 'SA'),
    ('Sud-Kivu', 'sud-kivu', 'SK'),
    ('Sud-Ubangi', 'sud-ubangi', 'SU'),
    ('Tanganyika', 'tanganyika', 'TA'),
    ('Tshopo', 'tshopo', 'TO'),
    ('Tshuapa', 'tshuapa', 'TU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'democratic-republic-of-the-congo' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'cd-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Denmark (DK) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'dk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central Denmark', 'central-denmark', '82'),
    ('Denmark', 'denmark', '84'),
    ('North Denmark', 'north-denmark', '81'),
    ('Southern Denmark', 'southern-denmark', '83'),
    ('Zealand', 'zealand', '85')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'denmark' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'dk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Djibouti (DJ) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'dj-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ali Sabieh', 'ali-sabieh', 'AS'),
    ('Arta', 'arta', 'AR'),
    ('Dikhil', 'dikhil', 'DI'),
    ('Djibouti', 'djibouti', 'DJ'),
    ('Obock', 'obock', 'OB'),
    ('Tadjourah', 'tadjourah', 'TA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'djibouti' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'dj-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Dominica (DM) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'dm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint Andrew', 'saint-andrew', '02'),
    ('Saint David', 'saint-david', '03'),
    ('Saint George', 'saint-george', '04'),
    ('Saint John', 'saint-john', '05'),
    ('Saint Joseph', 'saint-joseph', '06'),
    ('Saint Luke', 'saint-luke', '07'),
    ('Saint Mark', 'saint-mark', '08'),
    ('Saint Patrick', 'saint-patrick', '09'),
    ('Saint Paul', 'saint-paul', '10'),
    ('Saint Peter', 'saint-peter', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'dominica' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'dm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Dominican Republic (DO) — 42 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'do-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Azua', 'azua', '02'),
    ('Baoruco', 'baoruco', '03'),
    ('Barahona', 'barahona', '04'),
    ('Cibao Nordeste', 'cibao-nordeste', '33'),
    ('Cibao Noroeste', 'cibao-noroeste', '34'),
    ('Cibao Norte', 'cibao-norte', '35'),
    ('Cibao Sur', 'cibao-sur', '36'),
    ('Dajabón', 'dajabón', '05'),
    ('Distrito Nacional', 'distrito-nacional', '01'),
    ('Duarte', 'duarte', '06'),
    ('El Seibo', 'el-seibo', '08'),
    ('El Valle', 'el-valle', '37'),
    ('Elías Piña', 'elías-piña', '07'),
    ('Enriquillo', 'enriquillo', '38'),
    ('Espaillat', 'espaillat', '09'),
    ('Hato Mayor', 'hato-mayor', '30'),
    ('Hermanas Mirabal', 'hermanas-mirabal', '19'),
    ('Higuamo', 'higuamo', '39'),
    ('Independencia', 'independencia', '10'),
    ('La Altagracia', 'la-altagracia', '11'),
    ('La Romana', 'la-romana', '12'),
    ('La Vega', 'la-vega', '13'),
    ('María Trinidad Sánchez', 'maría-trinidad-sánchez', '14'),
    ('Monseñor Nouel', 'monseñor-nouel', '28'),
    ('Monte Cristi', 'monte-cristi', '15'),
    ('Monte Plata', 'monte-plata', '29'),
    ('Ozama', 'ozama', '40'),
    ('Pedernales', 'pedernales', '16'),
    ('Peravia', 'peravia', '17'),
    ('Puerto Plata', 'puerto-plata', '18'),
    ('Samaná', 'samaná', '20'),
    ('San Cristóbal', 'san-cristóbal', '21'),
    ('San José de Ocoa', 'san-josé-de-ocoa', '31'),
    ('San Juan', 'san-juan', '22'),
    ('San Pedro de Macorís', 'san-pedro-de-macorís', '23'),
    ('Sánchez Ramírez', 'sánchez-ramírez', '24'),
    ('Santiago', 'santiago', '25'),
    ('Santiago Rodríguez', 'santiago-rodríguez', '26'),
    ('Santo Domingo', 'santo-domingo', '32'),
    ('Valdesia', 'valdesia', '41'),
    ('Valverde', 'valverde', '27'),
    ('Yuma', 'yuma', '42')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'dominican-republic' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'do-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ecuador (EC) — 24 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ec-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Azuay', 'azuay', 'A'),
    ('Bolívar', 'bolívar', 'B'),
    ('Cañar', 'cañar', 'F'),
    ('Carchi', 'carchi', 'C'),
    ('Chimborazo', 'chimborazo', 'H'),
    ('Cotopaxi', 'cotopaxi', 'X'),
    ('El Oro', 'el-oro', 'O'),
    ('Esmeraldas', 'esmeraldas', 'E'),
    ('Galápagos', 'galápagos', 'W'),
    ('Guayas', 'guayas', 'G'),
    ('Imbabura', 'imbabura', 'I'),
    ('Loja', 'loja', 'L'),
    ('Los Ríos', 'los-ríos', 'R'),
    ('Manabí', 'manabí', 'M'),
    ('Morona-Santiago', 'morona-santiago', 'S'),
    ('Napo', 'napo', 'N'),
    ('Orellana', 'orellana', 'D'),
    ('Pastaza', 'pastaza', 'Y'),
    ('Pichincha', 'pichincha', 'P'),
    ('Santa Elena', 'santa-elena', 'SE'),
    ('Santo Domingo de los Tsáchilas', 'santo-domingo-de-los-tsáchilas', 'SD'),
    ('Sucumbíos', 'sucumbíos', 'U'),
    ('Tungurahua', 'tungurahua', 'T'),
    ('Zamora Chinchipe', 'zamora-chinchipe', 'Z')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ecuador' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ec-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Egypt (EG) — 27 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'eg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alexandria', 'alexandria', 'ALX'),
    ('Aswan', 'aswan', 'ASN'),
    ('Asyut', 'asyut', 'AST'),
    ('Beheira', 'beheira', 'BH'),
    ('Beni Suef', 'beni-suef', 'BNS'),
    ('Cairo', 'cairo', 'C'),
    ('Dakahlia', 'dakahlia', 'DK'),
    ('Damietta', 'damietta', 'DT'),
    ('Faiyum', 'faiyum', 'FYM'),
    ('Gharbia', 'gharbia', 'GH'),
    ('Giza', 'giza', 'GZ'),
    ('Ismailia', 'ismailia', 'IS'),
    ('Kafr El-Sheikh', 'kafr-el-sheikh', 'KFS'),
    ('Luxor', 'luxor', 'LX'),
    ('Matrouh', 'matrouh', 'MT'),
    ('Minya', 'minya', 'MN'),
    ('Monufia', 'monufia', 'MNF'),
    ('New Valley', 'new-valley', 'WAD'),
    ('North Sinai', 'north-sinai', 'SIN'),
    ('Port Said', 'port-said', 'PTS'),
    ('Qalyubia', 'qalyubia', 'KB'),
    ('Qena', 'qena', 'KN'),
    ('Red Sea', 'red-sea', 'BA'),
    ('Sharqia', 'sharqia', 'SHR'),
    ('Sohag', 'sohag', 'SHG'),
    ('South Sinai', 'south-sinai', 'JS'),
    ('Suez', 'suez', 'SUZ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'egypt' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'eg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- El Salvador (SV) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sv-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ahuachapán', 'ahuachapán', 'AH'),
    ('Cabañas', 'cabañas', 'CA'),
    ('Chalatenango', 'chalatenango', 'CH'),
    ('Cuscatlán', 'cuscatlán', 'CU'),
    ('La Libertad', 'la-libertad', 'LI'),
    ('La Paz', 'la-paz', 'PA'),
    ('La Unión ', 'la-unión', 'UN'),
    ('Morazán', 'morazán', 'MO'),
    ('San Miguel', 'san-miguel', 'SM'),
    ('San Salvador', 'san-salvador', 'SS'),
    ('San Vicente', 'san-vicente', 'SV'),
    ('Santa Ana', 'santa-ana', 'SA'),
    ('Sonsonate', 'sonsonate', 'SO'),
    ('Usulután', 'usulután', 'US')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'el-salvador' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sv-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Equatorial Guinea (GQ) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gq-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Annobón', 'annobón', 'AN'),
    ('Bioko Norte', 'bioko-norte', 'BN'),
    ('Bioko Sur', 'bioko-sur', 'BS'),
    ('Centro Sur', 'centro-sur', 'CS'),
    ('Djibloho', 'djibloho', 'DJ'),
    ('Insular', 'insular', 'I'),
    ('Kié-Ntem', 'kié-ntem', 'KN'),
    ('Litoral', 'litoral', 'LI'),
    ('Río Muni', 'río-muni', 'C'),
    ('Wele-Nzas', 'wele-nzas', 'WN')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'equatorial-guinea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gq-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Eritrea (ER) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'er-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anseba', 'anseba', 'AN'),
    ('Debub', 'debub', 'DU'),
    ('Gash-Barka', 'gash-barka', 'GB'),
    ('Maekel', 'maekel', 'MA'),
    ('Northern Red Sea', 'northern-red-sea', 'SK'),
    ('Southern Red Sea', 'southern-red-sea', 'DK')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'eritrea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'er-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Estonia (EE) — 94 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ee-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alutaguse', 'alutaguse', '130'),
    ('Anija', 'anija', '141'),
    ('Antsla', 'antsla', '142'),
    ('Elva', 'elva', '171'),
    ('Häädemeeste', 'häädemeeste', '214'),
    ('Haapsalu', 'haapsalu', '184'),
    ('Haljala', 'haljala', '191'),
    ('Harju', 'harju', '37'),
    ('Harku', 'harku', '198'),
    ('Hiiu', 'hiiu', '39'),
    ('Hiiumaa', 'hiiumaa', '205'),
    ('Ida-Viru', 'ida-viru', '45'),
    ('Järva', 'järva', '52'),
    ('Järva', 'järva', '255'),
    ('Joelähtme', 'joelähtme', '245'),
    ('Jõgeva', 'jõgeva', '50'),
    ('Jõgeva', 'jõgeva', '247'),
    ('Jõhvi', 'jõhvi', '251'),
    ('Kadrina', 'kadrina', '272'),
    ('Kambja', 'kambja', '283'),
    ('Kanepi', 'kanepi', '284'),
    ('Kastre', 'kastre', '291'),
    ('Kehtna', 'kehtna', '293'),
    ('Keila', 'keila', '296'),
    ('Kihnu', 'kihnu', '303'),
    ('Kiili', 'kiili', '305'),
    ('Kohila', 'kohila', '317'),
    ('Kohtla-Järve', 'kohtla-järve', '321'),
    ('Kose', 'kose', '338'),
    ('Kuusalu', 'kuusalu', '353'),
    ('Lääne', 'lääne', '56'),
    ('Lääne-Harju', 'lääne-harju', '430'),
    ('Lääne-Nigula', 'lääne-nigula', '441'),
    ('Lääne-Viru', 'lääne-viru', '60'),
    ('Lääneranna', 'lääneranna', '431'),
    ('Loksa', 'loksa', '424'),
    ('Lüganuse', 'lüganuse', '442'),
    ('Luunja', 'luunja', '432'),
    ('Maardu', 'maardu', '446'),
    ('Märjamaa', 'märjamaa', '503'),
    ('Muhu', 'muhu', '478'),
    ('Mulgi', 'mulgi', '480'),
    ('Mustvee', 'mustvee', '486'),
    ('Narva', 'narva', '511'),
    ('Narva-Jõesuu', 'narva-jõesuu', '514'),
    ('Noo', 'noo', '528'),
    ('Otepää', 'otepää', '557'),
    ('Paide', 'paide', '567'),
    ('Pärnu', 'pärnu', '68'),
    ('Pärnu', 'pärnu', '624'),
    ('Peipsiääre', 'peipsiääre', '586'),
    ('Põhja-Pärnu', 'põhja-pärnu', '638'),
    ('Põhja-Sakala', 'põhja-sakala', '615'),
    ('Poltsamaa', 'poltsamaa', '618'),
    ('Põlva', 'põlva', '622'),
    ('Põlva', 'põlva', '64'),
    ('Raasiku', 'raasiku', '651'),
    ('Rae', 'rae', '653'),
    ('Rakvere', 'rakvere', '663'),
    ('Rakvere', 'rakvere', '661'),
    ('Räpina', 'räpina', '708'),
    ('Rapla', 'rapla', '668'),
    ('Rapla', 'rapla', '71'),
    ('Rõuge', 'rõuge', '698'),
    ('Ruhnu', 'ruhnu', '689'),
    ('Saarde', 'saarde', '712'),
    ('Saare', 'saare', '74'),
    ('Saaremaa', 'saaremaa', '714'),
    ('Saku', 'saku', '719'),
    ('Saue', 'saue', '726'),
    ('Setomaa', 'setomaa', '732'),
    ('Sillamäe', 'sillamäe', '735'),
    ('Tallinn', 'tallinn', '784'),
    ('Tapa', 'tapa', '792'),
    ('Tartu', 'tartu', '793'),
    ('Tartu', 'tartu', '79'),
    ('Tartu', 'tartu', '796'),
    ('Toila', 'toila', '803'),
    ('Tori', 'tori', '809'),
    ('Tõrva', 'tõrva', '824'),
    ('Türi', 'türi', '834'),
    ('Väike-Maarja', 'väike-maarja', '928'),
    ('Valga', 'valga', '855'),
    ('Valga', 'valga', '81'),
    ('Viimsi', 'viimsi', '890'),
    ('Viljandi', 'viljandi', '897'),
    ('Viljandi', 'viljandi', '84'),
    ('Viljandi', 'viljandi', '899'),
    ('Vinni', 'vinni', '901'),
    ('Viru-Nigula', 'viru-nigula', '903'),
    ('Vormsi', 'vormsi', '907'),
    ('Võru', 'võru', '917'),
    ('Võru', 'võru', '87'),
    ('Võru', 'võru', '919')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'estonia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ee-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Eswatini (SZ) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Hhohho', 'hhohho', 'HH'),
    ('Lubombo', 'lubombo', 'LU'),
    ('Manzini', 'manzini', 'MA'),
    ('Shiselweni', 'shiselweni', 'SH')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'eswatini' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ethiopia (ET) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'et-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Addis Ababa', 'addis-ababa', 'AA'),
    ('Afar', 'afar', 'AF'),
    ('Amhara', 'amhara', 'AM'),
    ('Benishangul-Gumuz', 'benishangul-gumuz', 'BE'),
    ('Dire Dawa', 'dire-dawa', 'DD'),
    ('Gambela', 'gambela', 'GA'),
    ('Harari', 'harari', 'HA'),
    ('Oromia', 'oromia', 'OR'),
    ('Sidama', 'sidama', 'SI'),
    ('Somali', 'somali', 'SO'),
    ('Southern Nations, Nationalities, and Peoples', 'southern-nations-nationalities-and-peoples', 'SN'),
    ('Southwest Ethiopia Peoples', 'southwest-ethiopia-peoples', 'SW'),
    ('Tigray', 'tigray', 'TI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ethiopia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'et-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Faroe Islands (FO) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fo-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Eysturoy', 'eysturoy', 'EY'),
    ('Northern Isles', 'northern-isles', 'NO'),
    ('Sandoy', 'sandoy', 'SA'),
    ('Streymoy', 'streymoy', 'ST'),
    ('Suðuroy', 'suðuroy', 'SU'),
    ('Vágar', 'vágar', 'VA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'faroe-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fo-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Fiji Islands (FJ) — 19 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fj-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ba', 'ba', '01'),
    ('Bua', 'bua', '02'),
    ('Cakaudrove', 'cakaudrove', '03'),
    ('Central', 'central', 'C'),
    ('Eastern', 'eastern', 'E'),
    ('Kadavu', 'kadavu', '04'),
    ('Lau', 'lau', '05'),
    ('Lomaiviti', 'lomaiviti', '06'),
    ('Macuata', 'macuata', '07'),
    ('Nadroga-Navosa', 'nadroga-navosa', '08'),
    ('Naitasiri', 'naitasiri', '09'),
    ('Namosi', 'namosi', '10'),
    ('Northern', 'northern', 'N'),
    ('Ra', 'ra', '11'),
    ('Rewa', 'rewa', '12'),
    ('Rotuma', 'rotuma', 'R'),
    ('Serua', 'serua', '13'),
    ('Tailevu', 'tailevu', '14'),
    ('Western', 'western', 'W')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'fiji-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fj-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Finland (FI) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fi-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central Finland', 'central-finland', '08'),
    ('Central Ostrobothnia', 'central-ostrobothnia', '07'),
    ('Finland Proper', 'finland-proper', '19'),
    ('Kainuu', 'kainuu', '05'),
    ('Kymenlaakso', 'kymenlaakso', '09'),
    ('Lapland', 'lapland', '10'),
    ('North Karelia', 'north-karelia', '13'),
    ('Northern Ostrobothnia', 'northern-ostrobothnia', '14'),
    ('Northern Savonia', 'northern-savonia', '15'),
    ('Ostrobothnia', 'ostrobothnia', '12'),
    ('Päijänne Tavastia', 'päijänne-tavastia', '16'),
    ('Pirkanmaa', 'pirkanmaa', '11'),
    ('Satakunta', 'satakunta', '17'),
    ('South Karelia', 'south-karelia', '02'),
    ('Southern Ostrobothnia', 'southern-ostrobothnia', '03'),
    ('Southern Savonia', 'southern-savonia', '04'),
    ('Tavastia Proper', 'tavastia-proper', '06'),
    ('Uusimaa', 'uusimaa', '18')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'finland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fi-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- France (FR) — 124 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ain', 'ain', '01'),
    ('Aisne', 'aisne', '02'),
    ('Allier', 'allier', '03'),
    ('Alpes-de-Haute-Provence', 'alpes-de-haute-provence', '04'),
    ('Alpes-Maritimes', 'alpes-maritimes', '06'),
    ('Alsace', 'alsace', '6AE'),
    ('Ardèche', 'ardèche', '07'),
    ('Ardennes', 'ardennes', '08'),
    ('Ariège', 'ariège', '09'),
    ('Aube', 'aube', '10'),
    ('Aude', 'aude', '11'),
    ('Auvergne-Rhône-Alpes', 'auvergne-rhône-alpes', 'ARA'),
    ('Aveyron', 'aveyron', '12'),
    ('Bas-Rhin', 'bas-rhin', '67'),
    ('Bouches-du-Rhône', 'bouches-du-rhône', '13'),
    ('Bourgogne-Franche-Comté', 'bourgogne-franche-comté', 'BFC'),
    ('Bretagne', 'bretagne', 'BRE'),
    ('Calvados', 'calvados', '14'),
    ('Cantal', 'cantal', '15'),
    ('Centre-Val de Loire', 'centre-val-de-loire', 'CVL'),
    ('Charente', 'charente', '16'),
    ('Charente-Maritime', 'charente-maritime', '17'),
    ('Cher', 'cher', '18'),
    ('Clipperton', 'clipperton', 'CP'),
    ('Corrèze', 'corrèze', '19'),
    ('Corse', 'corse', '20R'),
    ('Corse-du-Sud', 'corse-du-sud', '2A'),
    ('Côte-d''Or', 'côte-dor', '21'),
    ('Côtes-d''Armor', 'côtes-darmor', '22'),
    ('Creuse', 'creuse', '23'),
    ('Deux-Sèvres', 'deux-sèvres', '79'),
    ('Dordogne', 'dordogne', '24'),
    ('Doubs', 'doubs', '25'),
    ('Drôme', 'drôme', '26'),
    ('Essonne', 'essonne', '91'),
    ('Eure', 'eure', '27'),
    ('Eure-et-Loir', 'eure-et-loir', '28'),
    ('Finistère', 'finistère', '29'),
    ('French Guiana', 'french-guiana', '973'),
    ('French Polynesia', 'french-polynesia', 'PF'),
    ('French Southern and Antarctic Lands', 'french-southern-and-antarctic-lands', 'TF'),
    ('Gard', 'gard', '30'),
    ('Gers', 'gers', '32'),
    ('Gironde', 'gironde', '33'),
    ('Grand-Est', 'grand-est', 'GES'),
    ('Guadeloupe', 'guadeloupe', '971'),
    ('Haut-Rhin', 'haut-rhin', '68'),
    ('Haute-Corse', 'haute-corse', '2B'),
    ('Haute-Garonne', 'haute-garonne', '31'),
    ('Haute-Loire', 'haute-loire', '43'),
    ('Haute-Marne', 'haute-marne', '52'),
    ('Haute-Saône', 'haute-saône', '70'),
    ('Haute-Savoie', 'haute-savoie', '74'),
    ('Haute-Vienne', 'haute-vienne', '87'),
    ('Hautes-Alpes', 'hautes-alpes', '05'),
    ('Hautes-Pyrénées', 'hautes-pyrénées', '65'),
    ('Hauts-de-France', 'hauts-de-france', 'HDF'),
    ('Hauts-de-Seine', 'hauts-de-seine', '92'),
    ('Hérault', 'hérault', '34'),
    ('Île-de-France', 'île-de-france', 'IDF'),
    ('Ille-et-Vilaine', 'ille-et-vilaine', '35'),
    ('Indre', 'indre', '36'),
    ('Indre-et-Loire', 'indre-et-loire', '37'),
    ('Isère', 'isère', '38'),
    ('Jura', 'jura', '39'),
    ('La Réunion', 'la-réunion', '974'),
    ('Landes', 'landes', '40'),
    ('Loir-et-Cher', 'loir-et-cher', '41'),
    ('Loire', 'loire', '42'),
    ('Loire-Atlantique', 'loire-atlantique', '44'),
    ('Loiret', 'loiret', '45'),
    ('Lot', 'lot', '46'),
    ('Lot-et-Garonne', 'lot-et-garonne', '47'),
    ('Lozère', 'lozère', '48'),
    ('Maine-et-Loire', 'maine-et-loire', '49'),
    ('Manche', 'manche', '50'),
    ('Marne', 'marne', '51'),
    ('Martinique', 'martinique', '972'),
    ('Mayenne', 'mayenne', '53'),
    ('Mayotte', 'mayotte', '976'),
    ('Métropole de Lyon', 'métropole-de-lyon', '69M'),
    ('Meurthe-et-Moselle', 'meurthe-et-moselle', '54'),
    ('Meuse', 'meuse', '55'),
    ('Morbihan', 'morbihan', '56'),
    ('Moselle', 'moselle', '57'),
    ('Nièvre', 'nièvre', '58'),
    ('Nord', 'nord', '59'),
    ('Normandie', 'normandie', 'NOR'),
    ('Nouvelle-Aquitaine', 'nouvelle-aquitaine', 'NAQ'),
    ('Nouvelle-Calédonie', 'nouvelle-calédonie', 'NC'),
    ('Occitanie', 'occitanie', 'OCC'),
    ('Oise', 'oise', '60'),
    ('Orne', 'orne', '61'),
    ('Paris', 'paris', '75C'),
    ('Pas-de-Calais', 'pas-de-calais', '62'),
    ('Pays-de-la-Loire', 'pays-de-la-loire', 'PDL'),
    ('Provence-Alpes-Côte-d’Azur', 'provence-alpes-côte-dazur', 'PAC'),
    ('Puy-de-Dôme', 'puy-de-dôme', '63'),
    ('Pyrénées-Atlantiques', 'pyrénées-atlantiques', '64'),
    ('Pyrénées-Orientales', 'pyrénées-orientales', '66'),
    ('Rhône', 'rhône', '69'),
    ('Saint Pierre and Miquelon', 'saint-pierre-and-miquelon', 'PM'),
    ('Saint-Barthélemy', 'saint-barthélemy', 'BL'),
    ('Saint-Martin', 'saint-martin', 'MF'),
    ('Saône-et-Loire', 'saône-et-loire', '71'),
    ('Sarthe', 'sarthe', '72'),
    ('Savoie', 'savoie', '73'),
    ('Seine-et-Marne', 'seine-et-marne', '77'),
    ('Seine-Maritime', 'seine-maritime', '76'),
    ('Seine-Saint-Denis', 'seine-saint-denis', '93'),
    ('Somme', 'somme', '80'),
    ('Tarn', 'tarn', '81'),
    ('Tarn-et-Garonne', 'tarn-et-garonne', '82'),
    ('Territoire de Belfort', 'territoire-de-belfort', '90'),
    ('Val-d''Oise', 'val-doise', '95'),
    ('Val-de-Marne', 'val-de-marne', '94'),
    ('Var', 'var', '83'),
    ('Vaucluse', 'vaucluse', '84'),
    ('Vendée', 'vendée', '85'),
    ('Vienne', 'vienne', '86'),
    ('Vosges', 'vosges', '88'),
    ('Wallis and Futuna', 'wallis-and-futuna', 'WF'),
    ('Yonne', 'yonne', '89'),
    ('Yvelines', 'yvelines', '78')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'france' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- French Guiana (GF) — 1 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Guyane', 'guyane', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'french-guiana' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- French Polynesia (PF) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Austral Islands', 'austral-islands', '01'),
    ('Leeward Islands', 'leeward-islands', '02'),
    ('Marquesas Islands', 'marquesas-islands', '03'),
    ('Tuamotu-Gambier', 'tuamotu-gambier', '04'),
    ('Windward Islands', 'windward-islands', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'french-polynesia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- French Southern Territories (TF) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adélie Land', 'adélie-land', '01'),
    ('Crozet Islands', 'crozet-islands', '02'),
    ('Kerguelen Islands', 'kerguelen-islands', '03'),
    ('Saint-Paul and Amsterdam Islands', 'saint-paul-and-amsterdam-islands', '04'),
    ('Scattered Islands', 'scattered-islands', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'french-southern-territories' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Gabon (GA) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ga-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Estuaire', 'estuaire', '1'),
    ('Haut-Ogooué', 'haut-ogooué', '2'),
    ('Moyen-Ogooué', 'moyen-ogooué', '3'),
    ('Ngounié', 'ngounié', '4'),
    ('Nyanga', 'nyanga', '5'),
    ('Ogooué-Ivindo', 'ogooué-ivindo', '6'),
    ('Ogooué-Lolo', 'ogooué-lolo', '7'),
    ('Ogooué-Maritime', 'ogooué-maritime', '8'),
    ('Woleu-Ntem', 'woleu-ntem', '9')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'gabon' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ga-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Georgia (GE) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ge-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abkhazia', 'abkhazia', 'AB'),
    ('Adjara', 'adjara', 'AJ'),
    ('Guria', 'guria', 'GU'),
    ('Imereti', 'imereti', 'IM'),
    ('Kakheti', 'kakheti', 'KA'),
    ('Kvemo Kartli', 'kvemo-kartli', 'KK'),
    ('Mtskheta-Mtianeti', 'mtskheta-mtianeti', 'MM'),
    ('Racha-Lechkhumi and Kvemo Svaneti', 'racha-lechkhumi-and-kvemo-svaneti', 'RL'),
    ('Samegrelo-Zemo Svaneti', 'samegrelo-zemo-svaneti', 'SZ'),
    ('Samtskhe-Javakheti', 'samtskhe-javakheti', 'SJ'),
    ('Shida Kartli', 'shida-kartli', 'SK'),
    ('Tbilisi', 'tbilisi', 'TB')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'georgia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ge-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Germany (DE) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'de-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Baden-Württemberg', 'baden-württemberg', 'BW'),
    ('Bavaria', 'bavaria', 'BY'),
    ('Berlin', 'berlin', 'BE'),
    ('Brandenburg', 'brandenburg', 'BB'),
    ('Bremen', 'bremen', 'HB'),
    ('Hamburg', 'hamburg', 'HH'),
    ('Hessen', 'hessen', 'HE'),
    ('Lower Saxony', 'lower-saxony', 'NI'),
    ('Mecklenburg-Vorpommern', 'mecklenburg-vorpommern', 'MV'),
    ('North Rhine-Westphalia', 'north-rhine-westphalia', 'NW'),
    ('Rhineland-Palatinate', 'rhineland-palatinate', 'RP'),
    ('Saarland', 'saarland', 'SL'),
    ('Saxony', 'saxony', 'SN'),
    ('Saxony-Anhalt', 'saxony-anhalt', 'ST'),
    ('Schleswig-Holstein', 'schleswig-holstein', 'SH'),
    ('Thuringia', 'thuringia', 'TH')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'germany' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'de-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ghana (GH) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gh-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ahafo', 'ahafo', 'AF'),
    ('Ashanti', 'ashanti', 'AH'),
    ('Bono', 'bono', 'BO'),
    ('Bono East', 'bono-east', 'BE'),
    ('Central', 'central', 'CP'),
    ('Eastern', 'eastern', 'EP'),
    ('Greater Accra', 'greater-accra', 'AA'),
    ('North East', 'north-east', 'NE'),
    ('Northern', 'northern', 'NP'),
    ('Oti', 'oti', 'OT'),
    ('Savannah', 'savannah', 'SV'),
    ('Upper East', 'upper-east', 'UE'),
    ('Upper West', 'upper-west', 'UW'),
    ('Volta', 'volta', 'TV'),
    ('Western', 'western', 'WP'),
    ('Western North', 'western-north', 'WN')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ghana' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Greece (GR) — 15 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Achaea', 'achaea', '13'),
    ('Attica', 'attica', 'I'),
    ('Central Greece', 'central-greece', 'H'),
    ('Central Macedonia', 'central-macedonia', 'B'),
    ('Crete', 'crete', 'M'),
    ('East Attica', 'east-attica', 'A2'),
    ('East Macedonia and Thrace', 'east-macedonia-and-thrace', 'A'),
    ('Epirus', 'epirus', 'D'),
    ('Ionian Islands', 'ionian-islands', 'F'),
    ('North Aegean', 'north-aegean', 'K'),
    ('Peloponnese', 'peloponnese', 'J'),
    ('South Aegean', 'south-aegean', 'L'),
    ('Thessaly', 'thessaly', 'E'),
    ('West Greece', 'west-greece', 'G'),
    ('West Macedonia', 'west-macedonia', 'C')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'greece' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Greenland (GL) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Avannaata', 'avannaata', 'AV'),
    ('Kujalleq', 'kujalleq', 'KU'),
    ('Qeqertalik', 'qeqertalik', 'QT'),
    ('Qeqqata', 'qeqqata', 'QE'),
    ('Sermersooq', 'sermersooq', 'SM')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'greenland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Grenada (GD) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gd-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Carriacou', 'carriacou', '10'),
    ('Saint Andrew', 'saint-andrew', '01'),
    ('Saint David', 'saint-david', '02'),
    ('Saint George', 'saint-george', '03'),
    ('Saint John', 'saint-john', '04'),
    ('Saint Mark', 'saint-mark', '05'),
    ('Saint Patrick', 'saint-patrick', '06')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'grenada' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gd-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guadeloupe (GP) — 2 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gp-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Basse-Terre', 'basse-terre', '01'),
    ('Pointe-à-Pitre', 'pointe-à-pitre', '02')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guadeloupe' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gp-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guam (GU) — 19 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Agana Heights', 'agana-heights', '01'),
    ('Asan-Maina', 'asan-maina', '02'),
    ('Barrigada', 'barrigada', '03'),
    ('Chalan Pago-Ordot', 'chalan-pago-ordot', '04'),
    ('Dededo', 'dededo', '05'),
    ('Hågat', 'hågat', '06'),
    ('Hagåtña', 'hagåtña', '12'),
    ('Inarajan (Inalåhan)', 'inarajan-inalåhan', '08'),
    ('Mangilao', 'mangilao', '10'),
    ('Merizo (Malesso)', 'merizo-malesso', '09'),
    ('Mongmong-Toto-Maite', 'mongmong-toto-maite', '11'),
    ('Piti', 'piti', '13'),
    ('Santa Rita (Sånta Rita-Sumai)', 'santa-rita-sånta-rita-sumai', '14'),
    ('Sinajana', 'sinajana', '15'),
    ('Talofofo (Talo''fo''fo)', 'talofofo-talofofo', '16'),
    ('Tamuning', 'tamuning', '17'),
    ('Umatac (Humåtak)', 'umatac-humåtak', '07'),
    ('Yigo', 'yigo', '18'),
    ('Yona', 'yona', '19')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guam' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guatemala (GT) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alta Verapaz ', 'alta-verapaz', '16'),
    ('Baja Verapaz ', 'baja-verapaz', '15'),
    ('Chimaltenango ', 'chimaltenango', '04'),
    ('Chiquimula ', 'chiquimula', '20'),
    ('El Progreso ', 'el-progreso', '02'),
    ('Escuintla ', 'escuintla', '05'),
    ('Guatemala ', 'guatemala', '01'),
    ('Huehuetenango ', 'huehuetenango', '13'),
    ('Izabal ', 'izabal', '18'),
    ('Jalapa ', 'jalapa', '21'),
    ('Jutiapa ', 'jutiapa', '22'),
    ('Petén ', 'petén', '17'),
    ('Quetzaltenango ', 'quetzaltenango', '09'),
    ('Quiché ', 'quiché', '14'),
    ('Retalhuleu ', 'retalhuleu', '11'),
    ('Sacatepéquez ', 'sacatepéquez', '03'),
    ('San Marcos ', 'san-marcos', '12'),
    ('Santa Rosa ', 'santa-rosa', '06'),
    ('Sololá ', 'sololá', '07'),
    ('Suchitepéquez ', 'suchitepéquez', '10'),
    ('Totonicapán ', 'totonicapán', '08'),
    ('Zacapa', 'zacapa', '19')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guatemala' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guernsey (GG) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alderney', 'alderney', '04'),
    ('Castel', 'castel', '01'),
    ('Forest', 'forest', '02'),
    ('Sark', 'sark', '07'),
    ('St Andrew', 'st-andrew', '03'),
    ('St Martin', 'st-martin', '05'),
    ('St Peter Port', 'st-peter-port', '06'),
    ('St Pierre du Bois', 'st-pierre-du-bois', '08'),
    ('St Sampson', 'st-sampson', '09'),
    ('St Saviour', 'st-saviour', '10'),
    ('Torteval', 'torteval', '11'),
    ('Vale', 'vale', '12')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guernsey' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guinea (GN) — 41 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Beyla', 'beyla', 'BE'),
    ('Boffa', 'boffa', 'BF'),
    ('Boké', 'boké', 'B'),
    ('Boké', 'boké', 'BK'),
    ('Conakry', 'conakry', 'C'),
    ('Coyah', 'coyah', 'CO'),
    ('Dabola', 'dabola', 'DB'),
    ('Dalaba', 'dalaba', 'DL'),
    ('Dinguiraye', 'dinguiraye', 'DI'),
    ('Dubréka', 'dubréka', 'DU'),
    ('Faranah', 'faranah', 'F'),
    ('Faranah', 'faranah', 'FA'),
    ('Forécariah', 'forécariah', 'FO'),
    ('Fria', 'fria', 'FR'),
    ('Gaoual', 'gaoual', 'GA'),
    ('Guéckédou', 'guéckédou', 'GU'),
    ('Kankan', 'kankan', 'KA'),
    ('Kankan', 'kankan', 'K'),
    ('Kérouané', 'kérouané', 'KE'),
    ('Kindia', 'kindia', 'D'),
    ('Kindia', 'kindia', 'KD'),
    ('Kissidougou', 'kissidougou', 'KS'),
    ('Koubia', 'koubia', 'KB'),
    ('Koundara', 'koundara', 'KN'),
    ('Kouroussa', 'kouroussa', 'KO'),
    ('Labé', 'labé', 'LA'),
    ('Labé', 'labé', 'L'),
    ('Lélouma', 'lélouma', 'LE'),
    ('Lola', 'lola', 'LO'),
    ('Macenta', 'macenta', 'MC'),
    ('Mali', 'mali', 'ML'),
    ('Mamou', 'mamou', 'M'),
    ('Mamou', 'mamou', 'MM'),
    ('Mandiana', 'mandiana', 'MD'),
    ('Nzérékoré', 'nzérékoré', 'N'),
    ('Nzérékoré', 'nzérékoré', 'NZ'),
    ('Pita', 'pita', 'PI'),
    ('Siguiri', 'siguiri', 'SI'),
    ('Télimélé', 'télimélé', 'TE'),
    ('Tougué', 'tougué', 'TO'),
    ('Yomou', 'yomou', 'YO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guinea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guinea-Bissau (GW) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bafatá', 'bafatá', 'BA'),
    ('Biombo', 'biombo', 'BM'),
    ('Bissau', 'bissau', 'BS'),
    ('Bolama', 'bolama', 'BL'),
    ('Cacheu', 'cacheu', 'CA'),
    ('Gabú', 'gabú', 'GA'),
    ('Leste', 'leste', 'L'),
    ('Norte', 'norte', 'N'),
    ('Oio', 'oio', 'OI'),
    ('Quinara', 'quinara', 'QU'),
    ('Sul', 'sul', 'S'),
    ('Tombali', 'tombali', 'TO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guinea-bissau' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Guyana (GY) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gy-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Barima-Waini', 'barima-waini', 'BA'),
    ('Cuyuni-Mazaruni', 'cuyuni-mazaruni', 'CU'),
    ('Demerara-Mahaica', 'demerara-mahaica', 'DE'),
    ('East Berbice-Corentyne', 'east-berbice-corentyne', 'EB'),
    ('Essequibo Islands-West Demerara', 'essequibo-islands-west-demerara', 'ES'),
    ('Mahaica-Berbice', 'mahaica-berbice', 'MA'),
    ('Pomeroon-Supenaam', 'pomeroon-supenaam', 'PM'),
    ('Potaro-Siparuni', 'potaro-siparuni', 'PT'),
    ('Upper Demerara-Berbice', 'upper-demerara-berbice', 'UD'),
    ('Upper Takutu-Upper Essequibo', 'upper-takutu-upper-essequibo', 'UT')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'guyana' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gy-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Haiti (HT) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ht-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Artibonite', 'artibonite', 'AR'),
    ('Centre', 'centre', 'CE'),
    ('Grand''Anse', 'grandanse', 'GA'),
    ('Nippes', 'nippes', 'NI'),
    ('Nord', 'nord', 'ND'),
    ('Nord-Est', 'nord-est', 'NE'),
    ('Nord-Ouest', 'nord-ouest', 'NO'),
    ('Ouest', 'ouest', 'OU'),
    ('Sud', 'sud', 'SD'),
    ('Sud-Est', 'sud-est', 'SE')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'haiti' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ht-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Honduras (HN) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'hn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Atlántida', 'atlántida', 'AT'),
    ('Bay Islands', 'bay-islands', 'IB'),
    ('Choluteca', 'choluteca', 'CH'),
    ('Colón', 'colón', 'CL'),
    ('Comayagua', 'comayagua', 'CM'),
    ('Copán', 'copán', 'CP'),
    ('Cortés', 'cortés', 'CR'),
    ('El Paraíso', 'el-paraíso', 'EP'),
    ('Francisco Morazán', 'francisco-morazán', 'FM'),
    ('Gracias a Dios', 'gracias-a-dios', 'GD'),
    ('Intibucá', 'intibucá', 'IN'),
    ('La Paz', 'la-paz', 'LP'),
    ('Lempira', 'lempira', 'LE'),
    ('Ocotepeque', 'ocotepeque', 'OC'),
    ('Olancho', 'olancho', 'OL'),
    ('Santa Bárbara', 'santa-bárbara', 'SB'),
    ('Valle', 'valle', 'VA'),
    ('Yoro', 'yoro', 'YO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'honduras' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'hn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Hong Kong S.A.R. (HK) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'hk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central and Western', 'central-and-western', 'HCW'),
    ('Eastern', 'eastern', 'HEA'),
    ('Islands', 'islands', 'NIS'),
    ('Kowloon City', 'kowloon-city', 'KKC'),
    ('Kwai Tsing', 'kwai-tsing', 'NKT'),
    ('Kwun Tong', 'kwun-tong', 'KKT'),
    ('North', 'north', 'NNO'),
    ('Sai Kung', 'sai-kung', 'NSK'),
    ('Sha Tin', 'sha-tin', 'NST'),
    ('Sham Shui Po', 'sham-shui-po', 'KSS'),
    ('Southern', 'southern', 'HSO'),
    ('Tai Po', 'tai-po', 'NTP'),
    ('Tsuen Wan', 'tsuen-wan', 'NTW'),
    ('Tuen Mun', 'tuen-mun', 'NTM'),
    ('Wan Chai', 'wan-chai', 'HWC'),
    ('Wong Tai Sin', 'wong-tai-sin', 'KWT'),
    ('Yau Tsim Mong', 'yau-tsim-mong', 'KYT'),
    ('Yuen Long', 'yuen-long', 'NYL')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'hong-kong-sar' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'hk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Hungary (HU) — 43 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'hu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bács-Kiskun', 'bács-kiskun', 'BK'),
    ('Baranya', 'baranya', 'BA'),
    ('Békés', 'békés', 'BE'),
    ('Békéscsaba', 'békéscsaba', 'BC'),
    ('Borsod-Abaúj-Zemplén', 'borsod-abaúj-zemplén', 'BZ'),
    ('Budapest', 'budapest', 'BU'),
    ('Csongrád County', 'csongrád-county', 'CS'),
    ('Debrecen', 'debrecen', 'DE'),
    ('Dunaújváros', 'dunaújváros', 'DU'),
    ('Eger', 'eger', 'EG'),
    ('Érd', 'érd', 'ER'),
    ('Fejér County', 'fejér-county', 'FE'),
    ('Győr', 'győr', 'GY'),
    ('Győr-Moson-Sopron County', 'győr-moson-sopron-county', 'GS'),
    ('Hajdú-Bihar County', 'hajdú-bihar-county', 'HB'),
    ('Heves County', 'heves-county', 'HE'),
    ('Hódmezővásárhely', 'hódmezővásárhely', 'HV'),
    ('Jász-Nagykun-Szolnok County', 'jász-nagykun-szolnok-county', 'JN'),
    ('Kaposvár', 'kaposvár', 'KV'),
    ('Kecskemét', 'kecskemét', 'KM'),
    ('Komárom-Esztergom', 'komárom-esztergom', 'KE'),
    ('Miskolc', 'miskolc', 'MI'),
    ('Nagykanizsa', 'nagykanizsa', 'NK'),
    ('Nógrád County', 'nógrád-county', 'NO'),
    ('Nyíregyháza', 'nyíregyháza', 'NY'),
    ('Pécs', 'pécs', 'PS'),
    ('Pest County', 'pest-county', 'PE'),
    ('Salgótarján', 'salgótarján', 'ST'),
    ('Somogy County', 'somogy-county', 'SO'),
    ('Sopron', 'sopron', 'SN'),
    ('Szabolcs-Szatmár-Bereg County', 'szabolcs-szatmár-bereg-county', 'SZ'),
    ('Szeged', 'szeged', 'SD'),
    ('Székesfehérvár', 'székesfehérvár', 'SF'),
    ('Szekszárd', 'szekszárd', 'SS'),
    ('Szolnok', 'szolnok', 'SK'),
    ('Szombathely', 'szombathely', 'SH'),
    ('Tatabánya', 'tatabánya', 'TB'),
    ('Tolna County', 'tolna-county', 'TO'),
    ('Vas County', 'vas-county', 'VA'),
    ('Veszprém', 'veszprém', 'VM'),
    ('Veszprém County', 'veszprém-county', 'VE'),
    ('Zala County', 'zala-county', 'ZA'),
    ('Zalaegerszeg', 'zalaegerszeg', 'ZE')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'hungary' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'hu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Iceland (IS) — 72 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'is-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Akranes', 'akranes', 'AKN'),
    ('Akureyri', 'akureyri', 'AKU'),
    ('Árborg', 'árborg', 'SFA'),
    ('Árneshreppur', 'árneshreppur', 'ARN'),
    ('Ásahreppur', 'ásahreppur', 'ASA'),
    ('Bláskógabyggð', 'bláskógabyggð', 'BLA'),
    ('Bolungarvík', 'bolungarvík', 'BOL'),
    ('Borgarbyggð', 'borgarbyggð', 'BOG'),
    ('Capital', 'capital', '1'),
    ('Dalabyggð', 'dalabyggð', 'DAB'),
    ('Dalvíkurbyggð', 'dalvíkurbyggð', 'DAV'),
    ('Eastern', 'eastern', '7'),
    ('Eyja- og Miklaholtshreppur', 'eyja-og-miklaholtshreppur', 'EOM'),
    ('Eyjafjarðarsveit', 'eyjafjarðarsveit', 'EYF'),
    ('Fjallabyggð', 'fjallabyggð', 'FJL'),
    ('Fjarðabyggð', 'fjarðabyggð', 'FJD'),
    ('Fljótsdalshreppur', 'fljótsdalshreppur', 'FLR'),
    ('Flóahreppur', 'flóahreppur', 'FLA'),
    ('Garðabær', 'garðabær', 'GAR'),
    ('Grímsnes- og Grafningshreppur', 'grímsnes-og-grafningshreppur', 'GOG'),
    ('Grindavík', 'grindavík', 'GRN'),
    ('Grundarfjörður', 'grundarfjörður', 'GRU'),
    ('Grýtubakkahreppur', 'grýtubakkahreppur', 'GRY'),
    ('Hafnarfjörður', 'hafnarfjörður', 'HAF'),
    ('Hörðarsveit', 'hörðarsveit', 'HRG'),
    ('Hornafjörður', 'hornafjörður', 'SHF'),
    ('Hrunamannahreppur', 'hrunamannahreppur', 'HRU'),
    ('Húnabyggð', 'húnabyggð', 'HUG'),
    ('Húnaþing vestra', 'húnaþing-vestra', 'HUV'),
    ('Hvalfjarðarsveit', 'hvalfjarðarsveit', 'HVA'),
    ('Hveragerði', 'hveragerði', 'HVE'),
    ('Ísafjörður', 'ísafjörður', 'ISA'),
    ('Kaldrananeshreppur', 'kaldrananeshreppur', 'KAL'),
    ('Kjósarhreppur', 'kjósarhreppur', 'KJO'),
    ('Kópavogur', 'kópavogur', 'KOP'),
    ('Langanesbyggð', 'langanesbyggð', 'LAN'),
    ('Mosfellsbær', 'mosfellsbær', 'MOS'),
    ('Múlaþing', 'múlaþing', 'MUL'),
    ('Mýrdalshreppur', 'mýrdalshreppur', 'MYR'),
    ('Norðurþing', 'norðurþing', 'NOR'),
    ('Northeastern', 'northeastern', '6'),
    ('Northwestern', 'northwestern', '5'),
    ('Ölfus', 'ölfus', 'SOL'),
    ('Rangárþing eystra', 'rangárþing-eystra', 'RGE'),
    ('Rangárþing ytra', 'rangárþing-ytra', 'RGY'),
    ('Reykhólahreppur', 'reykhólahreppur', 'RHH'),
    ('Reykjanesbær', 'reykjanesbær', 'RKN'),
    ('Reykjavík', 'reykjavík', 'RKV'),
    ('Seltjarnarnes', 'seltjarnarnes', 'SEL'),
    ('Skaftárhreppur', 'skaftárhreppur', 'SKF'),
    ('Skagabyggð', 'skagabyggð', 'SKG'),
    ('Skagafjörður', 'skagafjörður', 'SKR'),
    ('Skagaströnd', 'skagaströnd', 'SSS'),
    ('Skeiða- og Gnúpverjahreppur', 'skeiða-og-gnúpverjahreppur', 'SOG'),
    ('Skorradalshreppur', 'skorradalshreppur', 'SKO'),
    ('Snæfellsbær', 'snæfellsbær', 'SNF'),
    ('Southern', 'southern', '8'),
    ('Southern Peninsula', 'southern-peninsula', '2'),
    ('Strandabyggð', 'strandabyggð', 'STR'),
    ('Stykkishólmur', 'stykkishólmur', 'STY'),
    ('Súðavík', 'súðavík', 'SDV'),
    ('Suðurnesjabær', 'suðurnesjabær', 'SDN'),
    ('Svalbardsstrandarhreppur', 'svalbardsstrandarhreppur', 'SBT'),
    ('Tálknafjarðarhreppur', 'tálknafjarðarhreppur', 'TAL'),
    ('Tjörneshreppur', 'tjörneshreppur', 'TJO'),
    ('Vestmannaeyjar', 'vestmannaeyjar', 'VEM'),
    ('Vesturbyggð', 'vesturbyggð', 'VER'),
    ('Vogar', 'vogar', 'SVG'),
    ('Vopnafjarðarhreppur', 'vopnafjarðarhreppur', 'VOP'),
    ('Western', 'western', '3'),
    ('Westfjords', 'westfjords', '4'),
    ('Þingeyjarsveit', 'þingeyjarsveit', 'THG')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'iceland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'is-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- India (IN) — 36 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'in-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Andaman and Nicobar Islands', 'andaman-and-nicobar-islands', 'AN'),
    ('Andhra Pradesh', 'andhra-pradesh', 'AP'),
    ('Arunachal Pradesh', 'arunachal-pradesh', 'AR'),
    ('Assam', 'assam', 'AS'),
    ('Bihar', 'bihar', 'BR'),
    ('Chandigarh', 'chandigarh', 'CH'),
    ('Chhattisgarh', 'chhattisgarh', 'CT'),
    ('Dadra and Nagar Haveli and Daman and Diu', 'dadra-and-nagar-haveli-and-daman-and-diu', 'DH'),
    ('Delhi', 'delhi', 'DL'),
    ('Goa', 'goa', 'GA'),
    ('Gujarat', 'gujarat', 'GJ'),
    ('Haryana', 'haryana', 'HR'),
    ('Himachal Pradesh', 'himachal-pradesh', 'HP'),
    ('Jammu and Kashmir', 'jammu-and-kashmir', 'JK'),
    ('Jharkhand', 'jharkhand', 'JH'),
    ('Karnataka', 'karnataka', 'KA'),
    ('Kerala', 'kerala', 'KL'),
    ('Ladakh', 'ladakh', 'LA'),
    ('Lakshadweep', 'lakshadweep', 'LD'),
    ('Madhya Pradesh', 'madhya-pradesh', 'MP'),
    ('Maharashtra', 'maharashtra', 'MH'),
    ('Manipur', 'manipur', 'MN'),
    ('Meghalaya', 'meghalaya', 'ML'),
    ('Mizoram', 'mizoram', 'MZ'),
    ('Nagaland', 'nagaland', 'NL'),
    ('Odisha', 'odisha', 'OR'),
    ('Puducherry', 'puducherry', 'PY'),
    ('Punjab', 'punjab', 'PB'),
    ('Rajasthan', 'rajasthan', 'RJ'),
    ('Sikkim', 'sikkim', 'SK'),
    ('Tamil Nadu', 'tamil-nadu', 'TN'),
    ('Telangana', 'telangana', 'TG'),
    ('Tripura', 'tripura', 'TR'),
    ('Uttar Pradesh', 'uttar-pradesh', 'UP'),
    ('Uttarakhand', 'uttarakhand', 'UK'),
    ('West Bengal', 'west-bengal', 'WB')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'india' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'in-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Indonesia (ID) — 45 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'id-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aceh', 'aceh', 'AC'),
    ('Bali', 'bali', 'BA'),
    ('Banten', 'banten', 'BT'),
    ('Bengkulu', 'bengkulu', 'BE'),
    ('DI Yogyakarta', 'di-yogyakarta', 'YO'),
    ('DKI Jakarta', 'dki-jakarta', 'JK'),
    ('Gorontalo', 'gorontalo', 'GO'),
    ('Jambi', 'jambi', 'JA'),
    ('Jawa', 'jawa', 'JW'),
    ('Jawa Barat', 'jawa-barat', 'JB'),
    ('Jawa Tengah', 'jawa-tengah', 'JT'),
    ('Jawa Timur', 'jawa-timur', 'JI'),
    ('Kalimantan', 'kalimantan', 'KA'),
    ('Kalimantan Barat', 'kalimantan-barat', 'KB'),
    ('Kalimantan Selatan', 'kalimantan-selatan', 'KS'),
    ('Kalimantan Tengah', 'kalimantan-tengah', 'KT'),
    ('Kalimantan Timur', 'kalimantan-timur', 'KI'),
    ('Kalimantan Utara', 'kalimantan-utara', 'KU'),
    ('Kepulauan Bangka Belitung', 'kepulauan-bangka-belitung', 'BB'),
    ('Kepulauan Riau', 'kepulauan-riau', 'KR'),
    ('Lampung', 'lampung', 'LA'),
    ('Maluku', 'maluku', 'ML'),
    ('Maluku', 'maluku', 'MA'),
    ('Maluku Utara', 'maluku-utara', 'MU'),
    ('Nusa Tenggara', 'nusa-tenggara', 'NU'),
    ('Nusa Tenggara Barat', 'nusa-tenggara-barat', 'NB'),
    ('Nusa Tenggara Timur', 'nusa-tenggara-timur', 'NT'),
    ('Papua', 'papua', 'PA'),
    ('Papua', 'papua', 'PP'),
    ('Papua Barat', 'papua-barat', 'PB'),
    ('Papua Barat Daya', 'papua-barat-daya', 'PD'),
    ('Papua Pegunungan', 'papua-pegunungan', 'PE'),
    ('Papua Selatan', 'papua-selatan', 'PS'),
    ('Papua Tengah', 'papua-tengah', 'PT'),
    ('Riau', 'riau', 'RI'),
    ('Sulawesi', 'sulawesi', 'SL'),
    ('Sulawesi Barat', 'sulawesi-barat', 'SR'),
    ('Sulawesi Selatan', 'sulawesi-selatan', 'SN'),
    ('Sulawesi Tengah', 'sulawesi-tengah', 'ST'),
    ('Sulawesi Tenggara', 'sulawesi-tenggara', 'SG'),
    ('Sulawesi Utara', 'sulawesi-utara', 'SA'),
    ('Sumatera', 'sumatera', 'SM'),
    ('Sumatera Barat', 'sumatera-barat', 'SB'),
    ('Sumatera Selatan', 'sumatera-selatan', 'SS'),
    ('Sumatera Utara', 'sumatera-utara', 'SU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'indonesia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'id-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Iran (IR) — 31 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ir-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alborz', 'alborz', '30'),
    ('Ardabil', 'ardabil', '24'),
    ('Bushehr', 'bushehr', '18'),
    ('Chaharmahal and Bakhtiari', 'chaharmahal-and-bakhtiari', '14'),
    ('East Azerbaijan', 'east-azerbaijan', '03'),
    ('Fars', 'fars', '07'),
    ('Gilan', 'gilan', '01'),
    ('Golestan', 'golestan', '27'),
    ('Hamadan', 'hamadan', '13'),
    ('Hormozgan', 'hormozgan', '22'),
    ('Ilam', 'ilam', '16'),
    ('Isfahan', 'isfahan', '10'),
    ('Kerman', 'kerman', '08'),
    ('Kermanshah', 'kermanshah', '05'),
    ('Khuzestan', 'khuzestan', '06'),
    ('Kohgiluyeh and Boyer-Ahmad', 'kohgiluyeh-and-boyer-ahmad', '17'),
    ('Kurdistan', 'kurdistan', '12'),
    ('Lorestan', 'lorestan', '15'),
    ('Markazi', 'markazi', '00'),
    ('Mazandaran', 'mazandaran', '02'),
    ('North Khorasan', 'north-khorasan', '28'),
    ('Qazvin', 'qazvin', '26'),
    ('Qom', 'qom', '25'),
    ('Razavi Khorasan', 'razavi-khorasan', '09'),
    ('Semnan', 'semnan', '20'),
    ('Sistan and Baluchestan', 'sistan-and-baluchestan', '11'),
    ('South Khorasan', 'south-khorasan', '29'),
    ('Tehran', 'tehran', '23'),
    ('West Azarbaijan', 'west-azarbaijan', '04'),
    ('Yazd', 'yazd', '21'),
    ('Zanjan', 'zanjan', '19')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'iran' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ir-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Iraq (IQ) — 19 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'iq-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Anbar', 'al-anbar', 'AN'),
    ('Al Muthanna', 'al-muthanna', 'MU'),
    ('Al-Qādisiyyah', 'al-qādisiyyah', 'QA'),
    ('Babylon', 'babylon', 'BB'),
    ('Baghdad', 'baghdad', 'BG'),
    ('Basra', 'basra', 'BA'),
    ('Dhi Qar', 'dhi-qar', 'DQ'),
    ('Diyala', 'diyala', 'DI'),
    ('Dohuk', 'dohuk', 'DA'),
    ('Erbil', 'erbil', 'AR'),
    ('Iqlim Kurdistan', 'iqlim-kurdistan', 'KR'),
    ('Karbala', 'karbala', 'KA'),
    ('Kirkuk', 'kirkuk', 'KI'),
    ('Maysan', 'maysan', 'MA'),
    ('Najaf', 'najaf', 'NA'),
    ('Nineveh', 'nineveh', 'NI'),
    ('Saladin', 'saladin', 'SD'),
    ('Sulaymaniyah', 'sulaymaniyah', 'SU'),
    ('Wasit', 'wasit', 'WA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'iraq' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'iq-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ireland (IE) — 30 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ie-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Carlow', 'carlow', 'CW'),
    ('Cavan', 'cavan', 'CN'),
    ('Clare', 'clare', 'CE'),
    ('Connacht', 'connacht', 'C'),
    ('Cork', 'cork', 'CO'),
    ('Donegal', 'donegal', 'DL'),
    ('Dublin', 'dublin', 'D'),
    ('Galway', 'galway', 'G'),
    ('Kerry', 'kerry', 'KY'),
    ('Kildare', 'kildare', 'KE'),
    ('Kilkenny', 'kilkenny', 'KK'),
    ('Laois', 'laois', 'LS'),
    ('Leinster', 'leinster', 'L'),
    ('Leitrim', 'leitrim', 'LM'),
    ('Limerick', 'limerick', 'LK'),
    ('Longford', 'longford', 'LD'),
    ('Louth', 'louth', 'LH'),
    ('Mayo', 'mayo', 'MO'),
    ('Meath', 'meath', 'MH'),
    ('Monaghan', 'monaghan', 'MN'),
    ('Munster', 'munster', 'M'),
    ('Offaly', 'offaly', 'OY'),
    ('Roscommon', 'roscommon', 'RN'),
    ('Sligo', 'sligo', 'SO'),
    ('Tipperary', 'tipperary', 'TA'),
    ('Ulster', 'ulster', 'U'),
    ('Waterford', 'waterford', 'WD'),
    ('Westmeath', 'westmeath', 'WH'),
    ('Wexford', 'wexford', 'WX'),
    ('Wicklow', 'wicklow', 'WW')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ireland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ie-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Israel (IL) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'il-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central', 'central', 'M'),
    ('Haifa', 'haifa', 'HA'),
    ('Jerusalem', 'jerusalem', 'JM'),
    ('Northern', 'northern', 'Z'),
    ('Southern', 'southern', 'D'),
    ('Tel Aviv', 'tel-aviv', 'TA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'israel' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'il-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Italy (IT) — 126 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'it-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abruzzo', 'abruzzo', '65'),
    ('Agrigento', 'agrigento', 'AG'),
    ('Alessandria', 'alessandria', 'AL'),
    ('Ancona', 'ancona', 'AN'),
    ('Aosta Valley', 'aosta-valley', '23'),
    ('Apulia', 'apulia', '75'),
    ('Arezzo', 'arezzo', 'AR'),
    ('Ascoli Piceno', 'ascoli-piceno', 'AP'),
    ('Asti', 'asti', 'AT'),
    ('Avellino', 'avellino', 'AV'),
    ('Bari', 'bari', 'BA'),
    ('Barletta-Andria-Trani', 'barletta-andria-trani', 'BT'),
    ('Basilicata', 'basilicata', '77'),
    ('Belluno', 'belluno', 'BL'),
    ('Benevento', 'benevento', 'BN'),
    ('Bergamo', 'bergamo', 'BG'),
    ('Biella', 'biella', 'BI'),
    ('Bologna', 'bologna', 'BO'),
    ('Brescia', 'brescia', 'BS'),
    ('Brindisi', 'brindisi', 'BR'),
    ('Cagliari', 'cagliari', 'CA'),
    ('Calabria', 'calabria', '78'),
    ('Caltanissetta', 'caltanissetta', 'CL'),
    ('Campania', 'campania', '72'),
    ('Campobasso', 'campobasso', 'CB'),
    ('Caserta', 'caserta', 'CE'),
    ('Catania', 'catania', 'CT'),
    ('Catanzaro', 'catanzaro', 'CZ'),
    ('Chieti', 'chieti', 'CH'),
    ('Como', 'como', 'CO'),
    ('Cosenza', 'cosenza', 'CS'),
    ('Cremona', 'cremona', 'CR'),
    ('Crotone', 'crotone', 'KR'),
    ('Cuneo', 'cuneo', 'CN'),
    ('Emilia-Romagna', 'emilia-romagna', '45'),
    ('Enna', 'enna', 'EN'),
    ('Fermo', 'fermo', 'FM'),
    ('Ferrara', 'ferrara', 'FE'),
    ('Florence', 'florence', 'FI'),
    ('Foggia', 'foggia', 'FG'),
    ('Forlì-Cesena', 'forlì-cesena', 'FC'),
    ('Friuli–Venezia Giulia', 'friuli-venezia-giulia', '36'),
    ('Frosinone', 'frosinone', 'FR'),
    ('Genoa', 'genoa', 'GE'),
    ('Gorizia', 'gorizia', 'GO'),
    ('Grosseto', 'grosseto', 'GR'),
    ('Imperia', 'imperia', 'IM'),
    ('Isernia', 'isernia', 'IS'),
    ('L''Aquila', 'laquila', 'AQ'),
    ('La Spezia', 'la-spezia', 'SP'),
    ('Latina', 'latina', 'LT'),
    ('Lazio', 'lazio', '62'),
    ('Lecce', 'lecce', 'LE'),
    ('Lecco', 'lecco', 'LC'),
    ('Liguria', 'liguria', '42'),
    ('Livorno', 'livorno', 'LI'),
    ('Lodi', 'lodi', 'LO'),
    ('Lombardy', 'lombardy', '25'),
    ('Lucca', 'lucca', 'LU'),
    ('Macerata', 'macerata', 'MC'),
    ('Mantua', 'mantua', 'MN'),
    ('Marche', 'marche', '57'),
    ('Massa and Carrara', 'massa-and-carrara', 'MS'),
    ('Matera', 'matera', 'MT'),
    ('Messina', 'messina', 'ME'),
    ('Milan', 'milan', 'MI'),
    ('Modena', 'modena', 'MO'),
    ('Molise', 'molise', '67'),
    ('Monza and Brianza', 'monza-and-brianza', 'MB'),
    ('Naples', 'naples', 'NA'),
    ('Novara', 'novara', 'NO'),
    ('Nuoro', 'nuoro', 'NU'),
    ('Oristano', 'oristano', 'OR'),
    ('Padua', 'padua', 'PD'),
    ('Palermo', 'palermo', 'PA'),
    ('Parma', 'parma', 'PR'),
    ('Pavia', 'pavia', 'PV'),
    ('Perugia', 'perugia', 'PG'),
    ('Pesaro and Urbino', 'pesaro-and-urbino', 'PU'),
    ('Pescara', 'pescara', 'PE'),
    ('Piacenza', 'piacenza', 'PC'),
    ('Piedmont', 'piedmont', '21'),
    ('Pisa', 'pisa', 'PI'),
    ('Pistoia', 'pistoia', 'PT'),
    ('Pordenone', 'pordenone', 'PN'),
    ('Potenza', 'potenza', 'PZ'),
    ('Prato', 'prato', 'PO'),
    ('Ragusa', 'ragusa', 'RG'),
    ('Ravenna', 'ravenna', 'RA'),
    ('Reggio Calabria', 'reggio-calabria', 'RC'),
    ('Reggio Emilia', 'reggio-emilia', 'RE'),
    ('Rieti', 'rieti', 'RI'),
    ('Rimini', 'rimini', 'RN'),
    ('Rome', 'rome', 'RM'),
    ('Rovigo', 'rovigo', 'RO'),
    ('Salerno', 'salerno', 'SA'),
    ('Sardinia', 'sardinia', '88'),
    ('Sassari', 'sassari', 'SS'),
    ('Savona', 'savona', 'SV'),
    ('Sicily', 'sicily', '82'),
    ('Siena', 'siena', 'SI'),
    ('Siracusa', 'siracusa', 'SR'),
    ('Sondrio', 'sondrio', 'SO'),
    ('South Sardinia', 'south-sardinia', 'SU'),
    ('South Tyrol', 'south-tyrol', 'BZ'),
    ('Taranto', 'taranto', 'TA'),
    ('Teramo', 'teramo', 'TE'),
    ('Terni', 'terni', 'TR'),
    ('Trapani', 'trapani', 'TP'),
    ('Trentino', 'trentino', 'TN'),
    ('Trentino-South Tyrol', 'trentino-south-tyrol', '32'),
    ('Treviso', 'treviso', 'TV'),
    ('Trieste', 'trieste', 'TS'),
    ('Turin', 'turin', 'TO'),
    ('Tuscany', 'tuscany', '52'),
    ('Udine', 'udine', 'UD'),
    ('Umbria', 'umbria', '55'),
    ('Varese', 'varese', 'VA'),
    ('Veneto', 'veneto', '34'),
    ('Venice', 'venice', 'VE'),
    ('Verbano-Cusio-Ossola', 'verbano-cusio-ossola', 'VB'),
    ('Vercelli', 'vercelli', 'VC'),
    ('Verona', 'verona', 'VR'),
    ('Vibo Valentia', 'vibo-valentia', 'VV'),
    ('Vicenza', 'vicenza', 'VI'),
    ('Viterbo', 'viterbo', 'VT')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'italy' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'it-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ivory Coast (CI) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ci-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abidjan', 'abidjan', 'AB'),
    ('Bas-Sassandra', 'bas-sassandra', 'BS'),
    ('Comoé', 'comoé', 'CM'),
    ('Denguélé', 'denguélé', 'DN'),
    ('Gôh-Djiboua', 'gôh-djiboua', 'GD'),
    ('Lacs', 'lacs', 'LC'),
    ('Lagunes', 'lagunes', 'LG'),
    ('Montagnes', 'montagnes', 'MG'),
    ('Sassandra-Marahoué', 'sassandra-marahoué', 'SM'),
    ('Savanes', 'savanes', 'SV'),
    ('Vallée du Bandama', 'vallée-du-bandama', 'VB'),
    ('Woroba', 'woroba', 'WR'),
    ('Yamoussoukro', 'yamoussoukro', 'YM'),
    ('Zanzan', 'zanzan', 'ZZ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ivory-coast' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ci-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Jamaica (JM) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'jm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Clarendon', 'clarendon', '13'),
    ('Hanover', 'hanover', '09'),
    ('Kingston', 'kingston', '01'),
    ('Manchester', 'manchester', '12'),
    ('Portland', 'portland', '04'),
    ('Saint Andrew', 'saint-andrew', '02'),
    ('Saint Ann', 'saint-ann', '06'),
    ('Saint Catherine', 'saint-catherine', '14'),
    ('Saint Elizabeth', 'saint-elizabeth', '11'),
    ('Saint James', 'saint-james', '08'),
    ('Saint Mary', 'saint-mary', '05'),
    ('Saint Thomas', 'saint-thomas', '03'),
    ('Trelawny', 'trelawny', '07'),
    ('Westmoreland', 'westmoreland', '10')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'jamaica' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'jm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Japan (JP) — 47 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'jp-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aichi', 'aichi', '23'),
    ('Akita', 'akita', '05'),
    ('Aomori', 'aomori', '02'),
    ('Chiba', 'chiba', '12'),
    ('Ehime', 'ehime', '38'),
    ('Fukui', 'fukui', '18'),
    ('Fukuoka', 'fukuoka', '40'),
    ('Fukushima', 'fukushima', '07'),
    ('Gifu', 'gifu', '21'),
    ('Gunma', 'gunma', '10'),
    ('Hiroshima', 'hiroshima', '34'),
    ('Hokkaidō', 'hokkaidō', '01'),
    ('Hyōgo', 'hyōgo', '28'),
    ('Ibaraki', 'ibaraki', '08'),
    ('Ishikawa', 'ishikawa', '17'),
    ('Iwate', 'iwate', '03'),
    ('Kagawa', 'kagawa', '37'),
    ('Kagoshima', 'kagoshima', '46'),
    ('Kanagawa', 'kanagawa', '14'),
    ('Kōchi', 'kōchi', '39'),
    ('Kumamoto', 'kumamoto', '43'),
    ('Kyōto', 'kyōto', '26'),
    ('Mie', 'mie', '24'),
    ('Miyagi', 'miyagi', '04'),
    ('Miyazaki', 'miyazaki', '45'),
    ('Nagano', 'nagano', '20'),
    ('Nagasaki', 'nagasaki', '42'),
    ('Nara', 'nara', '29'),
    ('Niigata', 'niigata', '15'),
    ('Ōita', 'ōita', '44'),
    ('Okayama', 'okayama', '33'),
    ('Okinawa', 'okinawa', '47'),
    ('Ōsaka', 'ōsaka', '27'),
    ('Saga', 'saga', '41'),
    ('Saitama', 'saitama', '11'),
    ('Shiga', 'shiga', '25'),
    ('Shimane', 'shimane', '32'),
    ('Shizuoka', 'shizuoka', '22'),
    ('Tochigi', 'tochigi', '09'),
    ('Tokushima', 'tokushima', '36'),
    ('Tokyo', 'tokyo', '13'),
    ('Tottori', 'tottori', '31'),
    ('Toyama', 'toyama', '16'),
    ('Wakayama', 'wakayama', '30'),
    ('Yamagata', 'yamagata', '06'),
    ('Yamaguchi', 'yamaguchi', '35'),
    ('Yamanashi', 'yamanashi', '19')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'japan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'jp-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Jersey (JE) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'je-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Grouville', 'grouville', '01'),
    ('St Brelade', 'st-brelade', '02'),
    ('St Clement', 'st-clement', '03'),
    ('St Helier', 'st-helier', '04'),
    ('St John', 'st-john', '05'),
    ('St Lawrence', 'st-lawrence', '06'),
    ('St Martin', 'st-martin', '07'),
    ('St Mary', 'st-mary', '08'),
    ('St Ouen', 'st-ouen', '09'),
    ('St Peter', 'st-peter', '10'),
    ('St Saviour', 'st-saviour', '11'),
    ('Trinity', 'trinity', '12')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'jersey' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'je-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Jordan (JO) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'jo-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ajloun', 'ajloun', 'AJ'),
    ('Amman', 'amman', 'AM'),
    ('Aqaba', 'aqaba', 'AQ'),
    ('Balqa', 'balqa', 'BA'),
    ('Irbid', 'irbid', 'IR'),
    ('Jerash', 'jerash', 'JA'),
    ('Karak', 'karak', 'KA'),
    ('Ma''an', 'maan', 'MN'),
    ('Madaba', 'madaba', 'MD'),
    ('Mafraq', 'mafraq', 'MA'),
    ('Tafilah', 'tafilah', 'AT'),
    ('Zarqa', 'zarqa', 'AZ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'jordan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'jo-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kazakhstan (KZ) — 20 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abai', 'abai', '10'),
    ('Akmola', 'akmola', '11'),
    ('Aktobe', 'aktobe', '15'),
    ('Almaty', 'almaty', '19'),
    ('Almaty', 'almaty', '75'),
    ('Astana', 'astana', '71'),
    ('Atyrau', 'atyrau', '23'),
    ('East Kazakhstan', 'east-kazakhstan', '63'),
    ('Jambyl', 'jambyl', '31'),
    ('Jetisu', 'jetisu', '33'),
    ('Karaganda', 'karaganda', '35'),
    ('Kostanay', 'kostanay', '39'),
    ('Kyzylorda', 'kyzylorda', '43'),
    ('Mangystau', 'mangystau', '47'),
    ('North Kazakhstan', 'north-kazakhstan', '59'),
    ('Pavlodar', 'pavlodar', '55'),
    ('Shymkent', 'shymkent', '79'),
    ('Turkistan', 'turkistan', '61'),
    ('Ulytau', 'ulytau', '62'),
    ('West Kazakhstan', 'west-kazakhstan', '27')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kazakhstan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kenya (KE) — 47 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ke-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Baringo', 'baringo', '01'),
    ('Bomet', 'bomet', '02'),
    ('Bungoma', 'bungoma', '03'),
    ('Busia', 'busia', '04'),
    ('Elgeyo-Marakwet', 'elgeyo-marakwet', '05'),
    ('Embu', 'embu', '06'),
    ('Garissa', 'garissa', '07'),
    ('Homa Bay', 'homa-bay', '08'),
    ('Isiolo', 'isiolo', '09'),
    ('Kajiado', 'kajiado', '10'),
    ('Kakamega', 'kakamega', '11'),
    ('Kericho', 'kericho', '12'),
    ('Kiambu', 'kiambu', '13'),
    ('Kilifi', 'kilifi', '14'),
    ('Kirinyaga', 'kirinyaga', '15'),
    ('Kisii', 'kisii', '16'),
    ('Kisumu', 'kisumu', '17'),
    ('Kitui', 'kitui', '18'),
    ('Kwale', 'kwale', '19'),
    ('Laikipia', 'laikipia', '20'),
    ('Lamu', 'lamu', '21'),
    ('Machakos', 'machakos', '22'),
    ('Makueni', 'makueni', '23'),
    ('Mandera', 'mandera', '24'),
    ('Marsabit', 'marsabit', '25'),
    ('Meru', 'meru', '26'),
    ('Migori', 'migori', '27'),
    ('Mombasa', 'mombasa', '28'),
    ('Murang''a', 'muranga', '29'),
    ('Nairobi City', 'nairobi-city', '30'),
    ('Nakuru', 'nakuru', '31'),
    ('Nandi', 'nandi', '32'),
    ('Narok', 'narok', '33'),
    ('Nyamira', 'nyamira', '34'),
    ('Nyandarua', 'nyandarua', '35'),
    ('Nyeri', 'nyeri', '36'),
    ('Samburu', 'samburu', '37'),
    ('Siaya', 'siaya', '38'),
    ('Taita–Taveta', 'taita-taveta', '39'),
    ('Tana River', 'tana-river', '40'),
    ('Tharaka-Nithi', 'tharaka-nithi', '41'),
    ('Trans Nzoia', 'trans-nzoia', '42'),
    ('Turkana', 'turkana', '43'),
    ('Uasin Gishu', 'uasin-gishu', '44'),
    ('Vihiga', 'vihiga', '45'),
    ('Wajir', 'wajir', '46'),
    ('West Pokot', 'west-pokot', '47')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kenya' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ke-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kiribati (KI) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ki-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Gilbert', 'gilbert', 'G'),
    ('Line', 'line', 'L'),
    ('Phoenix', 'phoenix', 'P')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kiribati' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ki-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kosovo (XK) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'xk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ferizaj', 'ferizaj', 'XUF'),
    ('Gjakove', 'gjakove', 'XDG'),
    ('Gjilan', 'gjilan', 'XGJ'),
    ('Mitrovica', 'mitrovica', 'XKM'),
    ('Peja', 'peja', 'PEJ'),
    ('Pristina', 'pristina', 'XPI'),
    ('Prizren', 'prizren', 'PRI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kosovo' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'xk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kuwait (KW) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Ahmadi', 'al-ahmadi', 'AH'),
    ('Al Asimah', 'al-asimah', 'KU'),
    ('Al Farwaniyah', 'al-farwaniyah', 'FA'),
    ('Al Jahra', 'al-jahra', 'JA'),
    ('Hawalli', 'hawalli', 'HA'),
    ('Mubarak Al-Kabeer', 'mubarak-al-kabeer', 'MU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kuwait' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Kyrgyzstan (KG) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Batken', 'batken', 'B'),
    ('Bishkek', 'bishkek', 'GB'),
    ('Chuy', 'chuy', 'C'),
    ('Issyk-Kul', 'issyk-kul', 'Y'),
    ('Jalal-Abad', 'jalal-abad', 'J'),
    ('Naryn', 'naryn', 'N'),
    ('Osh', 'osh', 'GO'),
    ('Osh', 'osh', 'O'),
    ('Talas', 'talas', 'T')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'kyrgyzstan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Laos (LA) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'la-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Attapeu', 'attapeu', 'AT'),
    ('Bokeo', 'bokeo', 'BK'),
    ('Bolikhamsai', 'bolikhamsai', 'BL'),
    ('Champasak', 'champasak', 'CH'),
    ('Houaphanh', 'houaphanh', 'HO'),
    ('Khammouane', 'khammouane', 'KH'),
    ('Luang Namtha', 'luang-namtha', 'LM'),
    ('Luang Prabang', 'luang-prabang', 'LP'),
    ('Oudomxay', 'oudomxay', 'OU'),
    ('Phongsaly', 'phongsaly', 'PH'),
    ('Sainyabuli', 'sainyabuli', 'XA'),
    ('Salavan', 'salavan', 'SL'),
    ('Savannakhet', 'savannakhet', 'SV'),
    ('Sekong', 'sekong', 'XE'),
    ('Vientiane', 'vientiane', 'VI'),
    ('Vientiane', 'vientiane', 'VT'),
    ('Xaisomboun', 'xaisomboun', 'XS'),
    ('Xiangkhouang', 'xiangkhouang', 'XI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'laos' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'la-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Latvia (LV) — 43 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lv-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ādaži', 'ādaži', '011'),
    ('Aizkraukle', 'aizkraukle', '002'),
    ('Alūksne', 'alūksne', '007'),
    ('Augšdaugava', 'augšdaugava', '111'),
    ('Balvi', 'balvi', '015'),
    ('Bauska', 'bauska', '016'),
    ('Cēsis', 'cēsis', '022'),
    ('Daugavpils', 'daugavpils', 'DGV'),
    ('Dienvidkurzemes', 'dienvidkurzemes', '112'),
    ('Dobele', 'dobele', '026'),
    ('Gulbene', 'gulbene', '033'),
    ('Jēkabpils', 'jēkabpils', '042'),
    ('Jelgava', 'jelgava', '041'),
    ('Jelgava', 'jelgava', 'JEL'),
    ('Jūrmala', 'jūrmala', 'JUR'),
    ('Ķekava', 'ķekava', '052'),
    ('Krāslava', 'krāslava', '047'),
    ('Kuldīga', 'kuldīga', '050'),
    ('Liepāja', 'liepāja', 'LPX'),
    ('Limbaži', 'limbaži', '054'),
    ('Līvāni', 'līvāni', '056'),
    ('Ludza', 'ludza', '058'),
    ('Madona', 'madona', '059'),
    ('Mārupe', 'mārupe', '062'),
    ('Ogre', 'ogre', '067'),
    ('Olaine', 'olaine', '068'),
    ('Preiļi', 'preiļi', '073'),
    ('Rēzekne', 'rēzekne', 'REZ'),
    ('Rēzekne', 'rēzekne', '077'),
    ('Riga', 'riga', 'RIX'),
    ('Ropaži', 'ropaži', '080'),
    ('Salaspils', 'salaspils', '087'),
    ('Saldus', 'saldus', '088'),
    ('Saulkrasti', 'saulkrasti', '089'),
    ('Sigulda', 'sigulda', '091'),
    ('Smiltene', 'smiltene', '094'),
    ('Talsi', 'talsi', '097'),
    ('Tukums', 'tukums', '099'),
    ('Valka', 'valka', '101'),
    ('Valmiera', 'valmiera', '113'),
    ('Varakļāni', 'varakļāni', '102'),
    ('Ventspils', 'ventspils', 'VEN'),
    ('Ventspils', 'ventspils', '106')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'latvia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lv-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Lebanon (LB) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lb-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Akkar', 'akkar', 'AK'),
    ('Baalbek-Hermel', 'baalbek-hermel', 'BH'),
    ('Beirut', 'beirut', 'BA'),
    ('Beqaa', 'beqaa', 'BI'),
    ('Mount Lebanon', 'mount-lebanon', 'JL'),
    ('Nabatieh', 'nabatieh', 'NA'),
    ('North', 'north', 'AS'),
    ('South', 'south', 'JA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'lebanon' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lb-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Lesotho (LS) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ls-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Berea', 'berea', 'D'),
    ('Butha-Buthe', 'butha-buthe', 'B'),
    ('Leribe', 'leribe', 'C'),
    ('Mafeteng', 'mafeteng', 'E'),
    ('Maseru', 'maseru', 'A'),
    ('Mohale''s Hoek', 'mohales-hoek', 'F'),
    ('Mokhotlong', 'mokhotlong', 'J'),
    ('Qacha''s Nek', 'qachas-nek', 'H'),
    ('Quthing', 'quthing', 'G'),
    ('Thaba-Tseka', 'thaba-tseka', 'K')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'lesotho' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ls-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Liberia (LR) — 15 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bomi', 'bomi', 'BM'),
    ('Bong', 'bong', 'BG'),
    ('Gbarpolu', 'gbarpolu', 'GP'),
    ('Grand Bassa', 'grand-bassa', 'GB'),
    ('Grand Cape Mount', 'grand-cape-mount', 'CM'),
    ('Grand Gedeh', 'grand-gedeh', 'GG'),
    ('Grand Kru', 'grand-kru', 'GK'),
    ('Lofa', 'lofa', 'LO'),
    ('Margibi', 'margibi', 'MG'),
    ('Maryland', 'maryland', 'MY'),
    ('Montserrado', 'montserrado', 'MO'),
    ('Nimba', 'nimba', 'NI'),
    ('River Cess', 'river-cess', 'RI'),
    ('River Gee', 'river-gee', 'RG'),
    ('Sinoe', 'sinoe', 'SI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'liberia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Libya (LY) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ly-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Butnan', 'al-butnan', 'BU'),
    ('Al Wahat', 'al-wahat', 'WA'),
    ('Benghazi', 'benghazi', 'BA'),
    ('Derna', 'derna', 'DR'),
    ('Ghat', 'ghat', 'GT'),
    ('Jabal al Akhdar', 'jabal-al-akhdar', 'JA'),
    ('Jabal al Gharbi', 'jabal-al-gharbi', 'JG'),
    ('Jafara', 'jafara', 'JI'),
    ('Jufra', 'jufra', 'JU'),
    ('Kufra', 'kufra', 'KF'),
    ('Marj', 'marj', 'MJ'),
    ('Misrata', 'misrata', 'MI'),
    ('Murqub', 'murqub', 'MB'),
    ('Murzuq', 'murzuq', 'MQ'),
    ('Nalut', 'nalut', 'NL'),
    ('Nuqat al Khams', 'nuqat-al-khams', 'NQ'),
    ('Sabha', 'sabha', 'SB'),
    ('Sirte', 'sirte', 'SR'),
    ('Tripoli', 'tripoli', 'TB'),
    ('Wadi al Hayaa', 'wadi-al-hayaa', 'WD'),
    ('Wadi al Shatii', 'wadi-al-shatii', 'WS'),
    ('Zawiya', 'zawiya', 'ZA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'libya' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ly-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Liechtenstein (LI) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'li-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Balzers', 'balzers', '01'),
    ('Eschen', 'eschen', '02'),
    ('Gamprin', 'gamprin', '03'),
    ('Mauren', 'mauren', '04'),
    ('Planken', 'planken', '05'),
    ('Ruggell', 'ruggell', '06'),
    ('Schaan', 'schaan', '07'),
    ('Schellenberg', 'schellenberg', '08'),
    ('Triesen', 'triesen', '09'),
    ('Triesenberg', 'triesenberg', '10'),
    ('Vaduz', 'vaduz', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'liechtenstein' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'li-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Lithuania (LT) — 70 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Akmenė', 'akmenė', '01'),
    ('Alytus', 'alytus', '03'),
    ('Alytus', 'alytus', '02'),
    ('Alytus', 'alytus', 'AL'),
    ('Anykščiai', 'anykščiai', '04'),
    ('Birštonas', 'birštonas', '05'),
    ('Biržai', 'biržai', '06'),
    ('Druskininkai', 'druskininkai', '07'),
    ('Elektrėnai', 'elektrėnai', '08'),
    ('Ignalina', 'ignalina', '09'),
    ('Jonava', 'jonava', '10'),
    ('Joniškis', 'joniškis', '11'),
    ('Jurbarkas', 'jurbarkas', '12'),
    ('Kaišiadorys', 'kaišiadorys', '13'),
    ('Kalvarija', 'kalvarija', '14'),
    ('Kaunas', 'kaunas', 'KU'),
    ('Kaunas', 'kaunas', '16'),
    ('Kaunas', 'kaunas', '15'),
    ('Kazlų Rūda', 'kazlų-rūda', '17'),
    ('Kėdainiai', 'kėdainiai', '18'),
    ('Kelmė', 'kelmė', '19'),
    ('Klaipėda', 'klaipėda', 'KL'),
    ('Klaipėda', 'klaipėda', '21'),
    ('Klaipėdos miestas', 'klaipėdos-miestas', '20'),
    ('Kretinga', 'kretinga', '22'),
    ('Kupiškis', 'kupiškis', '23'),
    ('Lazdijai', 'lazdijai', '24'),
    ('Marijampolė', 'marijampolė', '25'),
    ('Marijampolė', 'marijampolė', 'MR'),
    ('Mažeikiai', 'mažeikiai', '26'),
    ('Molėtai', 'molėtai', '27'),
    ('Neringa', 'neringa', '28'),
    ('Pagėgiai', 'pagėgiai', '29'),
    ('Pakruojis', 'pakruojis', '30'),
    ('Palanga', 'palanga', '31'),
    ('Panevėžio miestas', 'panevėžio-miestas', '32'),
    ('Panevėžys', 'panevėžys', '33'),
    ('Panevėžys', 'panevėžys', 'PN'),
    ('Pasvalys', 'pasvalys', '34'),
    ('Plungė', 'plungė', '35'),
    ('Prienai', 'prienai', '36'),
    ('Radviliškis', 'radviliškis', '37'),
    ('Raseiniai', 'raseiniai', '38'),
    ('Rietavas', 'rietavas', '39'),
    ('Rokiškis', 'rokiškis', '40'),
    ('Šakiai', 'šakiai', '41'),
    ('Šalčininkai', 'šalčininkai', '42'),
    ('Šiauliai', 'šiauliai', 'SA'),
    ('Šiauliai', 'šiauliai', '43'),
    ('Šiauliai', 'šiauliai', '44'),
    ('Šilalė ', 'šilalė', '45'),
    ('Šilutė', 'šilutė', '46'),
    ('Širvintos', 'širvintos', '47'),
    ('Skuodas', 'skuodas', '48'),
    ('Švenčionys', 'švenčionys', '49'),
    ('Tauragė', 'tauragė', 'TA'),
    ('Tauragė', 'tauragė', '50'),
    ('Telšiai', 'telšiai', '51'),
    ('Telšiai', 'telšiai', 'TE'),
    ('Trakai', 'trakai', '52'),
    ('Ukmergė', 'ukmergė', '53'),
    ('Utena', 'utena', '54'),
    ('Utena', 'utena', 'UT'),
    ('Varėna', 'varėna', '55'),
    ('Vilkaviškis', 'vilkaviškis', '56'),
    ('Vilnius', 'vilnius', '58'),
    ('Vilnius', 'vilnius', '57'),
    ('Vilnius', 'vilnius', 'VL'),
    ('Visaginas', 'visaginas', '59'),
    ('Zarasai', 'zarasai', '60')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'lithuania' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Luxembourg (LU) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Capellen', 'capellen', 'CA'),
    ('Clervaux', 'clervaux', 'CL'),
    ('Diekirch', 'diekirch', 'DI'),
    ('Echternach', 'echternach', 'EC'),
    ('Esch-sur-Alzette', 'esch-sur-alzette', 'ES'),
    ('Grevenmacher', 'grevenmacher', 'G'),
    ('Luxembourg ', 'luxembourg', 'L'),
    ('Mersch', 'mersch', 'ME'),
    ('Redange', 'redange', 'RD'),
    ('Remich', 'remich', 'RM'),
    ('Vianden', 'vianden', 'VD'),
    ('Wiltz', 'wiltz', 'WI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'luxembourg' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Madagascar (MG) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Antananarivo', 'antananarivo', 'T'),
    ('Antsiranana', 'antsiranana', 'D'),
    ('Fianarantsoa', 'fianarantsoa', 'F'),
    ('Mahajanga', 'mahajanga', 'M'),
    ('Toamasina', 'toamasina', 'A'),
    ('Toliara', 'toliara', 'U')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'madagascar' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Malawi (MW) — 31 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Balaka', 'balaka', 'BA'),
    ('Blantyre', 'blantyre', 'BL'),
    ('Central', 'central', 'C'),
    ('Chikwawa', 'chikwawa', 'CK'),
    ('Chiradzulu', 'chiradzulu', 'CR'),
    ('Chitipa', 'chitipa', 'CT'),
    ('Dedza', 'dedza', 'DE'),
    ('Dowa', 'dowa', 'DO'),
    ('Karonga', 'karonga', 'KR'),
    ('Kasungu', 'kasungu', 'KS'),
    ('Likoma', 'likoma', 'LK'),
    ('Lilongwe', 'lilongwe', 'LI'),
    ('Machinga', 'machinga', 'MH'),
    ('Mangochi', 'mangochi', 'MG'),
    ('Mchinji', 'mchinji', 'MC'),
    ('Mulanje', 'mulanje', 'MU'),
    ('Mwanza', 'mwanza', 'MW'),
    ('Mzimba', 'mzimba', 'MZ'),
    ('Neno', 'neno', 'NE'),
    ('Nkhata Bay', 'nkhata-bay', 'NB'),
    ('Nkhotakota', 'nkhotakota', 'NK'),
    ('Northern', 'northern', 'N'),
    ('Nsanje', 'nsanje', 'NS'),
    ('Ntcheu', 'ntcheu', 'NU'),
    ('Ntchisi', 'ntchisi', 'NI'),
    ('Phalombe', 'phalombe', 'PH'),
    ('Rumphi', 'rumphi', 'RU'),
    ('Salima', 'salima', 'SA'),
    ('Southern', 'southern', 'S'),
    ('Thyolo', 'thyolo', 'TH'),
    ('Zomba', 'zomba', 'ZO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'malawi' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Malaysia (MY) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'my-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Johor', 'johor', '01'),
    ('Kedah', 'kedah', '02'),
    ('Kelantan', 'kelantan', '03'),
    ('Kuala Lumpur', 'kuala-lumpur', '14'),
    ('Labuan', 'labuan', '15'),
    ('Malacca', 'malacca', '04'),
    ('Negeri Sembilan', 'negeri-sembilan', '05'),
    ('Pahang', 'pahang', '06'),
    ('Penang', 'penang', '07'),
    ('Perak', 'perak', '08'),
    ('Perlis', 'perlis', '09'),
    ('Putrajaya', 'putrajaya', '16'),
    ('Sabah', 'sabah', '12'),
    ('Sarawak', 'sarawak', '13'),
    ('Selangor', 'selangor', '10'),
    ('Terengganu', 'terengganu', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'malaysia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'my-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Maldives (MV) — 21 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mv-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Addu', 'addu', '01'),
    ('Alif Alif', 'alif-alif', '02'),
    ('Alif Dhaal', 'alif-dhaal', '00'),
    ('Baa', 'baa', '20'),
    ('Dhaalu', 'dhaalu', '17'),
    ('Faafu', 'faafu', '14'),
    ('Gaafu Alif', 'gaafu-alif', '27'),
    ('Gaafu Dhaalu', 'gaafu-dhaalu', '28'),
    ('Gnaviyani', 'gnaviyani', '29'),
    ('Haa Alif', 'haa-alif', '07'),
    ('Haa Dhaalu', 'haa-dhaalu', '23'),
    ('Kaafu', 'kaafu', '26'),
    ('Laamu', 'laamu', '05'),
    ('Lhaviyani', 'lhaviyani', '03'),
    ('Malé', 'malé', 'MLE'),
    ('Meemu', 'meemu', '12'),
    ('Noonu', 'noonu', '25'),
    ('Raa', 'raa', '13'),
    ('Shaviyani', 'shaviyani', '24'),
    ('Thaa', 'thaa', '08'),
    ('Vaavu', 'vaavu', '04')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'maldives' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mv-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mali (ML) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ml-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bamako', 'bamako', 'BKO'),
    ('Gao', 'gao', '7'),
    ('Kayes', 'kayes', '1'),
    ('Kidal', 'kidal', '8'),
    ('Koulikoro', 'koulikoro', '2'),
    ('Ménaka', 'ménaka', '9'),
    ('Mopti', 'mopti', '5'),
    ('Ségou', 'ségou', '4'),
    ('Sikasso', 'sikasso', '3'),
    ('Taoudénit', 'taoudénit', '10'),
    ('Tombouctou', 'tombouctou', '6')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mali' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ml-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Malta (MT) — 68 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Attard', 'attard', '01'),
    ('Balzan', 'balzan', '02'),
    ('Birgu', 'birgu', '03'),
    ('Birkirkara', 'birkirkara', '04'),
    ('Birżebbuġa', 'birżebbuġa', '05'),
    ('Cospicua', 'cospicua', '06'),
    ('Dingli', 'dingli', '07'),
    ('Fgura', 'fgura', '08'),
    ('Floriana', 'floriana', '09'),
    ('Fontana', 'fontana', '10'),
    ('Għajnsielem', 'għajnsielem', '13'),
    ('Għarb', 'għarb', '14'),
    ('Għargħur', 'għargħur', '15'),
    ('Għasri', 'għasri', '16'),
    ('Għaxaq', 'għaxaq', '17'),
    ('Gudja', 'gudja', '11'),
    ('Gżira', 'gżira', '12'),
    ('Ħamrun', 'ħamrun', '18'),
    ('Iklin', 'iklin', '19'),
    ('Kalkara', 'kalkara', '21'),
    ('Kerċem', 'kerċem', '22'),
    ('Kirkop', 'kirkop', '23'),
    ('Lija', 'lija', '24'),
    ('Luqa', 'luqa', '25'),
    ('Marsa', 'marsa', '26'),
    ('Marsaskala', 'marsaskala', '27'),
    ('Marsaxlokk', 'marsaxlokk', '28'),
    ('Mdina', 'mdina', '29'),
    ('Mellieħa', 'mellieħa', '30'),
    ('Mġarr', 'mġarr', '31'),
    ('Mosta', 'mosta', '32'),
    ('Mqabba', 'mqabba', '33'),
    ('Msida', 'msida', '34'),
    ('Mtarfa', 'mtarfa', '35'),
    ('Munxar', 'munxar', '36'),
    ('Nadur', 'nadur', '37'),
    ('Naxxar', 'naxxar', '38'),
    ('Paola', 'paola', '39'),
    ('Pembroke', 'pembroke', '40'),
    ('Pietà', 'pietà', '41'),
    ('Qala', 'qala', '42'),
    ('Qormi', 'qormi', '43'),
    ('Qrendi', 'qrendi', '44'),
    ('Rabat', 'rabat', '46'),
    ('Safi', 'safi', '47'),
    ('San Ġwann', 'san-ġwann', '49'),
    ('San Lawrenz', 'san-lawrenz', '50'),
    ('Sannat', 'sannat', '52'),
    ('Santa Luċija', 'santa-luċija', '53'),
    ('Santa Venera', 'santa-venera', '54'),
    ('Senglea', 'senglea', '20'),
    ('Siġġiewi', 'siġġiewi', '55'),
    ('Sliema', 'sliema', '56'),
    ('St. Julian''s', 'st-julians', '48'),
    ('St. Paul''s Bay', 'st-pauls-bay', '51'),
    ('Swieqi', 'swieqi', '57'),
    ('Ta'' Xbiex', 'ta-xbiex', '58'),
    ('Tarxien', 'tarxien', '59'),
    ('Valletta', 'valletta', '60'),
    ('Victoria', 'victoria', '45'),
    ('Xagħra', 'xagħra', '61'),
    ('Xewkija', 'xewkija', '62'),
    ('Xgħajra', 'xgħajra', '63'),
    ('Żabbar', 'żabbar', '64'),
    ('Żebbuġ Gozo', 'żebbuġ-gozo', '65'),
    ('Żebbuġ Malta', 'żebbuġ-malta', '66'),
    ('Żejtun', 'żejtun', '67'),
    ('Żurrieq', 'żurrieq', '68')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'malta' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Man (Isle of) (IM) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'im-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ayre', 'ayre', '01'),
    ('Garff', 'garff', '02'),
    ('Glenfaba', 'glenfaba', '03'),
    ('Michael', 'michael', '04'),
    ('Middle', 'middle', '05'),
    ('Rushen', 'rushen', '06')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'man-isle-of' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'im-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Marshall Islands (MH) — 26 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mh-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ailinglaplap', 'ailinglaplap', 'ALL'),
    ('Ailuk', 'ailuk', 'ALK'),
    ('Arno', 'arno', 'ARN'),
    ('Aur', 'aur', 'AUR'),
    ('Bikini & Kili', 'bikini-and-kili', 'KIL'),
    ('Ebon', 'ebon', 'EBO'),
    ('Enewetak & Ujelang', 'enewetak-and-ujelang', 'ENI'),
    ('Jabat', 'jabat', 'JAB'),
    ('Jaluit', 'jaluit', 'JAL'),
    ('Kwajalein', 'kwajalein', 'KWA'),
    ('Lae', 'lae', 'LAE'),
    ('Lib', 'lib', 'LIB'),
    ('Likiep', 'likiep', 'LIK'),
    ('Majuro', 'majuro', 'MAJ'),
    ('Maloelap', 'maloelap', 'MAL'),
    ('Mejit', 'mejit', 'MEJ'),
    ('Mili', 'mili', 'MIL'),
    ('Namdrik', 'namdrik', 'NMK'),
    ('Namu', 'namu', 'NMU'),
    ('Ralik', 'ralik', 'L'),
    ('Ratak', 'ratak', 'T'),
    ('Rongelap', 'rongelap', 'RON'),
    ('Ujae', 'ujae', 'UJA'),
    ('Utrik', 'utrik', 'UTI'),
    ('Wotho', 'wotho', 'WTH'),
    ('Wotje', 'wotje', 'WTJ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'marshall-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Martinique (MQ) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mq-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Fort-de-France', 'fort-de-france', '01'),
    ('La Trinité', 'la-trinité', '02'),
    ('Le Marin', 'le-marin', '03'),
    ('Saint-Pierre', 'saint-pierre', '04')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'martinique' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mq-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mauritania (MR) — 15 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adrar', 'adrar', '07'),
    ('Assaba', 'assaba', '03'),
    ('Brakna', 'brakna', '05'),
    ('Dakhlet Nouadhibou', 'dakhlet-nouadhibou', '08'),
    ('Gorgol', 'gorgol', '04'),
    ('Guidimaka', 'guidimaka', '10'),
    ('Hodh Ech Chargui', 'hodh-ech-chargui', '01'),
    ('Hodh El Gharbi', 'hodh-el-gharbi', '02'),
    ('Inchiri', 'inchiri', '12'),
    ('Nouakchott-Nord', 'nouakchott-nord', '14'),
    ('Nouakchott-Ouest', 'nouakchott-ouest', '13'),
    ('Nouakchott-Sud', 'nouakchott-sud', '15'),
    ('Tagant', 'tagant', '09'),
    ('Tiris Zemmour', 'tiris-zemmour', '11'),
    ('Trarza', 'trarza', '06')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mauritania' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mauritius (MU) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Agalega Islands', 'agalega-islands', 'AG'),
    ('Black River', 'black-river', 'BL'),
    ('Flacq', 'flacq', 'FL'),
    ('Grand Port', 'grand-port', 'GP'),
    ('Moka', 'moka', 'MO'),
    ('Pamplemousses', 'pamplemousses', 'PA'),
    ('Plaines Wilhems', 'plaines-wilhems', 'PW'),
    ('Port Louis', 'port-louis', 'PL'),
    ('Rivière du Rempart', 'rivière-du-rempart', 'RR'),
    ('Rodrigues Island', 'rodrigues-island', 'RO'),
    ('Saint Brandon Islands', 'saint-brandon-islands', 'CC'),
    ('Savanne', 'savanne', 'SA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mauritius' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mayotte (YT) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'yt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Acoua', 'acoua', '14'),
    ('Bandraboua', 'bandraboua', '16'),
    ('Bandrélé', 'bandrélé', '05'),
    ('Boueni', 'boueni', '07'),
    ('Chiconi', 'chiconi', '11'),
    ('Chirongui', 'chirongui', '08'),
    ('Dembeni', 'dembeni', '04'),
    ('Dzaoudzi', 'dzaoudzi', '01'),
    ('Kani Keli', 'kani-keli', '06'),
    ('Koungou', 'koungou', '17'),
    ('M''Tsangamouji', 'mtsangamouji', '13'),
    ('Mamoudzou', 'mamoudzou', '03'),
    ('Mtsamboro', 'mtsamboro', '15'),
    ('Ouangani', 'ouangani', '10'),
    ('Pamandzi', 'pamandzi', '02'),
    ('Sada', 'sada', '09'),
    ('Tsingoni', 'tsingoni', '12')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mayotte' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'yt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mexico (MX) — 32 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mx-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aguascalientes', 'aguascalientes', 'AGU'),
    ('Baja California', 'baja-california', 'BCN'),
    ('Baja California Sur', 'baja-california-sur', 'BCS'),
    ('Campeche', 'campeche', 'CAM'),
    ('Chiapas', 'chiapas', 'CHP'),
    ('Chihuahua', 'chihuahua', 'CHH'),
    ('Ciudad de México', 'ciudad-de-méxico', 'CMX'),
    ('Coahuila de Zaragoza', 'coahuila-de-zaragoza', 'COA'),
    ('Colima', 'colima', 'COL'),
    ('Durango', 'durango', 'DUR'),
    ('Estado de México', 'estado-de-méxico', 'MEX'),
    ('Guanajuato', 'guanajuato', 'GUA'),
    ('Guerrero', 'guerrero', 'GRO'),
    ('Hidalgo', 'hidalgo', 'HID'),
    ('Jalisco', 'jalisco', 'JAL'),
    ('Michoacán de Ocampo', 'michoacán-de-ocampo', 'MIC'),
    ('Morelos', 'morelos', 'MOR'),
    ('Nayarit', 'nayarit', 'NAY'),
    ('Nuevo León', 'nuevo-león', 'NLE'),
    ('Oaxaca', 'oaxaca', 'OAX'),
    ('Puebla', 'puebla', 'PUE'),
    ('Querétaro', 'querétaro', 'QUE'),
    ('Quintana Roo', 'quintana-roo', 'ROO'),
    ('San Luis Potosí', 'san-luis-potosí', 'SLP'),
    ('Sinaloa', 'sinaloa', 'SIN'),
    ('Sonora', 'sonora', 'SON'),
    ('Tabasco', 'tabasco', 'TAB'),
    ('Tamaulipas', 'tamaulipas', 'TAM'),
    ('Tlaxcala', 'tlaxcala', 'TLA'),
    ('Veracruz de Ignacio de la Llave', 'veracruz-de-ignacio-de-la-llave', 'VER'),
    ('Yucatán', 'yucatán', 'YUC'),
    ('Zacatecas', 'zacatecas', 'ZAC')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mexico' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mx-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Micronesia (FM) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'fm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Chuuk', 'chuuk', 'TRK'),
    ('Kosrae', 'kosrae', 'KSA'),
    ('Pohnpei', 'pohnpei', 'PNI'),
    ('Yap', 'yap', 'YAP')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'micronesia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'fm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Moldova (MD) — 37 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'md-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anenii Noi', 'anenii-noi', 'AN'),
    ('Bălți', 'bălți', 'BA'),
    ('Basarabeasca', 'basarabeasca', 'BS'),
    ('Bender', 'bender', 'BD'),
    ('Briceni', 'briceni', 'BR'),
    ('Cahul', 'cahul', 'CA'),
    ('Călărași', 'călărași', 'CL'),
    ('Cantemir', 'cantemir', 'CT'),
    ('Căușeni', 'căușeni', 'CS'),
    ('Chișinău', 'chișinău', 'CU'),
    ('Cimișlia', 'cimișlia', 'CM'),
    ('Criuleni', 'criuleni', 'CR'),
    ('Dondușeni', 'dondușeni', 'DO'),
    ('Drochia', 'drochia', 'DR'),
    ('Dubăsari', 'dubăsari', 'DU'),
    ('Edineț', 'edineț', 'ED'),
    ('Fălești', 'fălești', 'FA'),
    ('Florești', 'florești', 'FL'),
    ('Gagauzia', 'gagauzia', 'GA'),
    ('Glodeni', 'glodeni', 'GL'),
    ('Hîncești', 'hîncești', 'HI'),
    ('Ialoveni', 'ialoveni', 'IA'),
    ('Leova', 'leova', 'LE'),
    ('Nisporeni', 'nisporeni', 'NI'),
    ('Ocnița', 'ocnița', 'OC'),
    ('Orhei', 'orhei', 'OR'),
    ('Rezina', 'rezina', 'RE'),
    ('Rîșcani', 'rîșcani', 'RI'),
    ('Sîngerei', 'sîngerei', 'SI'),
    ('Șoldănești', 'șoldănești', 'SD'),
    ('Soroca', 'soroca', 'SO'),
    ('Ștefan Vodă', 'ștefan-vodă', 'SV'),
    ('Strășeni', 'strășeni', 'ST'),
    ('Taraclia', 'taraclia', 'TA'),
    ('Telenești', 'telenești', 'TE'),
    ('Transnistria', 'transnistria', 'SN'),
    ('Ungheni', 'ungheni', 'UN')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'moldova' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'md-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Monaco (MC) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Fontvieille', 'fontvieille', 'FO'),
    ('Jardin Exotique', 'jardin-exotique', 'JE'),
    ('La Colle', 'la-colle', 'CL'),
    ('La Condamine', 'la-condamine', 'CO'),
    ('La Gare', 'la-gare', 'GA'),
    ('La Source', 'la-source', 'SO'),
    ('Larvotto', 'larvotto', 'LA'),
    ('Malbousquet', 'malbousquet', 'MA'),
    ('Monaco-Ville', 'monaco-ville', 'MO'),
    ('Moneghetti', 'moneghetti', 'MG'),
    ('Monte-Carlo', 'monte-carlo', 'MC'),
    ('Moulins', 'moulins', 'MU'),
    ('Port-Hercule', 'port-hercule', 'PH'),
    ('Saint-Roman', 'saint-roman', 'SR'),
    ('Sainte-Dévote', 'sainte-dévote', 'SD'),
    ('Spélugues', 'spélugues', 'SP'),
    ('Vallon de la Rousse', 'vallon-de-la-rousse', 'VR')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'monaco' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mongolia (MN) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Arkhangai', 'arkhangai', '073'),
    ('Bayan-Ölgii', 'bayan-ölgii', '071'),
    ('Bayankhongor', 'bayankhongor', '069'),
    ('Bulgan', 'bulgan', '067'),
    ('Darkhan-Uul', 'darkhan-uul', '037'),
    ('Dornod', 'dornod', '061'),
    ('Dornogovi', 'dornogovi', '063'),
    ('Dundgovi', 'dundgovi', '059'),
    ('Govi-Altai', 'govi-altai', '065'),
    ('Govisümber', 'govisümber', '064'),
    ('Khentii', 'khentii', '039'),
    ('Khovd', 'khovd', '043'),
    ('Khövsgöl', 'khövsgöl', '041'),
    ('Ömnögovi', 'ömnögovi', '053'),
    ('Orkhon', 'orkhon', '035'),
    ('Övörkhangai', 'övörkhangai', '055'),
    ('Selenge', 'selenge', '049'),
    ('Sükhbaatar', 'sükhbaatar', '051'),
    ('Töv', 'töv', '047'),
    ('Ulaanbaatar', 'ulaanbaatar', '001'),
    ('Uvs', 'uvs', '046'),
    ('Zavkhan', 'zavkhan', '057')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mongolia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Montenegro (ME) — 25 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'me-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Andrijevica', 'andrijevica', '01'),
    ('Bar', 'bar', '02'),
    ('Berane', 'berane', '03'),
    ('Bijelo Polje', 'bijelo-polje', '04'),
    ('Budva', 'budva', '05'),
    ('Danilovgrad', 'danilovgrad', '07'),
    ('Gusinje', 'gusinje', '22'),
    ('Herceg-Novi', 'herceg-novi', '08'),
    ('Kolašin', 'kolašin', '09'),
    ('Kotor', 'kotor', '10'),
    ('Mojkovac', 'mojkovac', '11'),
    ('Nikšić', 'nikšić', '12'),
    ('Old Royal Capital Cetinje', 'old-royal-capital-cetinje', '06'),
    ('Petnjica', 'petnjica', '23'),
    ('Plav', 'plav', '13'),
    ('Pljevlja', 'pljevlja', '14'),
    ('Plužine', 'plužine', '15'),
    ('Podgorica', 'podgorica', '16'),
    ('Rožaje', 'rožaje', '17'),
    ('Šavnik', 'šavnik', '18'),
    ('Tivat', 'tivat', '19'),
    ('Tuzi', 'tuzi', '24'),
    ('Ulcinj', 'ulcinj', '20'),
    ('Žabljak', 'žabljak', '21'),
    ('Zeta', 'zeta', '25')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'montenegro' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'me-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Montserrat (MS) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ms-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint Anthony', 'saint-anthony', '03'),
    ('Saint Georges', 'saint-georges', '02'),
    ('Saint Peter', 'saint-peter', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'montserrat' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ms-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Morocco (MA) — 87 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ma-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Agadir-Ida-Ou-Tanane', 'agadir-ida-ou-tanane', 'AGD'),
    ('Al Haouz', 'al-haouz', 'HAO'),
    ('Al Hoceïma', 'al-hoceïma', 'HOC'),
    ('Aousserd (EH)', 'aousserd-eh', 'AOU'),
    ('Assa-Zag (EH-partial)', 'assa-zag-eh-partial', 'ASZ'),
    ('Azilal', 'azilal', 'AZI'),
    ('Béni Mellal', 'béni-mellal', 'BEM'),
    ('Béni Mellal-Khénifra', 'béni-mellal-khénifra', '05'),
    ('Benslimane', 'benslimane', 'BES'),
    ('Berkane', 'berkane', 'BER'),
    ('Berrechid', 'berrechid', 'BRR'),
    ('Boujdour (EH)', 'boujdour-eh', 'BOD'),
    ('Boulemane', 'boulemane', 'BOM'),
    ('Casablanca', 'casablanca', 'CAS'),
    ('Casablanca-Settat', 'casablanca-settat', '06'),
    ('Chefchaouen', 'chefchaouen', 'CHE'),
    ('Chichaoua', 'chichaoua', 'CHI'),
    ('Chtouka-Ait Baha', 'chtouka-ait-baha', 'CHT'),
    ('Dakhla-Oued Ed-Dahab (EH)', 'dakhla-oued-ed-dahab-eh', '12'),
    ('Drâa-Tafilalet', 'drâa-tafilalet', '08'),
    ('Driouch', 'driouch', 'DRI'),
    ('El Hajeb', 'el-hajeb', 'HAJ'),
    ('El Jadida', 'el-jadida', 'JDI'),
    ('El Kelâa des Sraghna', 'el-kelâa-des-sraghna', 'KES'),
    ('Errachidia', 'errachidia', 'ERR'),
    ('Es-Semara (EH-partial)', 'es-semara-eh-partial', 'ESM'),
    ('Essaouira', 'essaouira', 'ESI'),
    ('Fahs-Anjra', 'fahs-anjra', 'FAH'),
    ('Fès', 'fès', 'FES'),
    ('Fès-Meknès', 'fès-meknès', '03'),
    ('Figuig', 'figuig', 'FIG'),
    ('Fquih Ben Salah', 'fquih-ben-salah', 'FQH'),
    ('Guelmim', 'guelmim', 'GUE'),
    ('Guelmim-Oued Noun (EH-partial)', 'guelmim-oued-noun-eh-partial', '10'),
    ('Guercif', 'guercif', 'GUF'),
    ('Ifrane', 'ifrane', 'IFR'),
    ('Inezgane-Ait Melloul', 'inezgane-ait-melloul', 'INE'),
    ('Jerada', 'jerada', 'JRA'),
    ('Kénitra', 'kénitra', 'KEN'),
    ('Khémisset', 'khémisset', 'KHE'),
    ('Khénifra', 'khénifra', 'KHN'),
    ('Khouribga', 'khouribga', 'KHO'),
    ('L''Oriental', 'loriental', '02'),
    ('Laâyoune (EH)', 'laâyoune-eh', 'LAA'),
    ('Laâyoune-Sakia El Hamra (EH-partial)', 'laâyoune-sakia-el-hamra-eh-partial', '11'),
    ('Larache', 'larache', 'LAR'),
    ('M’diq-Fnideq', 'mdiq-fnideq', 'MDF'),
    ('Marrakech', 'marrakech', 'MAR'),
    ('Marrakesh-Safi', 'marrakesh-safi', '07'),
    ('Médiouna', 'médiouna', 'MED'),
    ('Meknès', 'meknès', 'MEK'),
    ('Midelt', 'midelt', 'MID'),
    ('Mohammadia', 'mohammadia', 'MOH'),
    ('Moulay Yacoub', 'moulay-yacoub', 'MOU'),
    ('Nador', 'nador', 'NAD'),
    ('Nouaceur', 'nouaceur', 'NOU'),
    ('Ouarzazate', 'ouarzazate', 'OUA'),
    ('Oued Ed-Dahab (EH)', 'oued-ed-dahab-eh', 'OUD'),
    ('Ouezzane', 'ouezzane', 'OUZ'),
    ('Oujda-Angad', 'oujda-angad', 'OUJ'),
    ('Rabat', 'rabat', 'RAB'),
    ('Rabat-Salé-Kénitra', 'rabat-salé-kénitra', '04'),
    ('Rehamna', 'rehamna', 'REH'),
    ('Safi', 'safi', 'SAF'),
    ('Salé', 'salé', 'SAL'),
    ('Sefrou', 'sefrou', 'SEF'),
    ('Settat', 'settat', 'SET'),
    ('Sidi Bennour', 'sidi-bennour', 'SIB'),
    ('Sidi Ifni', 'sidi-ifni', 'SIF'),
    ('Sidi Kacem', 'sidi-kacem', 'SIK'),
    ('Sidi Slimane', 'sidi-slimane', 'SIL'),
    ('Skhirate-Témara', 'skhirate-témara', 'SKH'),
    ('Souss-Massa', 'souss-massa', '09'),
    ('Tan-Tan (EH-partial)', 'tan-tan-eh-partial', 'TNT'),
    ('Tanger-Assilah', 'tanger-assilah', 'TNG'),
    ('Tanger-Tétouan-Al Hoceïma', 'tanger-tétouan-al-hoceïma', '01'),
    ('Taounate', 'taounate', 'TAO'),
    ('Taourirt', 'taourirt', 'TAI'),
    ('Tarfaya (EH-partial)', 'tarfaya-eh-partial', 'TAF'),
    ('Taroudannt', 'taroudannt', 'TAR'),
    ('Tata', 'tata', 'TAT'),
    ('Taza', 'taza', 'TAZ'),
    ('Tétouan', 'tétouan', 'TET'),
    ('Tinghir', 'tinghir', 'TIN'),
    ('Tiznit', 'tiznit', 'TIZ'),
    ('Youssoufia', 'youssoufia', 'YUS'),
    ('Zagora', 'zagora', 'ZAG')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'morocco' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ma-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Mozambique (MZ) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Cabo Delgado', 'cabo-delgado', 'P'),
    ('Gaza', 'gaza', 'G'),
    ('Inhambane', 'inhambane', 'I'),
    ('Manica', 'manica', 'B'),
    ('Maputo', 'maputo', 'L'),
    ('Maputo', 'maputo', 'MPM'),
    ('Nampula', 'nampula', 'N'),
    ('Niassa', 'niassa', 'A'),
    ('Sofala', 'sofala', 'S'),
    ('Tete', 'tete', 'T'),
    ('Zambezia', 'zambezia', 'Q')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'mozambique' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Myanmar (MM) — 15 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ayeyarwady', 'ayeyarwady', '07'),
    ('Bago', 'bago', '02'),
    ('Chin', 'chin', '14'),
    ('Kachin', 'kachin', '11'),
    ('Kayah', 'kayah', '12'),
    ('Kayin', 'kayin', '13'),
    ('Magway', 'magway', '03'),
    ('Mandalay', 'mandalay', '04'),
    ('Mon State', 'mon-state', '15'),
    ('Naypyidaw', 'naypyidaw', '18'),
    ('Rakhine', 'rakhine', '16'),
    ('Sagaing', 'sagaing', '01'),
    ('Shan', 'shan', '17'),
    ('Tanintharyi', 'tanintharyi', '05'),
    ('Yangon', 'yangon', '06')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'myanmar' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Namibia (NA) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'na-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Erongo', 'erongo', 'ER'),
    ('Hardap', 'hardap', 'HA'),
    ('Karas', 'karas', 'KA'),
    ('Kavango East', 'kavango-east', 'KE'),
    ('Kavango West', 'kavango-west', 'KW'),
    ('Khomas', 'khomas', 'KH'),
    ('Kunene', 'kunene', 'KU'),
    ('Ohangwena', 'ohangwena', 'OW'),
    ('Omaheke', 'omaheke', 'OH'),
    ('Omusati', 'omusati', 'OS'),
    ('Oshana', 'oshana', 'ON'),
    ('Oshikoto', 'oshikoto', 'OT'),
    ('Otjozondjupa', 'otjozondjupa', 'OD'),
    ('Zambezi', 'zambezi', 'CA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'namibia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'na-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Nauru (NR) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aiwo', 'aiwo', '01'),
    ('Anabar', 'anabar', '02'),
    ('Anetan', 'anetan', '03'),
    ('Anibare', 'anibare', '04'),
    ('Baiti', 'baiti', '05'),
    ('Boe', 'boe', '06'),
    ('Buada', 'buada', '07'),
    ('Denigomodu', 'denigomodu', '08'),
    ('Ewa', 'ewa', '09'),
    ('Ijuw', 'ijuw', '10'),
    ('Meneng', 'meneng', '11'),
    ('Nibok', 'nibok', '12'),
    ('Uaboe', 'uaboe', '13'),
    ('Yaren', 'yaren', '14')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'nauru' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Nepal (NP) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'np-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bagmati', 'bagmati', 'P3'),
    ('Gandaki', 'gandaki', 'P4'),
    ('Karnali', 'karnali', 'P6'),
    ('Koshi', 'koshi', 'P1'),
    ('Lumbini', 'lumbini', 'P5'),
    ('Madhesh', 'madhesh', 'P2'),
    ('Sudurpashchim', 'sudurpashchim', 'P7')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'nepal' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'np-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Netherlands (NL) — 12 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Drenthe', 'drenthe', 'DR'),
    ('Flevoland', 'flevoland', 'FL'),
    ('Friesland', 'friesland', 'FR'),
    ('Gelderland', 'gelderland', 'GE'),
    ('Groningen', 'groningen', 'GR'),
    ('Limburg', 'limburg', 'LI'),
    ('North Brabant', 'north-brabant', 'NB'),
    ('North Holland', 'north-holland', 'NH'),
    ('Overijssel', 'overijssel', 'OV'),
    ('South Holland', 'south-holland', 'ZH'),
    ('Utrecht', 'utrecht', 'UT'),
    ('Zeeland', 'zeeland', 'ZE')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'netherlands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- New Caledonia (NC) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Loyalty Islands Province', 'loyalty-islands-province', '03'),
    ('North Province', 'north-province', '02'),
    ('South Province', 'south-province', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'new-caledonia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- New Zealand (NZ) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Auckland', 'auckland', 'AUK'),
    ('Bay of Plenty', 'bay-of-plenty', 'BOP'),
    ('Canterbury', 'canterbury', 'CAN'),
    ('Chatham Islands', 'chatham-islands', 'CIT'),
    ('Gisborne', 'gisborne', 'GIS'),
    ('Hawke''s Bay', 'hawkes-bay', 'HKB'),
    ('Manawatu-Whanganui', 'manawatu-whanganui', 'MWT'),
    ('Marlborough', 'marlborough', 'MBH'),
    ('Nelson', 'nelson', 'NSN'),
    ('Northland', 'northland', 'NTL'),
    ('Otago', 'otago', 'OTA'),
    ('Southland', 'southland', 'STL'),
    ('Taranaki', 'taranaki', 'TKI'),
    ('Tasman', 'tasman', 'TAS'),
    ('Waikato', 'waikato', 'WKO'),
    ('Wellington', 'wellington', 'WGN'),
    ('West Coast', 'west-coast', 'WTC')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'new-zealand' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Nicaragua (NI) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ni-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Boaco', 'boaco', 'BO'),
    ('Carazo', 'carazo', 'CA'),
    ('Chinandega', 'chinandega', 'CI'),
    ('Chontales', 'chontales', 'CO'),
    ('Estelí', 'estelí', 'ES'),
    ('Granada', 'granada', 'GR'),
    ('Jinotega', 'jinotega', 'JI'),
    ('León', 'león', 'LE'),
    ('Madriz', 'madriz', 'MD'),
    ('Managua', 'managua', 'MN'),
    ('Masaya', 'masaya', 'MS'),
    ('Matagalpa', 'matagalpa', 'MT'),
    ('North Caribbean Coast', 'north-caribbean-coast', 'AN'),
    ('Nueva Segovia', 'nueva-segovia', 'NS'),
    ('Río San Juan', 'río-san-juan', 'SJ'),
    ('Rivas', 'rivas', 'RI'),
    ('South Caribbean Coast', 'south-caribbean-coast', 'AS')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'nicaragua' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ni-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Niger (NE) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ne-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Agadez', 'agadez', '1'),
    ('Diffa', 'diffa', '2'),
    ('Dosso', 'dosso', '3'),
    ('Maradi', 'maradi', '4'),
    ('Niamey', 'niamey', '8'),
    ('Tahoua', 'tahoua', '5'),
    ('Tillabéri', 'tillabéri', '6'),
    ('Zinder', 'zinder', '7')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'niger' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ne-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Nigeria (NG) — 37 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ng-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abia', 'abia', 'AB'),
    ('Abuja Federal Capital Territory', 'abuja-federal-capital-territory', 'FC'),
    ('Adamawa', 'adamawa', 'AD'),
    ('Akwa Ibom', 'akwa-ibom', 'AK'),
    ('Anambra', 'anambra', 'AN'),
    ('Bauchi', 'bauchi', 'BA'),
    ('Bayelsa', 'bayelsa', 'BY'),
    ('Benue', 'benue', 'BE'),
    ('Borno', 'borno', 'BO'),
    ('Cross River', 'cross-river', 'CR'),
    ('Delta', 'delta', 'DE'),
    ('Ebonyi', 'ebonyi', 'EB'),
    ('Edo', 'edo', 'ED'),
    ('Ekiti', 'ekiti', 'EK'),
    ('Enugu', 'enugu', 'EN'),
    ('Gombe', 'gombe', 'GO'),
    ('Imo', 'imo', 'IM'),
    ('Jigawa', 'jigawa', 'JI'),
    ('Kaduna', 'kaduna', 'KD'),
    ('Kano', 'kano', 'KN'),
    ('Katsina', 'katsina', 'KT'),
    ('Kebbi', 'kebbi', 'KE'),
    ('Kogi', 'kogi', 'KO'),
    ('Kwara', 'kwara', 'KW'),
    ('Lagos', 'lagos', 'LA'),
    ('Nasarawa', 'nasarawa', 'NA'),
    ('Niger', 'niger', 'NI'),
    ('Ogun', 'ogun', 'OG'),
    ('Ondo', 'ondo', 'ON'),
    ('Osun', 'osun', 'OS'),
    ('Oyo', 'oyo', 'OY'),
    ('Plateau', 'plateau', 'PL'),
    ('Rivers', 'rivers', 'RI'),
    ('Sokoto', 'sokoto', 'SO'),
    ('Taraba', 'taraba', 'TA'),
    ('Yobe', 'yobe', 'YO'),
    ('Zamfara', 'zamfara', 'ZA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'nigeria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ng-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Niue (NU) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'nu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alofi North', 'alofi-north', '14'),
    ('Alofi South', 'alofi-south', '13'),
    ('Avatele', 'avatele', '11'),
    ('Hakupu', 'hakupu', '09'),
    ('Hikutavake', 'hikutavake', '04'),
    ('Lakepa', 'lakepa', '07'),
    ('Liku', 'liku', '08'),
    ('Makefu', 'makefu', '01'),
    ('Mutalau', 'mutalau', '06'),
    ('Namukulu', 'namukulu', '03'),
    ('Tamakautoga', 'tamakautoga', '12'),
    ('Toi', 'toi', '05'),
    ('Tuapa', 'tuapa', '02'),
    ('Vaiea', 'vaiea', '10')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'niue' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'nu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- North Korea (KP) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kp-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Chagang', 'chagang', '04'),
    ('Kaesong', 'kaesong', '15'),
    ('Kangwon', 'kangwon', '07'),
    ('Nampho', 'nampho', '14'),
    ('North Hamgyong', 'north-hamgyong', '09'),
    ('North Hwanghae', 'north-hwanghae', '06'),
    ('North Pyongan', 'north-pyongan', '03'),
    ('Pyongyang', 'pyongyang', '01'),
    ('Rason', 'rason', '13'),
    ('Ryanggang', 'ryanggang', '10'),
    ('South Hamgyong', 'south-hamgyong', '08'),
    ('South Hwanghae', 'south-hwanghae', '05'),
    ('South Pyongan', 'south-pyongan', '02')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'north-korea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kp-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- North Macedonia (MK) — 80 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aerodrom', 'aerodrom', '801'),
    ('Aračinovo', 'aračinovo', '802'),
    ('Berovo', 'berovo', '201'),
    ('Bitola', 'bitola', '501'),
    ('Bogdanci', 'bogdanci', '401'),
    ('Bogovinje', 'bogovinje', '601'),
    ('Bosilovo', 'bosilovo', '402'),
    ('Brvenica', 'brvenica', '602'),
    ('Butel', 'butel', '803'),
    ('Čair', 'čair', '815'),
    ('Čaška', 'čaška', '109'),
    ('Centar', 'centar', '814'),
    ('Centar Župa', 'centar-župa', '313'),
    ('Češinovo-Obleševo', 'češinovo-obleševo', '210'),
    ('Čučer-Sandevo', 'čučer-sandevo', '816'),
    ('Debar', 'debar', '303'),
    ('Debarca', 'debarca', '304'),
    ('Delčevo', 'delčevo', '203'),
    ('Demir Hisar', 'demir-hisar', '502'),
    ('Demir Kapija', 'demir-kapija', '103'),
    ('Dojran', 'dojran', '406'),
    ('Dolneni', 'dolneni', '503'),
    ('Gazi Baba', 'gazi-baba', '804'),
    ('Gevgelija', 'gevgelija', '405'),
    ('Gjorče Petrov', 'gjorče-petrov', '805'),
    ('Gostivar', 'gostivar', '604'),
    ('Gradsko', 'gradsko', '102'),
    ('Ilinden', 'ilinden', '807'),
    ('Jegunovce', 'jegunovce', '606'),
    ('Karbinci', 'karbinci', '205'),
    ('Karpoš', 'karpoš', '808'),
    ('Kavadarci', 'kavadarci', '104'),
    ('Kičevo', 'kičevo', '307'),
    ('Kisela Voda', 'kisela-voda', '809'),
    ('Kočani', 'kočani', '206'),
    ('Konče', 'konče', '407'),
    ('Kratovo', 'kratovo', '701'),
    ('Kriva Palanka', 'kriva-palanka', '702'),
    ('Krivogaštani', 'krivogaštani', '504'),
    ('Kruševo', 'kruševo', '505'),
    ('Kumanovo', 'kumanovo', '703'),
    ('Lipkovo', 'lipkovo', '704'),
    ('Lozovo', 'lozovo', '105'),
    ('Makedonska Kamenica', 'makedonska-kamenica', '207'),
    ('Makedonski Brod', 'makedonski-brod', '308'),
    ('Mavrovo and Rostuša', 'mavrovo-and-rostuša', '607'),
    ('Mogila', 'mogila', '506'),
    ('Negotino', 'negotino', '106'),
    ('Novaci', 'novaci', '507'),
    ('Novo Selo', 'novo-selo', '408'),
    ('Ohrid', 'ohrid', '310'),
    ('Pehčevo', 'pehčevo', '208'),
    ('Petrovec', 'petrovec', '810'),
    ('Plasnica', 'plasnica', '311'),
    ('Prilep', 'prilep', '508'),
    ('Probištip', 'probištip', '209'),
    ('Radoviš', 'radoviš', '409'),
    ('Rankovce', 'rankovce', '705'),
    ('Resen', 'resen', '509'),
    ('Rosoman', 'rosoman', '107'),
    ('Saraj', 'saraj', '811'),
    ('Sopište', 'sopište', '812'),
    ('Staro Nagoričane', 'staro-nagoričane', '706'),
    ('Štip', 'štip', '211'),
    ('Struga', 'struga', '312'),
    ('Strumica', 'strumica', '410'),
    ('Studeničani', 'studeničani', '813'),
    ('Šuto Orizari', 'šuto-orizari', '817'),
    ('Sveti Nikole', 'sveti-nikole', '108'),
    ('Tearce', 'tearce', '608'),
    ('Tetovo', 'tetovo', '609'),
    ('Valandovo', 'valandovo', '403'),
    ('Vasilevo', 'vasilevo', '404'),
    ('Veles', 'veles', '101'),
    ('Vevčani', 'vevčani', '301'),
    ('Vinica', 'vinica', '202'),
    ('Vrapčište', 'vrapčište', '603'),
    ('Zelenikovo', 'zelenikovo', '806'),
    ('Želino', 'želino', '605'),
    ('Zrnovci', 'zrnovci', '204')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'north-macedonia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Norway (NO) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'no-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Agder', 'agder', '42'),
    ('Akershus', 'akershus', '32'),
    ('Buskerud', 'buskerud', '33'),
    ('Finnmark', 'finnmark', '56'),
    ('Innlandet', 'innlandet', '34'),
    ('Jan Mayen', 'jan-mayen', '22'),
    ('Møre og Romsdal', 'møre-og-romsdal', '15'),
    ('Nordland', 'nordland', '18'),
    ('Oslo', 'oslo', '03'),
    ('Østfold', 'østfold', '31'),
    ('Rogaland', 'rogaland', '11'),
    ('Svalbard', 'svalbard', '21'),
    ('Telemark', 'telemark', '40'),
    ('Troms', 'troms', '55'),
    ('Trøndelag', 'trøndelag', '50'),
    ('Vestfold', 'vestfold', '39'),
    ('Vestland', 'vestland', '46')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'norway' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'no-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Oman (OM) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'om-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ad Dakhiliyah', 'ad-dakhiliyah', 'DA'),
    ('Ad Dhahirah', 'ad-dhahirah', 'ZA'),
    ('Al Batinah North', 'al-batinah-north', 'BS'),
    ('Al Batinah South', 'al-batinah-south', 'BJ'),
    ('Al Buraimi', 'al-buraimi', 'BU'),
    ('Al Wusta', 'al-wusta', 'WU'),
    ('Ash Sharqiyah North', 'ash-sharqiyah-north', 'SS'),
    ('Ash Sharqiyah South', 'ash-sharqiyah-south', 'SJ'),
    ('Dhofar', 'dhofar', 'ZU'),
    ('Musandam', 'musandam', 'MU'),
    ('Muscat', 'muscat', 'MA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'oman' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'om-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Pakistan (PK) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Azad Kashmir', 'azad-kashmir', 'JK'),
    ('Balochistan', 'balochistan', 'BA'),
    ('Gilgit-Baltistan', 'gilgit-baltistan', 'GB'),
    ('Islamabad', 'islamabad', 'IS'),
    ('Khyber Pakhtunkhwa', 'khyber-pakhtunkhwa', 'KP'),
    ('Punjab', 'punjab', 'PB'),
    ('Sindh', 'sindh', 'SD')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'pakistan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Palau (PW) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aimeliik', 'aimeliik', '002'),
    ('Airai', 'airai', '004'),
    ('Angaur', 'angaur', '010'),
    ('Hatohobei', 'hatohobei', '050'),
    ('Kayangel', 'kayangel', '100'),
    ('Koror', 'koror', '150'),
    ('Melekeok', 'melekeok', '212'),
    ('Ngaraard', 'ngaraard', '214'),
    ('Ngarchelong', 'ngarchelong', '218'),
    ('Ngardmau', 'ngardmau', '222'),
    ('Ngatpang', 'ngatpang', '224'),
    ('Ngchesar', 'ngchesar', '226'),
    ('Ngeremlengui', 'ngeremlengui', '227'),
    ('Ngiwal', 'ngiwal', '228'),
    ('Peleliu', 'peleliu', '350'),
    ('Sonsorol', 'sonsorol', '370')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'palau' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Palestinian Territory Occupied (PS) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ps-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bethlehem', 'bethlehem', 'BTH'),
    ('Deir El Balah', 'deir-el-balah', 'DEB'),
    ('Gaza', 'gaza', 'GZA'),
    ('Hebron', 'hebron', 'HBN'),
    ('Jenin', 'jenin', 'JEN'),
    ('Jericho ', 'jericho', 'JRH'),
    ('Jerusalem (Quds)', 'jerusalem-quds', 'JEM'),
    ('Khan Yunis', 'khan-yunis', 'KYS'),
    ('Nablus', 'nablus', 'NBS'),
    ('North Gaza', 'north-gaza', 'NGZ'),
    ('Qalqilya', 'qalqilya', 'QQA'),
    ('Rafah', 'rafah', 'RFH'),
    ('Ramallah', 'ramallah', 'RBH'),
    ('Salfit', 'salfit', 'SLT'),
    ('Tubas', 'tubas', 'TBS'),
    ('Tulkarm', 'tulkarm', 'TKM')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'palestinian-territory-occupied' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ps-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Panama (PA) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pa-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bocas del Toro', 'bocas-del-toro', '1'),
    ('Chiriquí Province', 'chiriquí-province', '4'),
    ('Coclé', 'coclé', '2'),
    ('Colón', 'colón', '3'),
    ('Darién', 'darién', '5'),
    ('Emberá-Wounaan Comarca', 'emberá-wounaan-comarca', 'EM'),
    ('Guna', 'guna', 'KY'),
    ('Herrera', 'herrera', '6'),
    ('Los Santos', 'los-santos', '7'),
    ('Naso Tjër Di', 'naso-tjër-di', 'NT'),
    ('Ngöbe-Buglé Comarca', 'ngöbe-buglé-comarca', 'NB'),
    ('Panamá', 'panamá', '8'),
    ('Panamá Oeste', 'panamá-oeste', '10'),
    ('Veraguas', 'veraguas', '9')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'panama' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pa-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Papua New Guinea (PG) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bougainville', 'bougainville', 'NSB'),
    ('Central', 'central', 'CPM'),
    ('Chimbu', 'chimbu', 'CPK'),
    ('East New Britain', 'east-new-britain', 'EBR'),
    ('East Sepik', 'east-sepik', 'ESW'),
    ('Eastern Highlands', 'eastern-highlands', 'EHG'),
    ('Enga', 'enga', 'EPW'),
    ('Gulf', 'gulf', 'GPK'),
    ('Hela', 'hela', 'HLA'),
    ('Jiwaka', 'jiwaka', 'JWK'),
    ('Madang', 'madang', 'MPM'),
    ('Manus', 'manus', 'MRL'),
    ('Milne Bay', 'milne-bay', 'MBA'),
    ('Morobe', 'morobe', 'MPL'),
    ('New Ireland', 'new-ireland', 'NIK'),
    ('Oro', 'oro', 'NPP'),
    ('Port Moresby', 'port-moresby', 'NCD'),
    ('Sandaun', 'sandaun', 'SAN'),
    ('Southern Highlands', 'southern-highlands', 'SHM'),
    ('West New Britain', 'west-new-britain', 'WBK'),
    ('Western', 'western', 'WPD'),
    ('Western Highlands', 'western-highlands', 'WHM')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'papua-new-guinea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Paraguay (PY) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'py-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alto Paraguay', 'alto-paraguay', '16'),
    ('Alto Paraná', 'alto-paraná', '10'),
    ('Amambay', 'amambay', '13'),
    ('Asuncion', 'asuncion', 'ASU'),
    ('Boquerón', 'boquerón', '19'),
    ('Caaguazú', 'caaguazú', '5'),
    ('Caazapá', 'caazapá', '6'),
    ('Canindeyú', 'canindeyú', '14'),
    ('Central', 'central', '11'),
    ('Concepción', 'concepción', '1'),
    ('Cordillera', 'cordillera', '3'),
    ('Guairá', 'guairá', '4'),
    ('Itapúa', 'itapúa', '7'),
    ('Misiones', 'misiones', '8'),
    ('Ñeembucú', 'ñeembucú', '12'),
    ('Paraguarí', 'paraguarí', '9'),
    ('Presidente Hayes', 'presidente-hayes', '15'),
    ('San Pedro', 'san-pedro', '2')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'paraguay' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'py-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Peru (PE) — 26 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pe-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Amazonas', 'amazonas', 'AMA'),
    ('Áncash', 'áncash', 'ANC'),
    ('Apurímac', 'apurímac', 'APU'),
    ('Arequipa', 'arequipa', 'ARE'),
    ('Ayacucho', 'ayacucho', 'AYA'),
    ('Cajamarca', 'cajamarca', 'CAJ'),
    ('Callao', 'callao', 'CAL'),
    ('Cusco', 'cusco', 'CUS'),
    ('Huancavelica', 'huancavelica', 'HUV'),
    ('Huanuco', 'huanuco', 'HUC'),
    ('Ica', 'ica', 'ICA'),
    ('Junín', 'junín', 'JUN'),
    ('La Libertad', 'la-libertad', 'LAL'),
    ('Lambayeque', 'lambayeque', 'LAM'),
    ('Lima', 'lima', 'LIM'),
    ('Loreto', 'loreto', 'LOR'),
    ('Madre de Dios', 'madre-de-dios', 'MDD'),
    ('Moquegua', 'moquegua', 'MOQ'),
    ('Municipalidad Metropolitana de Lima', 'municipalidad-metropolitana-de-lima', 'LMA'),
    ('Pasco', 'pasco', 'PAS'),
    ('Piura', 'piura', 'PIU'),
    ('Puno', 'puno', 'PUN'),
    ('San Martín', 'san-martín', 'SAM'),
    ('Tacna', 'tacna', 'TAC'),
    ('Tumbes', 'tumbes', 'TUM'),
    ('Ucayali', 'ucayali', 'UCA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'peru' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pe-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Philippines (PH) — 99 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ph-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abra', 'abra', 'ABR'),
    ('Agusan del Norte', 'agusan-del-norte', 'AGN'),
    ('Agusan del Sur', 'agusan-del-sur', 'AGS'),
    ('Aklan', 'aklan', 'AKL'),
    ('Albay', 'albay', 'ALB'),
    ('Antique', 'antique', 'ANT'),
    ('Apayao', 'apayao', 'APA'),
    ('Aurora', 'aurora', 'AUR'),
    ('Autonomous Region in Muslim Mindanao', 'autonomous-region-in-muslim-mindanao', '14'),
    ('Basilan', 'basilan', 'BAS'),
    ('Bataan', 'bataan', 'BAN'),
    ('Batanes', 'batanes', 'BTN'),
    ('Batangas', 'batangas', 'BTG'),
    ('Benguet', 'benguet', 'BEN'),
    ('Bicol', 'bicol', '05'),
    ('Biliran', 'biliran', 'BIL'),
    ('Bohol', 'bohol', 'BOH'),
    ('Bukidnon', 'bukidnon', 'BUK'),
    ('Bulacan', 'bulacan', 'BUL'),
    ('Cagayan', 'cagayan', 'CAG'),
    ('Cagayan Valley', 'cagayan-valley', '02'),
    ('Calabarzon', 'calabarzon', '40'),
    ('Camarines Norte', 'camarines-norte', 'CAN'),
    ('Camarines Sur', 'camarines-sur', 'CAS'),
    ('Camiguin', 'camiguin', 'CAM'),
    ('Capiz', 'capiz', 'CAP'),
    ('Caraga', 'caraga', '13'),
    ('Catanduanes', 'catanduanes', 'CAT'),
    ('Cavite', 'cavite', 'CAV'),
    ('Cebu', 'cebu', 'CEB'),
    ('Central Luzon', 'central-luzon', '03'),
    ('Central Visayas', 'central-visayas', '07'),
    ('Cordillera Administrative', 'cordillera-administrative', '15'),
    ('Cotabato', 'cotabato', 'NCO'),
    ('Davao', 'davao', '11'),
    ('Davao de Oro', 'davao-de-oro', 'COM'),
    ('Davao del Norte', 'davao-del-norte', 'DAV'),
    ('Davao del Sur', 'davao-del-sur', 'DAS'),
    ('Davao Occidental', 'davao-occidental', 'DVO'),
    ('Davao Oriental', 'davao-oriental', 'DAO'),
    ('Dinagat Islands', 'dinagat-islands', 'DIN'),
    ('Eastern Samar', 'eastern-samar', 'EAS'),
    ('Eastern Visayas', 'eastern-visayas', '08'),
    ('Guimaras', 'guimaras', 'GUI'),
    ('Ifugao', 'ifugao', 'IFU'),
    ('Ilocos', 'ilocos', '01'),
    ('Ilocos Norte', 'ilocos-norte', 'ILN'),
    ('Ilocos Sur', 'ilocos-sur', 'ILS'),
    ('Iloilo', 'iloilo', 'ILI'),
    ('Isabela', 'isabela', 'ISA'),
    ('Kalinga', 'kalinga', 'KAL'),
    ('La Union', 'la-union', 'LUN'),
    ('Laguna', 'laguna', 'LAG'),
    ('Lanao del Norte', 'lanao-del-norte', 'LAN'),
    ('Lanao del Sur', 'lanao-del-sur', 'LAS'),
    ('Leyte', 'leyte', 'LEY'),
    ('Maguindanao del Norte', 'maguindanao-del-norte', 'MGN'),
    ('Maguindanao del Sur', 'maguindanao-del-sur', 'MGS'),
    ('Marinduque', 'marinduque', 'MAD'),
    ('Masbate', 'masbate', 'MAS'),
    ('Mimaropa', 'mimaropa', '41'),
    ('Misamis Occidental', 'misamis-occidental', 'MSC'),
    ('Misamis Oriental', 'misamis-oriental', 'MSR'),
    ('Mountain Province', 'mountain-province', 'MOU'),
    ('National Capital Region (Metro Manila)', 'national-capital-region-metro-manila', '00'),
    ('Negros Occidental', 'negros-occidental', 'NEC'),
    ('Negros Oriental', 'negros-oriental', 'NER'),
    ('Northern Mindanao', 'northern-mindanao', '10'),
    ('Northern Samar', 'northern-samar', 'NSA'),
    ('Nueva Ecija', 'nueva-ecija', 'NUE'),
    ('Nueva Vizcaya', 'nueva-vizcaya', 'NUV'),
    ('Occidental Mindoro', 'occidental-mindoro', 'MDC'),
    ('Oriental Mindoro', 'oriental-mindoro', 'MDR'),
    ('Palawan', 'palawan', 'PLW'),
    ('Pampanga', 'pampanga', 'PAM'),
    ('Pangasinan', 'pangasinan', 'PAN'),
    ('Quezon', 'quezon', 'QUE'),
    ('Quirino', 'quirino', 'QUI'),
    ('Rizal', 'rizal', 'RIZ'),
    ('Romblon', 'romblon', 'ROM'),
    ('Sarangani', 'sarangani', 'SAR'),
    ('Siquijor', 'siquijor', 'SIG'),
    ('Soccsksargen', 'soccsksargen', '12'),
    ('Sorsogon', 'sorsogon', 'SOR'),
    ('South Cotabato', 'south-cotabato', 'SCO'),
    ('Southern Leyte', 'southern-leyte', 'SLE'),
    ('Sultan Kudarat', 'sultan-kudarat', 'SUK'),
    ('Sulu', 'sulu', 'SLU'),
    ('Surigao del Norte', 'surigao-del-norte', 'SUN'),
    ('Surigao del Sur', 'surigao-del-sur', 'SUR'),
    ('Tarlac', 'tarlac', 'TAR'),
    ('Tawi-Tawi', 'tawi-tawi', 'TAW'),
    ('Western Samar', 'western-samar', 'WSA'),
    ('Western Visayas', 'western-visayas', '06'),
    ('Zambales', 'zambales', 'ZMB'),
    ('Zamboanga del Norte', 'zamboanga-del-norte', 'ZAN'),
    ('Zamboanga del Sur', 'zamboanga-del-sur', 'ZAS'),
    ('Zamboanga Peninsula', 'zamboanga-peninsula', '09'),
    ('Zamboanga Sibugay', 'zamboanga-sibugay', 'ZSI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'philippines' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ph-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Poland (PL) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Greater Poland', 'greater-poland', '30'),
    ('Holy Cross', 'holy-cross', '26'),
    ('Kuyavia-Pomerania', 'kuyavia-pomerania', '04'),
    ('Lesser Poland', 'lesser-poland', '12'),
    ('Lower Silesia', 'lower-silesia', '02'),
    ('Lublin', 'lublin', '06'),
    ('Lubusz', 'lubusz', '08'),
    ('Łódź', 'łódź', '10'),
    ('Mazovia', 'mazovia', '14'),
    ('Podlaskie', 'podlaskie', '20'),
    ('Pomerania', 'pomerania', '22'),
    ('Silesia', 'silesia', '24'),
    ('Subcarpathia', 'subcarpathia', '18'),
    ('Upper Silesia', 'upper-silesia', '16'),
    ('Warmia-Masuria', 'warmia-masuria', '28'),
    ('West Pomerania', 'west-pomerania', '32')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'poland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Portugal (PT) — 20 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Açores', 'açores', '20'),
    ('Aveiro', 'aveiro', '01'),
    ('Beja', 'beja', '02'),
    ('Braga', 'braga', '03'),
    ('Bragança', 'bragança', '04'),
    ('Castelo Branco', 'castelo-branco', '05'),
    ('Coimbra', 'coimbra', '06'),
    ('Évora', 'évora', '07'),
    ('Faro', 'faro', '08'),
    ('Guarda', 'guarda', '09'),
    ('Leiria', 'leiria', '10'),
    ('Lisbon', 'lisbon', '11'),
    ('Madeira', 'madeira', '30'),
    ('Portalegre', 'portalegre', '12'),
    ('Porto', 'porto', '13'),
    ('Santarém', 'santarém', '14'),
    ('Setúbal', 'setúbal', '15'),
    ('Viana do Castelo', 'viana-do-castelo', '16'),
    ('Vila Real', 'vila-real', '17'),
    ('Viseu', 'viseu', '18')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'portugal' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Puerto Rico (PR) — 78 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adjuntas', 'adjuntas', '001'),
    ('Aguada', 'aguada', '003'),
    ('Aguadilla', 'aguadilla', '005'),
    ('Aguas Buenas', 'aguas-buenas', '007'),
    ('Aibonito', 'aibonito', '009'),
    ('Añasco', 'añasco', '011'),
    ('Arecibo', 'arecibo', 'AR'),
    ('Arroyo', 'arroyo', '015'),
    ('Barceloneta', 'barceloneta', '017'),
    ('Barranquitas', 'barranquitas', '019'),
    ('Bayamon', 'bayamon', 'BY'),
    ('Cabo Rojo', 'cabo-rojo', '023'),
    ('Caguas', 'caguas', 'CG'),
    ('Camuy', 'camuy', '027'),
    ('Canóvanas', 'canóvanas', '029'),
    ('Carolina', 'carolina', 'CL'),
    ('Cataño', 'cataño', '033'),
    ('Cayey', 'cayey', '035'),
    ('Ceiba', 'ceiba', '037'),
    ('Ciales', 'ciales', '039'),
    ('Cidra', 'cidra', '041'),
    ('Coamo', 'coamo', '043'),
    ('Comerío', 'comerío', '045'),
    ('Corozal', 'corozal', '047'),
    ('Culebra', 'culebra', '049'),
    ('Dorado', 'dorado', '051'),
    ('Fajardo', 'fajardo', '053'),
    ('Florida', 'florida', '054'),
    ('Guánica', 'guánica', '055'),
    ('Guayama', 'guayama', '057'),
    ('Guayanilla', 'guayanilla', '059'),
    ('Guaynabo', 'guaynabo', 'GN'),
    ('Gurabo', 'gurabo', '063'),
    ('Hatillo', 'hatillo', '065'),
    ('Hormigueros', 'hormigueros', '067'),
    ('Humacao', 'humacao', '069'),
    ('Isabela', 'isabela', '071'),
    ('Jayuya', 'jayuya', '073'),
    ('Juana Díaz', 'juana-díaz', '075'),
    ('Juncos', 'juncos', '077'),
    ('Lajas', 'lajas', '079'),
    ('Lares', 'lares', '081'),
    ('Las Marías', 'las-marías', '083'),
    ('Las Piedras', 'las-piedras', '085'),
    ('Loíza', 'loíza', '087'),
    ('Luquillo', 'luquillo', '089'),
    ('Manatí', 'manatí', '091'),
    ('Maricao', 'maricao', '093'),
    ('Maunabo', 'maunabo', '095'),
    ('Mayagüez', 'mayagüez', 'MG'),
    ('Moca', 'moca', '099'),
    ('Morovis', 'morovis', '101'),
    ('Naguabo', 'naguabo', '103'),
    ('Naranjito', 'naranjito', '105'),
    ('Orocovis', 'orocovis', '107'),
    ('Patillas', 'patillas', '109'),
    ('Peñuelas', 'peñuelas', '111'),
    ('Ponce', 'ponce', 'PO'),
    ('Quebradillas', 'quebradillas', '115'),
    ('Rincón', 'rincón', '117'),
    ('Río Grande', 'río-grande', '119'),
    ('Sabana Grande', 'sabana-grande', '121'),
    ('Salinas', 'salinas', '123'),
    ('San Germán', 'san-germán', '125'),
    ('San Juan', 'san-juan', 'SJ'),
    ('San Lorenzo', 'san-lorenzo', '129'),
    ('San Sebastián', 'san-sebastián', '131'),
    ('Santa Isabel', 'santa-isabel', '133'),
    ('Toa Alta', 'toa-alta', '135'),
    ('Toa Baja', 'toa-baja', 'TB'),
    ('Trujillo Alto', 'trujillo-alto', 'TA'),
    ('Utuado', 'utuado', '141'),
    ('Vega Alta', 'vega-alta', '143'),
    ('Vega Baja', 'vega-baja', '145'),
    ('Vieques', 'vieques', '147'),
    ('Villalba', 'villalba', '149'),
    ('Yabucoa', 'yabucoa', '151'),
    ('Yauco', 'yauco', '153')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'puerto-rico' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Qatar (QA) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'qa-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Daayen', 'al-daayen', 'ZA'),
    ('Al Khor', 'al-khor', 'KH'),
    ('Al Rayyan', 'al-rayyan', 'RA'),
    ('Al Wakrah', 'al-wakrah', 'WA'),
    ('Al-Shahaniya', 'al-shahaniya', 'SH'),
    ('Doha', 'doha', 'DA'),
    ('Madinat ash Shamal', 'madinat-ash-shamal', 'MS'),
    ('Umm Salal', 'umm-salal', 'US')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'qatar' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'qa-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Reunion (RE) — 4 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    're-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint-Benoît', 'saint-benoît', '01'),
    ('Saint-Denis', 'saint-denis', '02'),
    ('Saint-Paul', 'saint-paul', '03'),
    ('Saint-Pierre', 'saint-pierre', '04')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'reunion' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 're-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Romania (RO) — 42 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ro-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alba', 'alba', 'AB'),
    ('Arad', 'arad', 'AR'),
    ('Arges', 'arges', 'AG'),
    ('Bacău', 'bacău', 'BC'),
    ('Bihor', 'bihor', 'BH'),
    ('Bistrița-Năsăud', 'bistrița-năsăud', 'BN'),
    ('Botoșani', 'botoșani', 'BT'),
    ('Braila', 'braila', 'BR'),
    ('Brașov', 'brașov', 'BV'),
    ('Bucharest', 'bucharest', 'B'),
    ('Buzău', 'buzău', 'BZ'),
    ('Călărași', 'călărași', 'CL'),
    ('Caraș-Severin', 'caraș-severin', 'CS'),
    ('Cluj', 'cluj', 'CJ'),
    ('Constanța', 'constanța', 'CT'),
    ('Covasna', 'covasna', 'CV'),
    ('Dâmbovița', 'dâmbovița', 'DB'),
    ('Dolj', 'dolj', 'DJ'),
    ('Galați', 'galați', 'GL'),
    ('Giurgiu', 'giurgiu', 'GR'),
    ('Gorj', 'gorj', 'GJ'),
    ('Harghita', 'harghita', 'HR'),
    ('Hunedoara', 'hunedoara', 'HD'),
    ('Ialomița', 'ialomița', 'IL'),
    ('Iași', 'iași', 'IS'),
    ('Ilfov', 'ilfov', 'IF'),
    ('Maramureș', 'maramureș', 'MM'),
    ('Mehedinți', 'mehedinți', 'MH'),
    ('Mureș', 'mureș', 'MS'),
    ('Neamț', 'neamț', 'NT'),
    ('Olt', 'olt', 'OT'),
    ('Prahova', 'prahova', 'PH'),
    ('Sălaj', 'sălaj', 'SJ'),
    ('Satu Mare', 'satu-mare', 'SM'),
    ('Sibiu', 'sibiu', 'SB'),
    ('Suceava', 'suceava', 'SV'),
    ('Teleorman', 'teleorman', 'TR'),
    ('Timiș', 'timiș', 'TM'),
    ('Tulcea', 'tulcea', 'TL'),
    ('Vâlcea', 'vâlcea', 'VL'),
    ('Vaslui', 'vaslui', 'VS'),
    ('Vrancea', 'vrancea', 'VN')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'romania' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ro-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Russia (RU) — 83 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ru-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adygea', 'adygea', 'AD'),
    ('Altai', 'altai', 'ALT'),
    ('Altai', 'altai', 'AL'),
    ('Amur', 'amur', 'AMU'),
    ('Arkhangelsk', 'arkhangelsk', 'ARK'),
    ('Astrakhan', 'astrakhan', 'AST'),
    ('Bashkortostan', 'bashkortostan', 'BA'),
    ('Belgorod', 'belgorod', 'BEL'),
    ('Bryansk', 'bryansk', 'BRY'),
    ('Buryatia', 'buryatia', 'BU'),
    ('Chechen', 'chechen', 'CE'),
    ('Chelyabinsk', 'chelyabinsk', 'CHE'),
    ('Chukotka', 'chukotka', 'CHU'),
    ('Chuvash', 'chuvash', 'CU'),
    ('Dagestan', 'dagestan', 'DA'),
    ('Ingushetia', 'ingushetia', 'IN'),
    ('Irkutsk', 'irkutsk', 'IRK'),
    ('Ivanovo', 'ivanovo', 'IVA'),
    ('Jewish', 'jewish', 'YEV'),
    ('Kabardino-Balkar', 'kabardino-balkar', 'KB'),
    ('Kaliningrad', 'kaliningrad', 'KGD'),
    ('Kalmykia', 'kalmykia', 'KL'),
    ('Kaluga', 'kaluga', 'KLU'),
    ('Kamchatka', 'kamchatka', 'KAM'),
    ('Karachay-Cherkess', 'karachay-cherkess', 'KC'),
    ('Karelia', 'karelia', 'KR'),
    ('Kemerovo', 'kemerovo', 'KEM'),
    ('Khabarovsk', 'khabarovsk', 'KHA'),
    ('Khakassia', 'khakassia', 'KK'),
    ('Khanty-Mansi', 'khanty-mansi', 'KHM'),
    ('Kirov', 'kirov', 'KIR'),
    ('Komi', 'komi', 'KO'),
    ('Kostroma', 'kostroma', 'KOS'),
    ('Krasnodar', 'krasnodar', 'KDA'),
    ('Krasnoyarsk', 'krasnoyarsk', 'KYA'),
    ('Kurgan', 'kurgan', 'KGN'),
    ('Kursk', 'kursk', 'KRS'),
    ('Leningrad', 'leningrad', 'LEN'),
    ('Lipetsk', 'lipetsk', 'LIP'),
    ('Magadan', 'magadan', 'MAG'),
    ('Mari El', 'mari-el', 'ME'),
    ('Mordovia', 'mordovia', 'MO'),
    ('Moscow', 'moscow', 'MOS'),
    ('Moscow', 'moscow', 'MOW'),
    ('Murmansk', 'murmansk', 'MUR'),
    ('Nenets', 'nenets', 'NEN'),
    ('Nizhny Novgorod', 'nizhny-novgorod', 'NIZ'),
    ('North Ossetia-Alania', 'north-ossetia-alania', 'SE'),
    ('Novgorod', 'novgorod', 'NGR'),
    ('Novosibirsk', 'novosibirsk', 'NVS'),
    ('Omsk', 'omsk', 'OMS'),
    ('Orenburg', 'orenburg', 'ORE'),
    ('Oryol', 'oryol', 'ORL'),
    ('Penza', 'penza', 'PNZ'),
    ('Perm', 'perm', 'PER'),
    ('Primorsky', 'primorsky', 'PRI'),
    ('Pskov', 'pskov', 'PSK'),
    ('Rostov', 'rostov', 'ROS'),
    ('Ryazan', 'ryazan', 'RYA'),
    ('Saint Petersburg', 'saint-petersburg', 'SPE'),
    ('Sakha', 'sakha', 'SA'),
    ('Sakhalin', 'sakhalin', 'SAK'),
    ('Samara', 'samara', 'SAM'),
    ('Saratov', 'saratov', 'SAR'),
    ('Smolensk', 'smolensk', 'SMO'),
    ('Stavropol', 'stavropol', 'STA'),
    ('Sverdlovsk', 'sverdlovsk', 'SVE'),
    ('Tambov', 'tambov', 'TAM'),
    ('Tatarstan', 'tatarstan', 'TA'),
    ('Tomsk', 'tomsk', 'TOM'),
    ('Tula', 'tula', 'TUL'),
    ('Tuva', 'tuva', 'TY'),
    ('Tver', 'tver', 'TVE'),
    ('Tyumen', 'tyumen', 'TYU'),
    ('Udmurt', 'udmurt', 'UD'),
    ('Ulyanovsk', 'ulyanovsk', 'ULY'),
    ('Vladimir', 'vladimir', 'VLA'),
    ('Volgograd Oblast', 'volgograd-oblast', 'VGG'),
    ('Vologda', 'vologda', 'VLG'),
    ('Voronezh', 'voronezh', 'VOR'),
    ('Yamalo-Nenets', 'yamalo-nenets', 'YAN'),
    ('Yaroslavl', 'yaroslavl', 'YAR'),
    ('Zabaykalsky', 'zabaykalsky', 'ZAB')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'russia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ru-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Rwanda (RW) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'rw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Eastern', 'eastern', '02'),
    ('Kigali', 'kigali', '01'),
    ('Northern', 'northern', '03'),
    ('Southern', 'southern', '05'),
    ('Western', 'western', '04')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'rwanda' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'rw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint Helena (SH) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sh-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alarm Forest', 'alarm-forest', '01'),
    ('Blue Hill', 'blue-hill', '02'),
    ('Half Tree Hollow', 'half-tree-hollow', '03'),
    ('Jamestown', 'jamestown', '04'),
    ('Levelwood', 'levelwood', '05'),
    ('Longwood', 'longwood', '06'),
    ('Saint Paul''s', 'saint-pauls', '08'),
    ('Sandy Bay', 'sandy-bay', '07')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-helena' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sh-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint Kitts and Nevis (KN) — 16 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Christ Church Nichola Town', 'christ-church-nichola-town', '01'),
    ('Nevis', 'nevis', 'N'),
    ('Saint Anne Sandy Point', 'saint-anne-sandy-point', '02'),
    ('Saint George Basseterre', 'saint-george-basseterre', '03'),
    ('Saint George Gingerland', 'saint-george-gingerland', '04'),
    ('Saint James Windward', 'saint-james-windward', '05'),
    ('Saint John Capisterre', 'saint-john-capisterre', '06'),
    ('Saint John Figtree', 'saint-john-figtree', '07'),
    ('Saint Kitts', 'saint-kitts', 'K'),
    ('Saint Mary Cayon', 'saint-mary-cayon', '08'),
    ('Saint Paul Capisterre', 'saint-paul-capisterre', '09'),
    ('Saint Paul Charlestown', 'saint-paul-charlestown', '10'),
    ('Saint Peter Basseterre', 'saint-peter-basseterre', '11'),
    ('Saint Thomas Lowland', 'saint-thomas-lowland', '12'),
    ('Saint Thomas Middle Island', 'saint-thomas-middle-island', '13'),
    ('Trinity Palmetto Point', 'trinity-palmetto-point', '15')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-kitts-and-nevis' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint Lucia (LC) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anse la Raye', 'anse-la-raye', '01'),
    ('Canaries', 'canaries', '12'),
    ('Castries', 'castries', '02'),
    ('Choiseul', 'choiseul', '03'),
    ('Dennery', 'dennery', '05'),
    ('Gros Islet', 'gros-islet', '06'),
    ('Laborie', 'laborie', '07'),
    ('Micoud', 'micoud', '08'),
    ('Soufrière', 'soufrière', '10'),
    ('Vieux Fort', 'vieux-fort', '11')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-lucia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint Pierre and Miquelon (PM) — 1 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'pm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint-Pierre and Miquelon', 'saint-pierre-and-miquelon', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-pierre-and-miquelon' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'pm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint Vincent and the Grenadines (VC) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'vc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Charlotte', 'charlotte', '01'),
    ('Grenadines', 'grenadines', '06'),
    ('Saint Andrew', 'saint-andrew', '02'),
    ('Saint David', 'saint-david', '03'),
    ('Saint George', 'saint-george', '04'),
    ('Saint Patrick', 'saint-patrick', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-vincent-and-the-grenadines' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'vc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint-Barthelemy (BL) — 1 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint-Barthélemy', 'saint-barthélemy', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-barthelemy' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saint-Martin (French part) (MF) — 1 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'mf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint-Martin', 'saint-martin', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saint-martin-french-part' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'mf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Samoa (WS) — 11 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ws-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('A''ana', 'aana', 'AA'),
    ('Aiga-i-le-Tai', 'aiga-i-le-tai', 'AL'),
    ('Atua', 'atua', 'AT'),
    ('Fa''asaleleaga', 'faasaleleaga', 'FA'),
    ('Gaga''emauga', 'gagaemauga', 'GE'),
    ('Gaga''ifomauga', 'gagaifomauga', 'GI'),
    ('Palauli', 'palauli', 'PA'),
    ('Satupa''itea', 'satupaitea', 'SA'),
    ('Tuamasaga', 'tuamasaga', 'TU'),
    ('Va''a-o-Fonoti', 'vaa-o-fonoti', 'VF'),
    ('Vaisigano', 'vaisigano', 'VS')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'samoa' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ws-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- San Marino (SM) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Acquaviva', 'acquaviva', '01'),
    ('Borgo Maggiore', 'borgo-maggiore', '06'),
    ('Chiesanuova', 'chiesanuova', '02'),
    ('Domagnano', 'domagnano', '03'),
    ('Faetano', 'faetano', '04'),
    ('Fiorentino', 'fiorentino', '05'),
    ('Montegiardino', 'montegiardino', '08'),
    ('San Marino', 'san-marino', '07'),
    ('Serravalle', 'serravalle', '09')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'san-marino' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Sao Tome and Principe (ST) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'st-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Água Grande', 'água-grande', '01'),
    ('Cantagalo', 'cantagalo', '02'),
    ('Caué', 'caué', '03'),
    ('Lemba', 'lemba', '04'),
    ('Lobata', 'lobata', '05'),
    ('Mé-Zóchi', 'mé-zóchi', '06'),
    ('Príncipe', 'príncipe', 'P')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'sao-tome-and-principe' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'st-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Saudi Arabia (SA) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sa-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Bahah', 'al-bahah', '11'),
    ('Al Jawf', 'al-jawf', '12'),
    ('Al Madinah', 'al-madinah', '03'),
    ('Al-Qassim', 'al-qassim', '05'),
    ('Asir', 'asir', '14'),
    ('Eastern Province', 'eastern-province', '04'),
    ('Ha''il', 'hail', '06'),
    ('Jizan', 'jizan', '09'),
    ('Makkah', 'makkah', '02'),
    ('Najran', 'najran', '10'),
    ('Northern Borders', 'northern-borders', '08'),
    ('Riyadh', 'riyadh', '01'),
    ('Tabuk', 'tabuk', '07')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'saudi-arabia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sa-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Senegal (SN) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Dakar', 'dakar', 'DK'),
    ('Diourbel Region', 'diourbel-region', 'DB'),
    ('Fatick', 'fatick', 'FK'),
    ('Kaffrine', 'kaffrine', 'KA'),
    ('Kaolack', 'kaolack', 'KL'),
    ('Kédougou', 'kédougou', 'KE'),
    ('Kolda', 'kolda', 'KD'),
    ('Louga', 'louga', 'LG'),
    ('Matam', 'matam', 'MT'),
    ('Saint-Louis', 'saint-louis', 'SL'),
    ('Sédhiou', 'sédhiou', 'SE'),
    ('Tambacounda Region', 'tambacounda-region', 'TC'),
    ('Thiès Region', 'thiès-region', 'TH'),
    ('Ziguinchor', 'ziguinchor', 'ZG')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'senegal' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Serbia (RS) — 32 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'rs-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Belgrade', 'belgrade', '00'),
    ('Bor', 'bor', '14'),
    ('Braničevo', 'braničevo', '11'),
    ('Central Banat', 'central-banat', '02'),
    ('Jablanica', 'jablanica', '23'),
    ('Kolubara', 'kolubara', '09'),
    ('Kosovo', 'kosovo', '25'),
    ('Kosovo-Metohija', 'kosovo-metohija', 'KM'),
    ('Kosovo-Pomoravlje', 'kosovo-pomoravlje', '29'),
    ('Kosovska Mitrovica', 'kosovska-mitrovica', '28'),
    ('Mačva', 'mačva', '08'),
    ('Moravica', 'moravica', '17'),
    ('Nišava', 'nišava', '20'),
    ('North Bačka', 'north-bačka', '01'),
    ('North Banat', 'north-banat', '03'),
    ('Pčinja', 'pčinja', '24'),
    ('Peć', 'peć', '26'),
    ('Pirot', 'pirot', '22'),
    ('Podunavlje', 'podunavlje', '10'),
    ('Pomoravlje', 'pomoravlje', '13'),
    ('Prizren', 'prizren', '27'),
    ('Rasina', 'rasina', '19'),
    ('Raška', 'raška', '18'),
    ('South Bačka', 'south-bačka', '06'),
    ('South Banat', 'south-banat', '04'),
    ('Srem', 'srem', '07'),
    ('Šumadija', 'šumadija', '12'),
    ('Toplica', 'toplica', '21'),
    ('Vojvodina', 'vojvodina', 'VO'),
    ('West Bačka', 'west-bačka', '05'),
    ('Zaječar', 'zaječar', '15'),
    ('Zlatibor', 'zlatibor', '16')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'serbia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'rs-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Seychelles (SC) — 27 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Anse Boileau', 'anse-boileau', '02'),
    ('Anse Etoile', 'anse-etoile', '03'),
    ('Anse Royale', 'anse-royale', '05'),
    ('Anse-aux-Pins', 'anse-aux-pins', '01'),
    ('Au Cap', 'au-cap', '04'),
    ('Baie Lazare', 'baie-lazare', '06'),
    ('Baie Sainte Anne', 'baie-sainte-anne', '07'),
    ('Beau Vallon', 'beau-vallon', '08'),
    ('Bel Air', 'bel-air', '09'),
    ('Bel Ombre', 'bel-ombre', '10'),
    ('Cascade', 'cascade', '11'),
    ('Glacis', 'glacis', '12'),
    ('Grand''Anse Mahé', 'grandanse-mahé', '13'),
    ('Grand''Anse Praslin', 'grandanse-praslin', '14'),
    ('Ile Perseverance I', 'ile-perseverance-i', '26'),
    ('Ile Perseverance II', 'ile-perseverance-ii', '27'),
    ('La Digue', 'la-digue', '15'),
    ('La Rivière Anglaise', 'la-rivière-anglaise', '16'),
    ('Les Mamelles', 'les-mamelles', '24'),
    ('Mont Buxton', 'mont-buxton', '17'),
    ('Mont Fleuri', 'mont-fleuri', '18'),
    ('Plaisance', 'plaisance', '19'),
    ('Pointe La Rue', 'pointe-la-rue', '20'),
    ('Port Glaud', 'port-glaud', '21'),
    ('Roche Caiman', 'roche-caiman', '25'),
    ('Saint Louis', 'saint-louis', '22'),
    ('Takamaka', 'takamaka', '23')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'seychelles' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Sierra Leone (SL) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Eastern', 'eastern', 'E'),
    ('North Western', 'north-western', 'NW'),
    ('Northern', 'northern', 'N'),
    ('Southern', 'southern', 'S'),
    ('Western', 'western', 'W')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'sierra-leone' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Singapore (SG) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central Singapore', 'central-singapore', '01'),
    ('North East', 'north-east', '02'),
    ('North West', 'north-west', '03'),
    ('South East', 'south-east', '04'),
    ('South West', 'south-west', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'singapore' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Slovakia (SK) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Banská Bystrica', 'banská-bystrica', 'BC'),
    ('Bratislava', 'bratislava', 'BL'),
    ('Košice', 'košice', 'KI'),
    ('Nitra', 'nitra', 'NI'),
    ('Prešov', 'prešov', 'PV'),
    ('Trenčín', 'trenčín', 'TC'),
    ('Trnava', 'trnava', 'TA'),
    ('Žilina', 'žilina', 'ZI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'slovakia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Slovenia (SI) — 212 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'si-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ajdovščina', 'ajdovščina', '001'),
    ('Ankaran', 'ankaran', '213'),
    ('Apače', 'apače', '195'),
    ('Beltinci', 'beltinci', '002'),
    ('Benedikt', 'benedikt', '148'),
    ('Bistrica ob Sotli', 'bistrica-ob-sotli', '149'),
    ('Bled', 'bled', '003'),
    ('Bloke', 'bloke', '150'),
    ('Bohinj', 'bohinj', '004'),
    ('Borovnica', 'borovnica', '005'),
    ('Bovec', 'bovec', '006'),
    ('Braslovče', 'braslovče', '151'),
    ('Brda', 'brda', '007'),
    ('Brežice', 'brežice', '009'),
    ('Brezovica', 'brezovica', '008'),
    ('Cankova', 'cankova', '152'),
    ('Celje', 'celje', '011'),
    ('Cerklje na Gorenjskem', 'cerklje-na-gorenjskem', '012'),
    ('Cerknica', 'cerknica', '013'),
    ('Cerkno', 'cerkno', '014'),
    ('Cerkvenjak', 'cerkvenjak', '153'),
    ('Cirkulane', 'cirkulane', '196'),
    ('Črenšovci', 'črenšovci', '015'),
    ('Črna na Koroškem', 'črna-na-koroškem', '016'),
    ('Črnomelj', 'črnomelj', '017'),
    ('Destrnik', 'destrnik', '018'),
    ('Divača', 'divača', '019'),
    ('Dobje', 'dobje', '154'),
    ('Dobrepolje', 'dobrepolje', '020'),
    ('Dobrna', 'dobrna', '155'),
    ('Dobrova–Polhov Gradec', 'dobrova-polhov-gradec', '021'),
    ('Dobrovnik', 'dobrovnik', '156'),
    ('Dol pri Ljubljani', 'dol-pri-ljubljani', '022'),
    ('Dolenjske Toplice', 'dolenjske-toplice', '157'),
    ('Domžale', 'domžale', '023'),
    ('Dornava', 'dornava', '024'),
    ('Dravograd', 'dravograd', '025'),
    ('Duplek', 'duplek', '026'),
    ('Gorenja Vas–Poljane', 'gorenja-vas-poljane', '027'),
    ('Gorišnica', 'gorišnica', '028'),
    ('Gorje', 'gorje', '207'),
    ('Gornja Radgona', 'gornja-radgona', '029'),
    ('Gornji Grad', 'gornji-grad', '030'),
    ('Gornji Petrovci', 'gornji-petrovci', '031'),
    ('Grad', 'grad', '158'),
    ('Grosuplje', 'grosuplje', '032'),
    ('Hajdina', 'hajdina', '159'),
    ('Hoče–Slivnica', 'hoče-slivnica', '160'),
    ('Hodoš', 'hodoš', '161'),
    ('Horjul', 'horjul', '162'),
    ('Hrastnik', 'hrastnik', '034'),
    ('Hrpelje–Kozina', 'hrpelje-kozina', '035'),
    ('Idrija', 'idrija', '036'),
    ('Ig', 'ig', '037'),
    ('Ilirska Bistrica', 'ilirska-bistrica', '038'),
    ('Ivančna Gorica', 'ivančna-gorica', '039'),
    ('Izola', 'izola', '040'),
    ('Jesenice', 'jesenice', '041'),
    ('Jezersko', 'jezersko', '163'),
    ('Juršinci', 'juršinci', '042'),
    ('Kamnik', 'kamnik', '043'),
    ('Kanal ob Soči', 'kanal-ob-soči', '044'),
    ('Kidričevo', 'kidričevo', '045'),
    ('Kobarid', 'kobarid', '046'),
    ('Kobilje', 'kobilje', '047'),
    ('Kočevje', 'kočevje', '048'),
    ('Komen', 'komen', '049'),
    ('Komenda', 'komenda', '164'),
    ('Koper', 'koper', '050'),
    ('Kostanjevica na Krki', 'kostanjevica-na-krki', '197'),
    ('Kostel', 'kostel', '165'),
    ('Kozje', 'kozje', '051'),
    ('Kranj', 'kranj', '052'),
    ('Kranjska Gora', 'kranjska-gora', '053'),
    ('Križevci', 'križevci', '166'),
    ('Krško', 'krško', '054'),
    ('Kungota', 'kungota', '055'),
    ('Kuzma', 'kuzma', '056'),
    ('Laško', 'laško', '057'),
    ('Lenart', 'lenart', '058'),
    ('Lendava', 'lendava', '059'),
    ('Litija', 'litija', '060'),
    ('Ljubljana', 'ljubljana', '061'),
    ('Ljubno', 'ljubno', '062'),
    ('Ljutomer', 'ljutomer', '063'),
    ('Log–Dragomer', 'log-dragomer', '208'),
    ('Logatec', 'logatec', '064'),
    ('Loška Dolina', 'loška-dolina', '065'),
    ('Loški Potok', 'loški-potok', '066'),
    ('Lovrenc na Pohorju', 'lovrenc-na-pohorju', '167'),
    ('Luče', 'luče', '067'),
    ('Lukovica', 'lukovica', '068'),
    ('Majšperk', 'majšperk', '069'),
    ('Makole', 'makole', '198'),
    ('Maribor', 'maribor', '070'),
    ('Markovci', 'markovci', '168'),
    ('Medvode', 'medvode', '071'),
    ('Mengeš', 'mengeš', '072'),
    ('Metlika', 'metlika', '073'),
    ('Mežica', 'mežica', '074'),
    ('Miklavž na Dravskem Polju', 'miklavž-na-dravskem-polju', '169'),
    ('Miren–Kostanjevica', 'miren-kostanjevica', '075'),
    ('Mirna', 'mirna', '212'),
    ('Mirna Peč', 'mirna-peč', '170'),
    ('Mislinja', 'mislinja', '076'),
    ('Mokronog–Trebelno', 'mokronog-trebelno', '199'),
    ('Moravče', 'moravče', '077'),
    ('Moravske Toplice', 'moravske-toplice', '078'),
    ('Mozirje', 'mozirje', '079'),
    ('Murska Sobota', 'murska-sobota', '080'),
    ('Muta', 'muta', '081'),
    ('Naklo', 'naklo', '082'),
    ('Nazarje', 'nazarje', '083'),
    ('Nova Gorica', 'nova-gorica', '084'),
    ('Novo Mesto', 'novo-mesto', '085'),
    ('Odranci', 'odranci', '086'),
    ('Oplotnica', 'oplotnica', '171'),
    ('Ormož', 'ormož', '087'),
    ('Osilnica', 'osilnica', '088'),
    ('Pesnica', 'pesnica', '089'),
    ('Piran', 'piran', '090'),
    ('Pivka', 'pivka', '091'),
    ('Podčetrtek', 'podčetrtek', '092'),
    ('Podlehnik', 'podlehnik', '172'),
    ('Podvelka', 'podvelka', '093'),
    ('Poljčane', 'poljčane', '200'),
    ('Polzela', 'polzela', '173'),
    ('Postojna', 'postojna', '094'),
    ('Prebold', 'prebold', '174'),
    ('Preddvor', 'preddvor', '095'),
    ('Prevalje', 'prevalje', '175'),
    ('Ptuj', 'ptuj', '096'),
    ('Puconci', 'puconci', '097'),
    ('Rače–Fram', 'rače-fram', '098'),
    ('Radeče', 'radeče', '099'),
    ('Radenci', 'radenci', '100'),
    ('Radlje ob Dravi', 'radlje-ob-dravi', '101'),
    ('Radovljica', 'radovljica', '102'),
    ('Ravne na Koroškem', 'ravne-na-koroškem', '103'),
    ('Razkrižje', 'razkrižje', '176'),
    ('Rečica ob Savinji', 'rečica-ob-savinji', '209'),
    ('Renče–Vogrsko', 'renče-vogrsko', '201'),
    ('Ribnica', 'ribnica', '104'),
    ('Ribnica na Pohorju', 'ribnica-na-pohorju', '177'),
    ('Rogaška Slatina', 'rogaška-slatina', '106'),
    ('Rogašovci', 'rogašovci', '105'),
    ('Rogatec', 'rogatec', '107'),
    ('Ruše', 'ruše', '108'),
    ('Šalovci', 'šalovci', '033'),
    ('Selnica ob Dravi', 'selnica-ob-dravi', '178'),
    ('Semič', 'semič', '109'),
    ('Šempeter–Vrtojba', 'šempeter-vrtojba', '183'),
    ('Šenčur', 'šenčur', '117'),
    ('Šentilj', 'šentilj', '118'),
    ('Šentjernej', 'šentjernej', '119'),
    ('Šentjur', 'šentjur', '120'),
    ('Šentrupert', 'šentrupert', '211'),
    ('Sevnica', 'sevnica', '110'),
    ('Sežana', 'sežana', '111'),
    ('Škocjan', 'škocjan', '121'),
    ('Škofja Loka', 'škofja-loka', '122'),
    ('Škofljica', 'škofljica', '123'),
    ('Slovenj Gradec', 'slovenj-gradec', '112'),
    ('Slovenska Bistrica', 'slovenska-bistrica', '113'),
    ('Slovenske Konjice', 'slovenske-konjice', '114'),
    ('Šmarje pri Jelšah', 'šmarje-pri-jelšah', '124'),
    ('Šmarješke Toplice', 'šmarješke-toplice', '206'),
    ('Šmartno ob Paki', 'šmartno-ob-paki', '125'),
    ('Šmartno pri Litiji', 'šmartno-pri-litiji', '194'),
    ('Sodražica', 'sodražica', '179'),
    ('Solčava', 'solčava', '180'),
    ('Šoštanj', 'šoštanj', '126'),
    ('Središče ob Dravi', 'središče-ob-dravi', '202'),
    ('Starše', 'starše', '115'),
    ('Štore', 'štore', '127'),
    ('Straža', 'straža', '203'),
    ('Sveta Ana', 'sveta-ana', '181'),
    ('Sveta Trojica v Slovenskih Goricah', 'sveta-trojica-v-slovenskih-goricah', '204'),
    ('Sveti Andraž v Slovenskih Goricah', 'sveti-andraž-v-slovenskih-goricah', '182'),
    ('Sveti Jurij ob Ščavnici', 'sveti-jurij-ob-ščavnici', '116'),
    ('Sveti Jurij v Slovenskih Goricah', 'sveti-jurij-v-slovenskih-goricah', '210'),
    ('Sveti Tomaž', 'sveti-tomaž', '205'),
    ('Tabor', 'tabor', '184'),
    ('Tišina', 'tišina', '010'),
    ('Tolmin', 'tolmin', '128'),
    ('Trbovlje', 'trbovlje', '129'),
    ('Trebnje', 'trebnje', '130'),
    ('Trnovska Vas', 'trnovska-vas', '185'),
    ('Tržič', 'tržič', '131'),
    ('Trzin', 'trzin', '186'),
    ('Turnišče', 'turnišče', '132'),
    ('Velenje', 'velenje', '133'),
    ('Velika Polana', 'velika-polana', '187'),
    ('Velike Lašče', 'velike-lašče', '134'),
    ('Veržej', 'veržej', '188'),
    ('Videm', 'videm', '135'),
    ('Vipava', 'vipava', '136'),
    ('Vitanje', 'vitanje', '137'),
    ('Vodice', 'vodice', '138'),
    ('Vojnik', 'vojnik', '139'),
    ('Vransko', 'vransko', '189'),
    ('Vrhnika', 'vrhnika', '140'),
    ('Vuzenica', 'vuzenica', '141'),
    ('Zagorje ob Savi', 'zagorje-ob-savi', '142'),
    ('Žalec', 'žalec', '190'),
    ('Zavrč', 'zavrč', '143'),
    ('Železniki', 'železniki', '146'),
    ('Žetale', 'žetale', '191'),
    ('Žiri', 'žiri', '147'),
    ('Žirovnica', 'žirovnica', '192'),
    ('Zreče', 'zreče', '144'),
    ('Žužemberk', 'žužemberk', '193')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'slovenia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'si-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Solomon Islands (SB) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sb-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central', 'central', 'CE'),
    ('Choiseul', 'choiseul', 'CH'),
    ('Guadalcanal', 'guadalcanal', 'GU'),
    ('Honiara', 'honiara', 'CT'),
    ('Isabel', 'isabel', 'IS'),
    ('Makira-Ulawa', 'makira-ulawa', 'MK'),
    ('Malaita', 'malaita', 'ML'),
    ('Rennell and Bellona', 'rennell-and-bellona', 'RB'),
    ('Temotu', 'temotu', 'TE'),
    ('Western', 'western', 'WE')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'solomon-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sb-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Somalia (SO) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'so-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Awdal', 'awdal', 'AW'),
    ('Bakool', 'bakool', 'BK'),
    ('Banaadir', 'banaadir', 'BN'),
    ('Bari', 'bari', 'BR'),
    ('Bay', 'bay', 'BY'),
    ('Galguduud', 'galguduud', 'GA'),
    ('Gedo', 'gedo', 'GE'),
    ('Hiran', 'hiran', 'HI'),
    ('Lower Juba', 'lower-juba', 'JH'),
    ('Lower Shebelle', 'lower-shebelle', 'SH'),
    ('Middle Juba', 'middle-juba', 'JD'),
    ('Middle Shebelle', 'middle-shebelle', 'SD'),
    ('Mudug', 'mudug', 'MU'),
    ('Nugal', 'nugal', 'NU'),
    ('Sanaag', 'sanaag', 'SA'),
    ('Sool', 'sool', 'SO'),
    ('Togdheer', 'togdheer', 'TO'),
    ('Woqooyi Galbeed', 'woqooyi-galbeed', 'WO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'somalia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'so-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- South Africa (ZA) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'za-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Eastern Cape', 'eastern-cape', 'EC'),
    ('Free State', 'free-state', 'FS'),
    ('Gauteng', 'gauteng', 'GP'),
    ('KwaZulu-Natal', 'kwazulu-natal', 'KZN'),
    ('Limpopo', 'limpopo', 'LP'),
    ('Mpumalanga', 'mpumalanga', 'MP'),
    ('North West', 'north-west', 'NW'),
    ('Northern Cape', 'northern-cape', 'NC'),
    ('Western Cape', 'western-cape', 'WC')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'south-africa' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'za-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- South Korea (KR) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'kr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Busan', 'busan', '26'),
    ('Daegu', 'daegu', '27'),
    ('Daejeon', 'daejeon', '30'),
    ('Gangwon', 'gangwon', '42'),
    ('Gwangju', 'gwangju', '29'),
    ('Gyeonggi', 'gyeonggi', '41'),
    ('Incheon', 'incheon', '28'),
    ('Jeju', 'jeju', '49'),
    ('North Chungcheong', 'north-chungcheong', '43'),
    ('North Gyeongsang', 'north-gyeongsang', '47'),
    ('North Jeolla', 'north-jeolla', '45'),
    ('Sejong City', 'sejong-city', '50'),
    ('Seoul', 'seoul', '11'),
    ('South Chungcheong', 'south-chungcheong', '44'),
    ('South Gyeongsang', 'south-gyeongsang', '48'),
    ('South Jeolla', 'south-jeolla', '46'),
    ('Ulsan', 'ulsan', '31')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'south-korea' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'kr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- South Sudan (SS) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ss-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central Equatoria', 'central-equatoria', 'EC'),
    ('Eastern Equatoria', 'eastern-equatoria', 'EE'),
    ('Jonglei State', 'jonglei-state', 'JG'),
    ('Lakes', 'lakes', 'LK'),
    ('Northern Bahr el Ghazal', 'northern-bahr-el-ghazal', 'BN'),
    ('Unity', 'unity', 'UY'),
    ('Upper Nile', 'upper-nile', 'NU'),
    ('Warrap', 'warrap', 'WR'),
    ('Western Bahr el Ghazal', 'western-bahr-el-ghazal', 'BW'),
    ('Western Equatoria', 'western-equatoria', 'EW')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'south-sudan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ss-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Spain (ES) — 69 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'es-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('A Coruña', 'a-coruña', 'C'),
    ('Albacete', 'albacete', 'AB'),
    ('Alicante', 'alicante', 'A'),
    ('Almeria', 'almeria', 'AL'),
    ('Andalusia', 'andalusia', 'AN'),
    ('Araba', 'araba', 'VI'),
    ('Aragon', 'aragon', 'AR'),
    ('Asturias', 'asturias', 'O'),
    ('Asturias, Principality of', 'asturias-principality-of', 'AS'),
    ('Ávila', 'ávila', 'AV'),
    ('Badajoz', 'badajoz', 'BA'),
    ('Balearic Islands', 'balearic-islands', 'IB'),
    ('Barcelona', 'barcelona', 'B'),
    ('Basque Country', 'basque-country', 'PV'),
    ('Bizkaia', 'bizkaia', 'BI'),
    ('Burgos', 'burgos', 'BU'),
    ('Caceres', 'caceres', 'CC'),
    ('Cádiz', 'cádiz', 'CA'),
    ('Canary Islands', 'canary-islands', 'CN'),
    ('Cantabria', 'cantabria', 'S'),
    ('Cantabria', 'cantabria', 'CB'),
    ('Castellón', 'castellón', 'CS'),
    ('Castile and Leon', 'castile-and-leon', 'CL'),
    ('Castilla-La Mancha', 'castilla-la-mancha', 'CM'),
    ('Catalonia', 'catalonia', 'CT'),
    ('Ceuta', 'ceuta', 'CE'),
    ('Ciudad Real', 'ciudad-real', 'CR'),
    ('Community of Madrid', 'community-of-madrid', 'MD'),
    ('Córdoba', 'córdoba', 'CO'),
    ('Cuenca', 'cuenca', 'CU'),
    ('Estremadura', 'estremadura', 'EX'),
    ('Galicia', 'galicia', 'GA'),
    ('Gipuzkoa', 'gipuzkoa', 'SS'),
    ('Girona', 'girona', 'GI'),
    ('Granada', 'granada', 'GR'),
    ('Guadalajara', 'guadalajara', 'GU'),
    ('Huelva', 'huelva', 'H'),
    ('Huesca', 'huesca', 'HU'),
    ('Islas Baleares', 'islas-baleares', 'PM'),
    ('Jaén', 'jaén', 'J'),
    ('La Rioja', 'la-rioja', 'RI'),
    ('La Rioja', 'la-rioja', 'LO'),
    ('Las Palmas', 'las-palmas', 'GC'),
    ('León', 'león', 'LE'),
    ('Lleida', 'lleida', 'L'),
    ('Lugo', 'lugo', 'LU'),
    ('Madrid', 'madrid', 'M'),
    ('Málaga', 'málaga', 'MA'),
    ('Melilla', 'melilla', 'ML'),
    ('Murcia', 'murcia', 'MU'),
    ('Navarra', 'navarra', 'NA'),
    ('Navarre', 'navarre', 'NC'),
    ('Ourense', 'ourense', 'OR'),
    ('Palencia', 'palencia', 'P'),
    ('Pontevedra', 'pontevedra', 'PO'),
    ('Region of Murcia', 'region-of-murcia', 'MC'),
    ('Salamanca', 'salamanca', 'SA'),
    ('Santa Cruz de Tenerife', 'santa-cruz-de-tenerife', 'TF'),
    ('Segovia', 'segovia', 'SG'),
    ('Sevilla', 'sevilla', 'SE'),
    ('Soria', 'soria', 'SO'),
    ('Tarragona', 'tarragona', 'T'),
    ('Teruel', 'teruel', 'TE'),
    ('Toledo', 'toledo', 'TO'),
    ('Valencia', 'valencia', 'V'),
    ('Valencian Community', 'valencian-community', 'VC'),
    ('Valladolid', 'valladolid', 'VA'),
    ('Zamora', 'zamora', 'ZA'),
    ('Zaragoza', 'zaragoza', 'Z')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'spain' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'es-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Sri Lanka (LK) — 34 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'lk-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ampara', 'ampara', '52'),
    ('Anuradhapura', 'anuradhapura', '71'),
    ('Badulla', 'badulla', '81'),
    ('Batticaloa', 'batticaloa', '51'),
    ('Central', 'central', '2'),
    ('Colombo', 'colombo', '11'),
    ('Eastern', 'eastern', '5'),
    ('Galle', 'galle', '31'),
    ('Gampaha', 'gampaha', '12'),
    ('Hambantota', 'hambantota', '33'),
    ('Jaffna', 'jaffna', '41'),
    ('Kalutara', 'kalutara', '13'),
    ('Kandy', 'kandy', '21'),
    ('Kegalle', 'kegalle', '92'),
    ('Kilinochchi', 'kilinochchi', '42'),
    ('Kurunegala', 'kurunegala', '61'),
    ('Mannar', 'mannar', '43'),
    ('Matale', 'matale', '22'),
    ('Matara', 'matara', '32'),
    ('Monaragala', 'monaragala', '82'),
    ('Mullaitivu', 'mullaitivu', '45'),
    ('North Central', 'north-central', '7'),
    ('North Western', 'north-western', '6'),
    ('Northern', 'northern', '4'),
    ('Nuwara Eliya', 'nuwara-eliya', '23'),
    ('Polonnaruwa', 'polonnaruwa', '72'),
    ('Puttalam', 'puttalam', '62'),
    ('Ratnapura', 'ratnapura', '91'),
    ('Sabaragamuwa', 'sabaragamuwa', '9'),
    ('Southern', 'southern', '3'),
    ('Trincomalee', 'trincomalee', '53'),
    ('Uva', 'uva', '8'),
    ('Vavuniya', 'vavuniya', '44'),
    ('Western', 'western', '1')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'sri-lanka' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'lk-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Sudan (SD) — 18 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sd-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al Jazirah', 'al-jazirah', 'GZ'),
    ('Al Qadarif', 'al-qadarif', 'GD'),
    ('Blue Nile', 'blue-nile', 'NB'),
    ('Central Darfur', 'central-darfur', 'DC'),
    ('East Darfur', 'east-darfur', 'DE'),
    ('Kassala', 'kassala', 'KA'),
    ('Khartoum', 'khartoum', 'KH'),
    ('North Darfur', 'north-darfur', 'DN'),
    ('North Kordofan', 'north-kordofan', 'KN'),
    ('Northern', 'northern', 'NO'),
    ('Red Sea', 'red-sea', 'RS'),
    ('River Nile', 'river-nile', 'NR'),
    ('Sennar', 'sennar', 'SI'),
    ('South Darfur', 'south-darfur', 'DS'),
    ('South Kordofan', 'south-kordofan', 'KS'),
    ('West Darfur', 'west-darfur', 'DW'),
    ('West Kordofan', 'west-kordofan', 'GK'),
    ('White Nile', 'white-nile', 'NW')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'sudan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sd-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Suriname (SR) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Brokopondo', 'brokopondo', 'BR'),
    ('Commewijne', 'commewijne', 'CM'),
    ('Coronie', 'coronie', 'CR'),
    ('Marowijne', 'marowijne', 'MA'),
    ('Nickerie', 'nickerie', 'NI'),
    ('Para', 'para', 'PR'),
    ('Paramaribo', 'paramaribo', 'PM'),
    ('Saramacca', 'saramacca', 'SA'),
    ('Sipaliwini', 'sipaliwini', 'SI'),
    ('Wanica', 'wanica', 'WA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'suriname' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Sweden (SE) — 21 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'se-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Blekinge', 'blekinge', 'K'),
    ('Dalarna', 'dalarna', 'W'),
    ('Gävleborg', 'gävleborg', 'X'),
    ('Gotland', 'gotland', 'I'),
    ('Halland', 'halland', 'N'),
    ('Jämtland', 'jämtland', 'Z'),
    ('Jönköping', 'jönköping', 'F'),
    ('Kalmar', 'kalmar', 'H'),
    ('Kronoberg', 'kronoberg', 'G'),
    ('Norrbotten', 'norrbotten', 'BD'),
    ('Örebro', 'örebro', 'T'),
    ('Östergötland', 'östergötland', 'E'),
    ('Skåne', 'skåne', 'M'),
    ('Södermanland', 'södermanland', 'D'),
    ('Stockholm', 'stockholm', 'AB'),
    ('Uppsala', 'uppsala', 'C'),
    ('Värmland', 'värmland', 'S'),
    ('Västerbotten', 'västerbotten', 'AC'),
    ('Västernorrland', 'västernorrland', 'Y'),
    ('Västmanland', 'västmanland', 'U'),
    ('Västra Götaland', 'västra-götaland', 'O')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'sweden' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'se-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Switzerland (CH) — 26 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ch-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aargau', 'aargau', 'AG'),
    ('Appenzell Ausserrhoden', 'appenzell-ausserrhoden', 'AR'),
    ('Appenzell Innerrhoden', 'appenzell-innerrhoden', 'AI'),
    ('Basel-Land', 'basel-land', 'BL'),
    ('Basel-Stadt', 'basel-stadt', 'BS'),
    ('Bern', 'bern', 'BE'),
    ('Fribourg', 'fribourg', 'FR'),
    ('Geneva', 'geneva', 'GE'),
    ('Glarus', 'glarus', 'GL'),
    ('Graubünden', 'graubünden', 'GR'),
    ('Jura', 'jura', 'JU'),
    ('Lucerne', 'lucerne', 'LU'),
    ('Neuchâtel', 'neuchâtel', 'NE'),
    ('Nidwalden', 'nidwalden', 'NW'),
    ('Obwalden', 'obwalden', 'OW'),
    ('Schaffhausen', 'schaffhausen', 'SH'),
    ('Schwyz', 'schwyz', 'SZ'),
    ('Solothurn', 'solothurn', 'SO'),
    ('St. Gallen', 'st-gallen', 'SG'),
    ('Thurgau', 'thurgau', 'TG'),
    ('Ticino', 'ticino', 'TI'),
    ('Uri', 'uri', 'UR'),
    ('Valais', 'valais', 'VS'),
    ('Vaud', 'vaud', 'VD'),
    ('Zug', 'zug', 'ZG'),
    ('Zürich', 'zürich', 'ZH')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'switzerland' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ch-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Syria (SY) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'sy-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Al-Hasakah', 'al-hasakah', 'HA'),
    ('Al-Raqqah', 'al-raqqah', 'RA'),
    ('Aleppo', 'aleppo', 'HL'),
    ('As-Suwayda', 'as-suwayda', 'SU'),
    ('Damascus', 'damascus', 'DI'),
    ('Daraa', 'daraa', 'DR'),
    ('Deir ez-Zor', 'deir-ez-zor', 'DY'),
    ('Hama', 'hama', 'HM'),
    ('Homs', 'homs', 'HI'),
    ('Idlib', 'idlib', 'ID'),
    ('Latakia', 'latakia', 'LA'),
    ('Quneitra', 'quneitra', 'QU'),
    ('Rif Dimashq', 'rif-dimashq', 'RD'),
    ('Tartus', 'tartus', 'TA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'syria' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'sy-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Taiwan (TW) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Changhua', 'changhua', 'CHA'),
    ('Chiayi', 'chiayi', 'CYI'),
    ('Chiayi County', 'chiayi-county', 'CYQ'),
    ('Hsinchu', 'hsinchu', 'HSZ'),
    ('Hsinchu County', 'hsinchu-county', 'HSQ'),
    ('Hualien', 'hualien', 'HUA'),
    ('Kaohsiung', 'kaohsiung', 'KHH'),
    ('Keelung', 'keelung', 'KEE'),
    ('Kinmen', 'kinmen', 'KIN'),
    ('Lienchiang', 'lienchiang', 'LIE'),
    ('Miaoli', 'miaoli', 'MIA'),
    ('Nantou', 'nantou', 'NAN'),
    ('New Taipei', 'new-taipei', 'NWT'),
    ('Penghu', 'penghu', 'PEN'),
    ('Pingtung', 'pingtung', 'PIF'),
    ('Taichung', 'taichung', 'TXG'),
    ('Tainan', 'tainan', 'TNN'),
    ('Taipei', 'taipei', 'TPE'),
    ('Taitung', 'taitung', 'TTT'),
    ('Taoyuan', 'taoyuan', 'TAO'),
    ('Yilan', 'yilan', 'ILA'),
    ('Yunlin', 'yunlin', 'YUN')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'taiwan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Tajikistan (TJ) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tj-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Dushanbe', 'dushanbe', 'DU'),
    ('Gorno-Badakhshan', 'gorno-badakhshan', 'GB'),
    ('Khatlon', 'khatlon', 'KT'),
    ('Nohiyahoi Tobei Jumhurí ', 'nohiyahoi-tobei-jumhurí', 'RA'),
    ('Sughd ', 'sughd', 'SU')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'tajikistan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tj-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Tanzania (TZ) — 31 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Arusha', 'arusha', '01'),
    ('Dar es Salaam', 'dar-es-salaam', '02'),
    ('Dodoma', 'dodoma', '03'),
    ('Geita', 'geita', '27'),
    ('Iringa', 'iringa', '04'),
    ('Kagera', 'kagera', '05'),
    ('Katavi', 'katavi', '28'),
    ('Kigoma', 'kigoma', '08'),
    ('Kilimanjaro', 'kilimanjaro', '09'),
    ('Lindi', 'lindi', '12'),
    ('Manyara', 'manyara', '26'),
    ('Mara', 'mara', '13'),
    ('Mbeya', 'mbeya', '14'),
    ('Morogoro', 'morogoro', '16'),
    ('Mtwara', 'mtwara', '17'),
    ('Mwanza', 'mwanza', '18'),
    ('Njombe', 'njombe', '29'),
    ('Pemba North', 'pemba-north', '06'),
    ('Pemba South', 'pemba-south', '10'),
    ('Pwani', 'pwani', '19'),
    ('Rukwa', 'rukwa', '20'),
    ('Ruvuma', 'ruvuma', '21'),
    ('Shinyanga', 'shinyanga', '22'),
    ('Simiyu', 'simiyu', '30'),
    ('Singida', 'singida', '23'),
    ('Songwe', 'songwe', '31'),
    ('Tabora', 'tabora', '24'),
    ('Tanga', 'tanga', '25'),
    ('Zanzibar North', 'zanzibar-north', '07'),
    ('Zanzibar South', 'zanzibar-south', '11'),
    ('Zanzibar West', 'zanzibar-west', '15')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'tanzania' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Thailand (TH) — 78 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'th-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Amnat Charoen', 'amnat-charoen', '37'),
    ('Ang Thong', 'ang-thong', '15'),
    ('Bangkok', 'bangkok', '10'),
    ('Bueng Kan', 'bueng-kan', '38'),
    ('Buri Ram', 'buri-ram', '31'),
    ('Chachoengsao', 'chachoengsao', '24'),
    ('Chai Nat', 'chai-nat', '18'),
    ('Chaiyaphum', 'chaiyaphum', '36'),
    ('Chanthaburi', 'chanthaburi', '22'),
    ('Chiang Mai', 'chiang-mai', '50'),
    ('Chiang Rai', 'chiang-rai', '57'),
    ('Chon Buri', 'chon-buri', '20'),
    ('Chumphon', 'chumphon', '86'),
    ('Kalasin', 'kalasin', '46'),
    ('Kamphaeng Phet', 'kamphaeng-phet', '62'),
    ('Kanchanaburi', 'kanchanaburi', '71'),
    ('Khon Kaen', 'khon-kaen', '40'),
    ('Krabi', 'krabi', '81'),
    ('Lampang', 'lampang', '52'),
    ('Lamphun', 'lamphun', '51'),
    ('Loei', 'loei', '42'),
    ('Lop Buri', 'lop-buri', '16'),
    ('Mae Hong Son', 'mae-hong-son', '58'),
    ('Maha Sarakham', 'maha-sarakham', '44'),
    ('Mukdahan', 'mukdahan', '49'),
    ('Nakhon Nayok', 'nakhon-nayok', '26'),
    ('Nakhon Pathom', 'nakhon-pathom', '73'),
    ('Nakhon Phanom', 'nakhon-phanom', '48'),
    ('Nakhon Ratchasima', 'nakhon-ratchasima', '30'),
    ('Nakhon Sawan', 'nakhon-sawan', '60'),
    ('Nakhon Si Thammarat', 'nakhon-si-thammarat', '80'),
    ('Nan', 'nan', '55'),
    ('Narathiwat', 'narathiwat', '96'),
    ('Nong Bua Lam Phu', 'nong-bua-lam-phu', '39'),
    ('Nong Khai', 'nong-khai', '43'),
    ('Nonthaburi', 'nonthaburi', '12'),
    ('Pathum Thani', 'pathum-thani', '13'),
    ('Pattani', 'pattani', '94'),
    ('Pattaya', 'pattaya', 'S'),
    ('Phangnga', 'phangnga', '82'),
    ('Phatthalung', 'phatthalung', '93'),
    ('Phayao', 'phayao', '56'),
    ('Phetchabun', 'phetchabun', '67'),
    ('Phetchaburi', 'phetchaburi', '76'),
    ('Phichit', 'phichit', '66'),
    ('Phitsanulok', 'phitsanulok', '65'),
    ('Phra Nakhon Si Ayutthaya', 'phra-nakhon-si-ayutthaya', '14'),
    ('Phrae', 'phrae', '54'),
    ('Phuket', 'phuket', '83'),
    ('Prachin Buri', 'prachin-buri', '25'),
    ('Prachuap Khiri Khan', 'prachuap-khiri-khan', '77'),
    ('Ranong', 'ranong', '85'),
    ('Ratchaburi', 'ratchaburi', '70'),
    ('Rayong', 'rayong', '21'),
    ('Roi Et', 'roi-et', '45'),
    ('Sa Kaeo', 'sa-kaeo', '27'),
    ('Sakon Nakhon', 'sakon-nakhon', '47'),
    ('Samut Prakan', 'samut-prakan', '11'),
    ('Samut Sakhon', 'samut-sakhon', '74'),
    ('Samut Songkhram', 'samut-songkhram', '75'),
    ('Saraburi', 'saraburi', '19'),
    ('Satun', 'satun', '91'),
    ('Si Sa Ket', 'si-sa-ket', '33'),
    ('Sing Buri', 'sing-buri', '17'),
    ('Songkhla', 'songkhla', '90'),
    ('Sukhothai', 'sukhothai', '64'),
    ('Suphan Buri', 'suphan-buri', '72'),
    ('Surat Thani', 'surat-thani', '84'),
    ('Surin', 'surin', '32'),
    ('Tak', 'tak', '63'),
    ('Trang', 'trang', '92'),
    ('Trat', 'trat', '23'),
    ('Ubon Ratchathani', 'ubon-ratchathani', '34'),
    ('Udon Thani', 'udon-thani', '41'),
    ('Uthai Thani', 'uthai-thani', '61'),
    ('Uttaradit', 'uttaradit', '53'),
    ('Yala', 'yala', '95'),
    ('Yasothon', 'yasothon', '35')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'thailand' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'th-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- The Bahamas (BS) — 32 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'bs-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Acklins', 'acklins', 'AK'),
    ('Berry Islands', 'berry-islands', 'BY'),
    ('Bimini', 'bimini', 'BI'),
    ('Black Point', 'black-point', 'BP'),
    ('Cat Island', 'cat-island', 'CI'),
    ('Central Abaco', 'central-abaco', 'CO'),
    ('Central Andros', 'central-andros', 'CS'),
    ('Central Eleuthera', 'central-eleuthera', 'CE'),
    ('Crooked Island', 'crooked-island', 'CK'),
    ('East Grand Bahama', 'east-grand-bahama', 'EG'),
    ('Exuma', 'exuma', 'EX'),
    ('Freeport', 'freeport', 'FP'),
    ('Grand Cay', 'grand-cay', 'GC'),
    ('Harbour Island', 'harbour-island', 'HI'),
    ('Hope Town', 'hope-town', 'HT'),
    ('Inagua', 'inagua', 'IN'),
    ('Long Island', 'long-island', 'LI'),
    ('Mangrove Cay', 'mangrove-cay', 'MC'),
    ('Mayaguana', 'mayaguana', 'MG'),
    ('Moore''s Island', 'moores-island', 'MI'),
    ('New Providence', 'new-providence', 'NP'),
    ('North Abaco', 'north-abaco', 'NO'),
    ('North Andros', 'north-andros', 'NS'),
    ('North Eleuthera', 'north-eleuthera', 'NE'),
    ('Ragged Island', 'ragged-island', 'RI'),
    ('Rum Cay', 'rum-cay', 'RC'),
    ('San Salvador Island', 'san-salvador-island', 'SS'),
    ('South Abaco', 'south-abaco', 'SO'),
    ('South Andros', 'south-andros', 'SA'),
    ('South Eleuthera', 'south-eleuthera', 'SE'),
    ('Spanish Wells', 'spanish-wells', 'SW'),
    ('West Grand Bahama', 'west-grand-bahama', 'WG')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'the-bahamas' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'bs-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- The Gambia (GM) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Banjul', 'banjul', 'B'),
    ('Central River', 'central-river', 'M'),
    ('Lower River', 'lower-river', 'L'),
    ('North Bank', 'north-bank', 'N'),
    ('Upper River', 'upper-river', 'U'),
    ('West Coast', 'west-coast', 'W')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'the-gambia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Timor-Leste (TL) — 13 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tl-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aileu', 'aileu', 'AL'),
    ('Ainaro', 'ainaro', 'AN'),
    ('Baucau', 'baucau', 'BA'),
    ('Bobonaro', 'bobonaro', 'BO'),
    ('Cova Lima', 'cova-lima', 'CO'),
    ('Dili', 'dili', 'DI'),
    ('Ermera', 'ermera', 'ER'),
    ('Lautém', 'lautém', 'LA'),
    ('Liquiçá', 'liquiçá', 'LI'),
    ('Manatuto', 'manatuto', 'MT'),
    ('Manufahi', 'manufahi', 'MF'),
    ('Oecusse', 'oecusse', 'OE'),
    ('Viqueque', 'viqueque', 'VI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'timor-leste' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tl-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Togo (TG) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tg-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Centrale', 'centrale', 'C'),
    ('Kara', 'kara', 'K'),
    ('Maritime', 'maritime', 'M'),
    ('Plateaux', 'plateaux', 'P'),
    ('Savanes', 'savanes', 'S')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'togo' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tg-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Tonga (TO) — 5 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'to-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Haʻapai', 'haʻapai', '02'),
    ('ʻEua', 'ʻeua', '01'),
    ('Niuas', 'niuas', '03'),
    ('Tongatapu', 'tongatapu', '04'),
    ('Vavaʻu', 'vavaʻu', '05')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'tonga' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'to-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Trinidad and Tobago (TT) — 17 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tt-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Arima', 'arima', 'ARI'),
    ('Chaguanas', 'chaguanas', 'CHA'),
    ('Couva-Tabaquite-Talparo', 'couva-tabaquite-talparo', 'CTT'),
    ('Diego Martin', 'diego-martin', 'DMN'),
    ('Eastern Tobago', 'eastern-tobago', 'ETO'),
    ('Penal-Debe', 'penal-debe', 'PED'),
    ('Point Fortin', 'point-fortin', 'PTF'),
    ('Port of Spain', 'port-of-spain', 'POS'),
    ('Princes Town', 'princes-town', 'PRT'),
    ('Rio Claro-Mayaro', 'rio-claro-mayaro', 'MRC'),
    ('San Fernando', 'san-fernando', 'SFO'),
    ('San Juan-Laventille', 'san-juan-laventille', 'SJL'),
    ('Sangre Grande', 'sangre-grande', 'SGE'),
    ('Siparia', 'siparia', 'SIP'),
    ('Tobago', 'tobago', 'TOB'),
    ('Tunapuna-Piarco', 'tunapuna-piarco', 'TUP'),
    ('Western Tobago', 'western-tobago', 'WTO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'trinidad-and-tobago' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tt-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Tunisia (TN) — 24 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ariana', 'ariana', '12'),
    ('Béja', 'béja', '31'),
    ('Ben Arous', 'ben-arous', '13'),
    ('Bizerte', 'bizerte', '23'),
    ('Gabès', 'gabès', '81'),
    ('Gafsa', 'gafsa', '71'),
    ('Jendouba', 'jendouba', '32'),
    ('Kairouan', 'kairouan', '41'),
    ('Kasserine', 'kasserine', '42'),
    ('Kebili', 'kebili', '73'),
    ('Kef', 'kef', '33'),
    ('Mahdia', 'mahdia', '53'),
    ('Manouba', 'manouba', '14'),
    ('Medenine', 'medenine', '82'),
    ('Monastir', 'monastir', '52'),
    ('Nabeul', 'nabeul', '21'),
    ('Sfax', 'sfax', '61'),
    ('Sidi Bouzid', 'sidi-bouzid', '43'),
    ('Siliana', 'siliana', '34'),
    ('Sousse', 'sousse', '51'),
    ('Tataouine', 'tataouine', '83'),
    ('Tozeur', 'tozeur', '72'),
    ('Tunis', 'tunis', '11'),
    ('Zaghouan', 'zaghouan', '22')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'tunisia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Turkey (TR) — 81 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tr-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Adana', 'adana', '01'),
    ('Adıyaman', 'adıyaman', '02'),
    ('Afyonkarahisar', 'afyonkarahisar', '03'),
    ('Ağrı', 'ağrı', '04'),
    ('Aksaray', 'aksaray', '68'),
    ('Amasya', 'amasya', '05'),
    ('Ankara', 'ankara', '06'),
    ('Antalya', 'antalya', '07'),
    ('Ardahan', 'ardahan', '75'),
    ('Artvin', 'artvin', '08'),
    ('Aydın', 'aydın', '09'),
    ('Balıkesir', 'balıkesir', '10'),
    ('Bartın', 'bartın', '74'),
    ('Batman', 'batman', '72'),
    ('Bayburt', 'bayburt', '69'),
    ('Bilecik', 'bilecik', '11'),
    ('Bingöl', 'bingöl', '12'),
    ('Bitlis', 'bitlis', '13'),
    ('Bolu', 'bolu', '14'),
    ('Burdur', 'burdur', '15'),
    ('Bursa', 'bursa', '16'),
    ('Çanakkale', 'çanakkale', '17'),
    ('Çankırı', 'çankırı', '18'),
    ('Çorum', 'çorum', '19'),
    ('Denizli', 'denizli', '20'),
    ('Diyarbakır', 'diyarbakır', '21'),
    ('Düzce', 'düzce', '81'),
    ('Edirne', 'edirne', '22'),
    ('Elazığ', 'elazığ', '23'),
    ('Erzincan', 'erzincan', '24'),
    ('Erzurum', 'erzurum', '25'),
    ('Eskişehir', 'eskişehir', '26'),
    ('Gaziantep', 'gaziantep', '27'),
    ('Giresun', 'giresun', '28'),
    ('Gümüşhane', 'gümüşhane', '29'),
    ('Hakkâri', 'hakkâri', '30'),
    ('Hatay', 'hatay', '31'),
    ('Iğdır', 'iğdır', '76'),
    ('Isparta', 'isparta', '32'),
    ('İstanbul', 'i̇stanbul', '34'),
    ('İzmir', 'i̇zmir', '35'),
    ('Kahramanmaraş', 'kahramanmaraş', '46'),
    ('Karabük', 'karabük', '78'),
    ('Karaman', 'karaman', '70'),
    ('Kars', 'kars', '36'),
    ('Kastamonu', 'kastamonu', '37'),
    ('Kayseri', 'kayseri', '38'),
    ('Kilis', 'kilis', '79'),
    ('Kırıkkale', 'kırıkkale', '71'),
    ('Kırklareli', 'kırklareli', '39'),
    ('Kırşehir', 'kırşehir', '40'),
    ('Kocaeli', 'kocaeli', '41'),
    ('Konya', 'konya', '42'),
    ('Kütahya', 'kütahya', '43'),
    ('Malatya', 'malatya', '44'),
    ('Manisa', 'manisa', '45'),
    ('Mardin', 'mardin', '47'),
    ('Mersin', 'mersin', '33'),
    ('Muğla', 'muğla', '48'),
    ('Muş', 'muş', '49'),
    ('Nevşehir', 'nevşehir', '50'),
    ('Niğde', 'niğde', '51'),
    ('Ordu', 'ordu', '52'),
    ('Osmaniye', 'osmaniye', '80'),
    ('Rize', 'rize', '53'),
    ('Sakarya', 'sakarya', '54'),
    ('Samsun', 'samsun', '55'),
    ('Şanlıurfa', 'şanlıurfa', '63'),
    ('Siirt', 'siirt', '56'),
    ('Sinop', 'sinop', '57'),
    ('Sivas', 'sivas', '58'),
    ('Şırnak', 'şırnak', '73'),
    ('Tekirdağ', 'tekirdağ', '59'),
    ('Tokat', 'tokat', '60'),
    ('Trabzon', 'trabzon', '61'),
    ('Tunceli', 'tunceli', '62'),
    ('Uşak', 'uşak', '64'),
    ('Van', 'van', '65'),
    ('Yalova', 'yalova', '77'),
    ('Yozgat', 'yozgat', '66'),
    ('Zonguldak', 'zonguldak', '67')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'turkey' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tr-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Turkmenistan (TM) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Ahal', 'ahal', 'A'),
    ('Ashgabat', 'ashgabat', 'S'),
    ('Balkan', 'balkan', 'B'),
    ('Daşoguz', 'daşoguz', 'D'),
    ('Lebap', 'lebap', 'L'),
    ('Mary', 'mary', 'M')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'turkmenistan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Turks and Caicos Islands (TC) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tc-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Grand Turk', 'grand-turk', '05'),
    ('Middle Caicos', 'middle-caicos', '03'),
    ('North Caicos', 'north-caicos', '02'),
    ('Providenciales', 'providenciales', '01'),
    ('Salt Cay', 'salt-cay', '06'),
    ('South Caicos', 'south-caicos', '04')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'turks-and-caicos-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tc-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Tuvalu (TV) — 8 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'tv-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Funafuti', 'funafuti', 'FUN'),
    ('Nanumanga', 'nanumanga', 'NMG'),
    ('Nanumea', 'nanumea', 'NMA'),
    ('Niutao Island Council', 'niutao-island-council', 'NIT'),
    ('Nui', 'nui', 'NUI'),
    ('Nukufetau', 'nukufetau', 'NKF'),
    ('Nukulaelae', 'nukulaelae', 'NKL'),
    ('Vaitupu', 'vaitupu', 'VAI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'tuvalu' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'tv-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Uganda (UG) — 139 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ug-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abim', 'abim', '314'),
    ('Adjumani', 'adjumani', '301'),
    ('Agago', 'agago', '322'),
    ('Alebtong', 'alebtong', '323'),
    ('Amolatar', 'amolatar', '315'),
    ('Amudat', 'amudat', '324'),
    ('Amuria', 'amuria', '216'),
    ('Amuru', 'amuru', '316'),
    ('Apac', 'apac', '302'),
    ('Arua', 'arua', '303'),
    ('Budaka', 'budaka', '217'),
    ('Bududa', 'bududa', '218'),
    ('Bugiri', 'bugiri', '201'),
    ('Bugweri', 'bugweri', '235'),
    ('Buhweju', 'buhweju', '420'),
    ('Buikwe', 'buikwe', '117'),
    ('Bukedea', 'bukedea', '219'),
    ('Bukomansimbi', 'bukomansimbi', '118'),
    ('Bukwo', 'bukwo', '220'),
    ('Bulambuli', 'bulambuli', '225'),
    ('Buliisa', 'buliisa', '416'),
    ('Bundibugyo', 'bundibugyo', '401'),
    ('Bunyangabu', 'bunyangabu', '430'),
    ('Bushenyi', 'bushenyi', '402'),
    ('Busia', 'busia', '202'),
    ('Butaleja', 'butaleja', '221'),
    ('Butambala', 'butambala', '119'),
    ('Butebo', 'butebo', '233'),
    ('Buvuma', 'buvuma', '120'),
    ('Buyende', 'buyende', '226'),
    ('Central', 'central', 'C'),
    ('Dokolo', 'dokolo', '317'),
    ('Eastern', 'eastern', 'E'),
    ('Gomba', 'gomba', '121'),
    ('Gulu', 'gulu', '304'),
    ('Hoima', 'hoima', '403'),
    ('Ibanda', 'ibanda', '417'),
    ('Iganga', 'iganga', '203'),
    ('Isingiro', 'isingiro', '418'),
    ('Jinja', 'jinja', '204'),
    ('Kaabong', 'kaabong', '318'),
    ('Kabale', 'kabale', '404'),
    ('Kabarole', 'kabarole', '405'),
    ('Kaberamaido', 'kaberamaido', '213'),
    ('Kagadi', 'kagadi', '427'),
    ('Kakumiro', 'kakumiro', '428'),
    ('Kalaki', 'kalaki', '237'),
    ('Kalangala', 'kalangala', '101'),
    ('Kaliro', 'kaliro', '222'),
    ('Kalungu', 'kalungu', '122'),
    ('Kampala', 'kampala', '102'),
    ('Kamuli', 'kamuli', '205'),
    ('Kamwenge', 'kamwenge', '413'),
    ('Kanungu', 'kanungu', '414'),
    ('Kapchorwa', 'kapchorwa', '206'),
    ('Kapelebyong', 'kapelebyong', '236'),
    ('Karenga', 'karenga', '335'),
    ('Kasanda', 'kasanda', '126'),
    ('Kasese', 'kasese', '406'),
    ('Katakwi', 'katakwi', '207'),
    ('Kayunga', 'kayunga', '112'),
    ('Kazo', 'kazo', '433'),
    ('Kibaale', 'kibaale', '407'),
    ('Kiboga', 'kiboga', '103'),
    ('Kibuku', 'kibuku', '227'),
    ('Kikuube', 'kikuube', '432'),
    ('Kiruhura', 'kiruhura', '419'),
    ('Kiryandongo', 'kiryandongo', '421'),
    ('Kisoro', 'kisoro', '408'),
    ('Kitagwenda', 'kitagwenda', '434'),
    ('Kitgum', 'kitgum', '305'),
    ('Koboko', 'koboko', '319'),
    ('Kole', 'kole', '325'),
    ('Kotido', 'kotido', '306'),
    ('Kumi', 'kumi', '208'),
    ('Kwania', 'kwania', '333'),
    ('Kween', 'kween', '228'),
    ('Kyankwanzi', 'kyankwanzi', '123'),
    ('Kyegegwa', 'kyegegwa', '422'),
    ('Kyenjojo', 'kyenjojo', '415'),
    ('Kyotera', 'kyotera', '125'),
    ('Lamwo', 'lamwo', '326'),
    ('Lira', 'lira', '307'),
    ('Luuka', 'luuka', '229'),
    ('Luwero', 'luwero', '104'),
    ('Lwengo', 'lwengo', '124'),
    ('Lyantonde', 'lyantonde', '114'),
    ('Madi-Okollo', 'madi-okollo', '336'),
    ('Manafwa', 'manafwa', '223'),
    ('Maracha', 'maracha', '320'),
    ('Masaka', 'masaka', '105'),
    ('Masindi', 'masindi', '409'),
    ('Mayuge', 'mayuge', '214'),
    ('Mbale', 'mbale', '209'),
    ('Mbarara', 'mbarara', '410'),
    ('Mitooma', 'mitooma', '423'),
    ('Mityana', 'mityana', '115'),
    ('Moroto', 'moroto', '308'),
    ('Moyo', 'moyo', '309'),
    ('Mpigi', 'mpigi', '106'),
    ('Mubende', 'mubende', '107'),
    ('Mukono', 'mukono', '108'),
    ('Nabilatuk', 'nabilatuk', '334'),
    ('Nakapiripirit', 'nakapiripirit', '311'),
    ('Nakaseke', 'nakaseke', '116'),
    ('Nakasongola', 'nakasongola', '109'),
    ('Namayingo', 'namayingo', '230'),
    ('Namisindwa', 'namisindwa', '234'),
    ('Namutumba', 'namutumba', '224'),
    ('Napak', 'napak', '327'),
    ('Nebbi', 'nebbi', '310'),
    ('Ngora', 'ngora', '231'),
    ('Northern', 'northern', 'N'),
    ('Ntoroko', 'ntoroko', '424'),
    ('Ntungamo', 'ntungamo', '411'),
    ('Nwoya', 'nwoya', '328'),
    ('Obongi', 'obongi', '337'),
    ('Omoro', 'omoro', '331'),
    ('Otuke', 'otuke', '329'),
    ('Oyam', 'oyam', '321'),
    ('Pader', 'pader', '312'),
    ('Pakwach', 'pakwach', '332'),
    ('Pallisa', 'pallisa', '210'),
    ('Rakai', 'rakai', '110'),
    ('Rubanda', 'rubanda', '429'),
    ('Rubirizi', 'rubirizi', '425'),
    ('Rukiga', 'rukiga', '431'),
    ('Rukungiri', 'rukungiri', '412'),
    ('Rwampara', 'rwampara', '435'),
    ('Sembabule', 'sembabule', '111'),
    ('Serere', 'serere', '232'),
    ('Sheema', 'sheema', '426'),
    ('Sironko', 'sironko', '215'),
    ('Soroti', 'soroti', '211'),
    ('Tororo', 'tororo', '212'),
    ('Wakiso', 'wakiso', '113'),
    ('Western', 'western', 'W'),
    ('Yumbe', 'yumbe', '313'),
    ('Zombo', 'zombo', '330')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'uganda' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ug-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Ukraine (UA) — 27 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ua-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Autonomous Republic of Crimea', 'autonomous-republic-of-crimea', '43'),
    ('Cherkaska', 'cherkaska', '71'),
    ('Chernihivska', 'chernihivska', '74'),
    ('Chernivetska', 'chernivetska', '77'),
    ('Dnipropetrovska', 'dnipropetrovska', '12'),
    ('Donetska', 'donetska', '14'),
    ('Ivano-Frankivska', 'ivano-frankivska', '26'),
    ('Kharkivska', 'kharkivska', '63'),
    ('Khersonska', 'khersonska', '65'),
    ('Khmelnytska', 'khmelnytska', '68'),
    ('Kirovohradska', 'kirovohradska', '35'),
    ('Kyiv', 'kyiv', '30'),
    ('Kyivska', 'kyivska', '32'),
    ('Luhanska', 'luhanska', '09'),
    ('Lvivska', 'lvivska', '46'),
    ('Mykolaivska', 'mykolaivska', '48'),
    ('Odeska', 'odeska', '51'),
    ('Poltavska', 'poltavska', '53'),
    ('Rivnenska', 'rivnenska', '56'),
    ('Sevastopol', 'sevastopol', '40'),
    ('Sumska', 'sumska', '59'),
    ('Ternopilska', 'ternopilska', '61'),
    ('Vinnytska', 'vinnytska', '05'),
    ('Volynska', 'volynska', '07'),
    ('Zakarpatska', 'zakarpatska', '21'),
    ('Zaporizka', 'zaporizka', '23'),
    ('Zhytomyrska', 'zhytomyrska', '18')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'ukraine' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ua-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United Arab Emirates (AE) — 7 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ae-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abu Dhabi', 'abu-dhabi', 'AZ'),
    ('Ajman', 'ajman', 'AJ'),
    ('Dubai', 'dubai', 'DU'),
    ('Fujairah', 'fujairah', 'FU'),
    ('Ras Al Khaimah', 'ras-al-khaimah', 'RK'),
    ('Sharjah', 'sharjah', 'SH'),
    ('Umm Al Quwain', 'umm-al-quwain', 'UQ')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-arab-emirates' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ae-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United Kingdom (GB) — 221 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'gb-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Aberdeen', 'aberdeen', 'ABE'),
    ('Aberdeenshire', 'aberdeenshire', 'ABD'),
    ('Angus', 'angus', 'ANS'),
    ('Antrim and Newtownabbey', 'antrim-and-newtownabbey', 'ANN'),
    ('Ards and North Down', 'ards-and-north-down', 'AND'),
    ('Argyll and Bute', 'argyll-and-bute', 'AGB'),
    ('Armagh, Banbridge and Craigavon', 'armagh-banbridge-and-craigavon', 'ABC'),
    ('Barking and Dagenham', 'barking-and-dagenham', 'BDG'),
    ('Barnet', 'barnet', 'BNE'),
    ('Barnsley', 'barnsley', 'BNS'),
    ('Bath and North East Somerset', 'bath-and-north-east-somerset', 'BAS'),
    ('Bedford', 'bedford', 'BDF'),
    ('Belfast', 'belfast', 'BFS'),
    ('Bexley', 'bexley', 'BEX'),
    ('Birmingham', 'birmingham', 'BIR'),
    ('Blackburn with Darwen', 'blackburn-with-darwen', 'BBD'),
    ('Blackpool', 'blackpool', 'BPL'),
    ('Blaenau Gwent', 'blaenau-gwent', 'BGW'),
    ('Bolton', 'bolton', 'BOL'),
    ('Bournemouth, Christchurch and Poole', 'bournemouth-christchurch-and-poole', 'BCP'),
    ('Bracknell Forest', 'bracknell-forest', 'BRC'),
    ('Bradford', 'bradford', 'BRD'),
    ('Brent', 'brent', 'BEN'),
    ('Bridgend', 'bridgend', 'BGE'),
    ('Brighton and Hove', 'brighton-and-hove', 'BNH'),
    ('Bristol', 'bristol', 'BST'),
    ('Bromley', 'bromley', 'BRY'),
    ('Buckinghamshire', 'buckinghamshire', 'BKM'),
    ('Bury', 'bury', 'BUR'),
    ('Caerphilly', 'caerphilly', 'CAY'),
    ('Calderdale', 'calderdale', 'CLD'),
    ('Cambridgeshire', 'cambridgeshire', 'CAM'),
    ('Camden', 'camden', 'CMD'),
    ('Cardiff', 'cardiff', 'CRF'),
    ('Carmarthenshire', 'carmarthenshire', 'CMN'),
    ('Causeway Coast and Glens', 'causeway-coast-and-glens', 'CCG'),
    ('Central Bedfordshire', 'central-bedfordshire', 'CBF'),
    ('Ceredigion', 'ceredigion', 'CGN'),
    ('Cheshire East', 'cheshire-east', 'CHE'),
    ('Cheshire West and Chester', 'cheshire-west-and-chester', 'CHW'),
    ('City of Kingston upon Hull', 'city-of-kingston-upon-hull', 'KHL'),
    ('City of Southampton', 'city-of-southampton', 'STH'),
    ('Clackmannanshire', 'clackmannanshire', 'CLK'),
    ('Conwy', 'conwy', 'CWY'),
    ('Cornwall', 'cornwall', 'CON'),
    ('Coventry', 'coventry', 'COV'),
    ('Croydon', 'croydon', 'CRY'),
    ('Cumbria', 'cumbria', 'CMA'),
    ('Darlington', 'darlington', 'DAL'),
    ('Denbighshire', 'denbighshire', 'DEN'),
    ('Derby', 'derby', 'DER'),
    ('Derbyshire', 'derbyshire', 'DBY'),
    ('Derry City and Strabane', 'derry-city-and-strabane', 'DRS'),
    ('Devon', 'devon', 'DEV'),
    ('Doncaster', 'doncaster', 'DNC'),
    ('Dorset', 'dorset', 'DOR'),
    ('Dudley', 'dudley', 'DUD'),
    ('Dumfries and Galloway', 'dumfries-and-galloway', 'DGY'),
    ('Dundee', 'dundee', 'DND'),
    ('Durham', 'durham', 'DUR'),
    ('Ealing', 'ealing', 'EAL'),
    ('East Ayrshire', 'east-ayrshire', 'EAY'),
    ('East Dunbartonshire', 'east-dunbartonshire', 'EDU'),
    ('East Lothian', 'east-lothian', 'ELN'),
    ('East Renfrewshire', 'east-renfrewshire', 'ERW'),
    ('East Riding of Yorkshire', 'east-riding-of-yorkshire', 'ERY'),
    ('East Sussex', 'east-sussex', 'ESX'),
    ('Edinburgh', 'edinburgh', 'EDH'),
    ('Enfield', 'enfield', 'ENF'),
    ('England', 'england', 'ENG'),
    ('Essex', 'essex', 'ESS'),
    ('Falkirk', 'falkirk', 'FAL'),
    ('Fermanagh and Omagh', 'fermanagh-and-omagh', 'FMO'),
    ('Fife', 'fife', 'FIF'),
    ('Flintshire', 'flintshire', 'FLN'),
    ('Gateshead', 'gateshead', 'GAT'),
    ('Glasgow', 'glasgow', 'GLG'),
    ('Gloucestershire', 'gloucestershire', 'GLS'),
    ('Greenwich', 'greenwich', 'GRE'),
    ('Gwynedd', 'gwynedd', 'GWN'),
    ('Hackney', 'hackney', 'HCK'),
    ('Halton', 'halton', 'HAL'),
    ('Hammersmith and Fulham', 'hammersmith-and-fulham', 'HMF'),
    ('Hampshire', 'hampshire', 'HAM'),
    ('Haringey', 'haringey', 'HRY'),
    ('Harrow', 'harrow', 'HRW'),
    ('Hartlepool', 'hartlepool', 'HPL'),
    ('Havering', 'havering', 'HAV'),
    ('Herefordshire', 'herefordshire', 'HEF'),
    ('Hertfordshire', 'hertfordshire', 'HRT'),
    ('Highland', 'highland', 'HLD'),
    ('Hillingdon', 'hillingdon', 'HIL'),
    ('Hounslow', 'hounslow', 'HNS'),
    ('Inverclyde', 'inverclyde', 'IVC'),
    ('Isle of Anglesey', 'isle-of-anglesey', 'AGY'),
    ('Isle of Wight', 'isle-of-wight', 'IOW'),
    ('Isles of Scilly', 'isles-of-scilly', 'IOS'),
    ('Islington', 'islington', 'ISL'),
    ('Kensington and Chelsea', 'kensington-and-chelsea', 'KEC'),
    ('Kent', 'kent', 'KEN'),
    ('Kingston upon Thames', 'kingston-upon-thames', 'KTT'),
    ('Kirklees', 'kirklees', 'KIR'),
    ('Knowsley', 'knowsley', 'KWL'),
    ('Lambeth', 'lambeth', 'LBH'),
    ('Lancashire', 'lancashire', 'LAN'),
    ('Leeds', 'leeds', 'LDS'),
    ('Leicester', 'leicester', 'LCE'),
    ('Leicestershire', 'leicestershire', 'LEC'),
    ('Lewisham', 'lewisham', 'LEW'),
    ('Lincolnshire', 'lincolnshire', 'LIN'),
    ('Lisburn and Castlereagh', 'lisburn-and-castlereagh', 'LBC'),
    ('Liverpool', 'liverpool', 'LIV'),
    ('London', 'london', 'LND'),
    ('Luton', 'luton', 'LUT'),
    ('Manchester', 'manchester', 'MAN'),
    ('Medway', 'medway', 'MDW'),
    ('Merthyr Tydfil', 'merthyr-tydfil', 'MTY'),
    ('Merton', 'merton', 'MRT'),
    ('Mid and East Antrim', 'mid-and-east-antrim', 'MEA'),
    ('Mid Ulster', 'mid-ulster', 'MUL'),
    ('Middlesbrough', 'middlesbrough', 'MDB'),
    ('Midlothian', 'midlothian', 'MLN'),
    ('Milton Keynes', 'milton-keynes', 'MIK'),
    ('Monmouthshire', 'monmouthshire', 'MON'),
    ('Moray', 'moray', 'MRY'),
    ('Neath Port Talbot', 'neath-port-talbot', 'NTL'),
    ('Newcastle upon Tyne', 'newcastle-upon-tyne', 'NET'),
    ('Newham', 'newham', 'NWM'),
    ('Newport', 'newport', 'NWP'),
    ('Newry, Mourne and Down', 'newry-mourne-and-down', 'NMD'),
    ('Norfolk', 'norfolk', 'NFK'),
    ('North Ayrshire', 'north-ayrshire', 'NAY'),
    ('North East Lincolnshire', 'north-east-lincolnshire', 'NEL'),
    ('North Lanarkshire', 'north-lanarkshire', 'NLK'),
    ('North Lincolnshire', 'north-lincolnshire', 'NLN'),
    ('North Northamptonshire', 'north-northamptonshire', 'NNH'),
    ('North Somerset', 'north-somerset', 'NSM'),
    ('North Tyneside', 'north-tyneside', 'NTY'),
    ('North Yorkshire', 'north-yorkshire', 'NYK'),
    ('Northern Ireland', 'northern-ireland', 'NIR'),
    ('Northumberland', 'northumberland', 'NBL'),
    ('Nottingham', 'nottingham', 'NGM'),
    ('Nottinghamshire', 'nottinghamshire', 'NTT'),
    ('Oldham', 'oldham', 'OLD'),
    ('Orkney Islands', 'orkney-islands', 'ORK'),
    ('Outer Hebrides', 'outer-hebrides', 'ELS'),
    ('Oxfordshire', 'oxfordshire', 'OXF'),
    ('Pembrokeshire', 'pembrokeshire', 'PEM'),
    ('Perth and Kinross', 'perth-and-kinross', 'PKN'),
    ('Peterborough', 'peterborough', 'PTE'),
    ('Plymouth', 'plymouth', 'PLY'),
    ('Portsmouth', 'portsmouth', 'POR'),
    ('Powys', 'powys', 'POW'),
    ('Reading', 'reading', 'RDG'),
    ('Redbridge', 'redbridge', 'RDB'),
    ('Redcar and Cleveland', 'redcar-and-cleveland', 'RCC'),
    ('Renfrewshire', 'renfrewshire', 'RFW'),
    ('Rhondda Cynon Taf', 'rhondda-cynon-taf', 'RCT'),
    ('Richmond upon Thames', 'richmond-upon-thames', 'RIC'),
    ('Rochdale', 'rochdale', 'RCH'),
    ('Rotherham', 'rotherham', 'ROT'),
    ('Rutland', 'rutland', 'RUT'),
    ('Salford', 'salford', 'SLF'),
    ('Sandwell', 'sandwell', 'SAW'),
    ('Scotland', 'scotland', 'SCT'),
    ('Scottish Borders', 'scottish-borders', 'SCB'),
    ('Sefton', 'sefton', 'SFT'),
    ('Sheffield', 'sheffield', 'SHF'),
    ('Shetland Islands', 'shetland-islands', 'ZET'),
    ('Shropshire', 'shropshire', 'SHR'),
    ('Slough', 'slough', 'SLG'),
    ('Solihull', 'solihull', 'SOL'),
    ('Somerset', 'somerset', 'SOM'),
    ('South Ayrshire', 'south-ayrshire', 'SAY'),
    ('South Gloucestershire', 'south-gloucestershire', 'SGC'),
    ('South Lanarkshire', 'south-lanarkshire', 'SLK'),
    ('South Tyneside', 'south-tyneside', 'STY'),
    ('Southend-on-Sea', 'southend-on-sea', 'SOS'),
    ('Southwark', 'southwark', 'SWK'),
    ('St Helens', 'st-helens', 'SHN'),
    ('Staffordshire', 'staffordshire', 'STS'),
    ('Stirling', 'stirling', 'STG'),
    ('Stockport', 'stockport', 'SKP'),
    ('Stockton-on-Tees', 'stockton-on-tees', 'STT'),
    ('Stoke-on-Trent', 'stoke-on-trent', 'STE'),
    ('Suffolk', 'suffolk', 'SFK'),
    ('Sunderland', 'sunderland', 'SND'),
    ('Surrey', 'surrey', 'SRY'),
    ('Sutton', 'sutton', 'STN'),
    ('Swansea', 'swansea', 'SWA'),
    ('Swindon', 'swindon', 'SWD'),
    ('Tameside', 'tameside', 'TAM'),
    ('Telford and Wrekin', 'telford-and-wrekin', 'TFW'),
    ('Thurrock', 'thurrock', 'THR'),
    ('Torbay', 'torbay', 'TOB'),
    ('Torfaen', 'torfaen', 'TOF'),
    ('Tower Hamlets', 'tower-hamlets', 'TWH'),
    ('Trafford', 'trafford', 'TRF'),
    ('Vale of Glamorgan', 'vale-of-glamorgan', 'VGL'),
    ('Wakefield', 'wakefield', 'WKF'),
    ('Wales', 'wales', 'WLS'),
    ('Walsall', 'walsall', 'WLL'),
    ('Waltham Forest', 'waltham-forest', 'WFT'),
    ('Wandsworth', 'wandsworth', 'WND'),
    ('Warrington', 'warrington', 'WRT'),
    ('Warwickshire', 'warwickshire', 'WAR'),
    ('West Berkshire', 'west-berkshire', 'WBK'),
    ('West Dunbartonshire', 'west-dunbartonshire', 'WDU'),
    ('West Lothian', 'west-lothian', 'WLN'),
    ('West Northamptonshire', 'west-northamptonshire', 'WNH'),
    ('West Sussex', 'west-sussex', 'WSX'),
    ('Westminster', 'westminster', 'WSM'),
    ('Wigan', 'wigan', 'WGN'),
    ('Wiltshire', 'wiltshire', 'WIL'),
    ('Windsor and Maidenhead', 'windsor-and-maidenhead', 'WNM'),
    ('Wirral', 'wirral', 'WRL'),
    ('Wokingham', 'wokingham', 'WOK'),
    ('Wolverhampton', 'wolverhampton', 'WLV'),
    ('Worcestershire', 'worcestershire', 'WOR'),
    ('Wrexham', 'wrexham', 'WRX'),
    ('York', 'york', 'YOR')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-kingdom' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'gb-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United States (US) — 60 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'us-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alabama', 'alabama', 'AL'),
    ('Alaska', 'alaska', 'AK'),
    ('American Samoa', 'american-samoa', 'AS'),
    ('Arizona', 'arizona', 'AZ'),
    ('Arkansas', 'arkansas', 'AR'),
    ('Armed Forces Europe', 'armed-forces-europe', 'AE'),
    ('Armed Forces of the Americas', 'armed-forces-of-the-americas', 'AA'),
    ('Armed Forces Pacific', 'armed-forces-pacific', 'AP'),
    ('California', 'california', 'CA'),
    ('Colorado', 'colorado', 'CO'),
    ('Connecticut', 'connecticut', 'CT'),
    ('Delaware', 'delaware', 'DE'),
    ('District of Columbia', 'district-of-columbia', 'DC'),
    ('Florida', 'florida', 'FL'),
    ('Georgia', 'georgia', 'GA'),
    ('Guam', 'guam', 'GU'),
    ('Hawaii', 'hawaii', 'HI'),
    ('Idaho', 'idaho', 'ID'),
    ('Illinois', 'illinois', 'IL'),
    ('Indiana', 'indiana', 'IN'),
    ('Iowa', 'iowa', 'IA'),
    ('Kansas', 'kansas', 'KS'),
    ('Kentucky', 'kentucky', 'KY'),
    ('Louisiana', 'louisiana', 'LA'),
    ('Maine', 'maine', 'ME'),
    ('Maryland', 'maryland', 'MD'),
    ('Massachusetts', 'massachusetts', 'MA'),
    ('Michigan', 'michigan', 'MI'),
    ('Minnesota', 'minnesota', 'MN'),
    ('Mississippi', 'mississippi', 'MS'),
    ('Missouri', 'missouri', 'MO'),
    ('Montana', 'montana', 'MT'),
    ('Nebraska', 'nebraska', 'NE'),
    ('Nevada', 'nevada', 'NV'),
    ('New Hampshire', 'new-hampshire', 'NH'),
    ('New Jersey', 'new-jersey', 'NJ'),
    ('New Mexico', 'new-mexico', 'NM'),
    ('New York', 'new-york', 'NY'),
    ('North Carolina', 'north-carolina', 'NC'),
    ('North Dakota', 'north-dakota', 'ND'),
    ('Northern Mariana Islands', 'northern-mariana-islands', 'MP'),
    ('Ohio', 'ohio', 'OH'),
    ('Oklahoma', 'oklahoma', 'OK'),
    ('Oregon', 'oregon', 'OR'),
    ('Pennsylvania', 'pennsylvania', 'PA'),
    ('Puerto Rico', 'puerto-rico', 'PR'),
    ('Rhode Island', 'rhode-island', 'RI'),
    ('South Carolina', 'south-carolina', 'SC'),
    ('South Dakota', 'south-dakota', 'SD'),
    ('Tennessee', 'tennessee', 'TN'),
    ('Texas', 'texas', 'TX'),
    ('United States Minor Outlying Islands', 'united-states-minor-outlying-islands', 'UM'),
    ('United States Virgin Islands', 'united-states-virgin-islands', 'VI'),
    ('Utah', 'utah', 'UT'),
    ('Vermont', 'vermont', 'VT'),
    ('Virginia', 'virginia', 'VA'),
    ('Washington', 'washington', 'WA'),
    ('West Virginia', 'west-virginia', 'WV'),
    ('Wisconsin', 'wisconsin', 'WI'),
    ('Wyoming', 'wyoming', 'WY')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-states' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'us-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- United States Minor Outlying Islands (UM) — 9 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'um-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Baker Island', 'baker-island', '81'),
    ('Howland Island', 'howland-island', '84'),
    ('Jarvis Island', 'jarvis-island', '86'),
    ('Johnston Atoll', 'johnston-atoll', '67'),
    ('Kingman Reef', 'kingman-reef', '89'),
    ('Midway Islands', 'midway-islands', '71'),
    ('Navassa Island', 'navassa-island', '76'),
    ('Palmyra Atoll', 'palmyra-atoll', '95'),
    ('Wake Island', 'wake-island', '79')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'united-states-minor-outlying-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'um-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Uruguay (UY) — 19 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'uy-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Artigas', 'artigas', 'AR'),
    ('Canelones', 'canelones', 'CA'),
    ('Cerro Largo', 'cerro-largo', 'CL'),
    ('Colonia', 'colonia', 'CO'),
    ('Durazno', 'durazno', 'DU'),
    ('Flores', 'flores', 'FS'),
    ('Florida', 'florida', 'FD'),
    ('Lavalleja', 'lavalleja', 'LA'),
    ('Maldonado', 'maldonado', 'MA'),
    ('Montevideo', 'montevideo', 'MO'),
    ('Paysandú', 'paysandú', 'PA'),
    ('Río Negro', 'río-negro', 'RN'),
    ('Rivera', 'rivera', 'RV'),
    ('Rocha', 'rocha', 'RO'),
    ('Salto', 'salto', 'SA'),
    ('San José', 'san-josé', 'SJ'),
    ('Soriano', 'soriano', 'SO'),
    ('Tacuarembó', 'tacuarembó', 'TA'),
    ('Treinta y Tres', 'treinta-y-tres', 'TT')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'uruguay' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'uy-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Uzbekistan (UZ) — 14 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'uz-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Andijan', 'andijan', 'AN'),
    ('Bukhara', 'bukhara', 'BU'),
    ('Fergana', 'fergana', 'FA'),
    ('Jizzakh', 'jizzakh', 'JI'),
    ('Karakalpakstan', 'karakalpakstan', 'QR'),
    ('Namangan', 'namangan', 'NG'),
    ('Navoiy', 'navoiy', 'NW'),
    ('Qashqadaryo', 'qashqadaryo', 'QA'),
    ('Samarqand', 'samarqand', 'SA'),
    ('Sirdaryo', 'sirdaryo', 'SI'),
    ('Surxondaryo', 'surxondaryo', 'SU'),
    ('Tashkent', 'tashkent', 'TK'),
    ('Tashkent', 'tashkent', 'TO'),
    ('Xorazm', 'xorazm', 'XO')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'uzbekistan' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'uz-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Vanuatu (VU) — 6 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'vu-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Malampa', 'malampa', 'MAP'),
    ('Penama', 'penama', 'PAM'),
    ('Sanma', 'sanma', 'SAM'),
    ('Shefa', 'shefa', 'SEE'),
    ('Tafea', 'tafea', 'TAE'),
    ('Torba', 'torba', 'TOB')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'vanuatu' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'vu-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Venezuela (VE) — 25 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    've-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Amazonas', 'amazonas', 'Z'),
    ('Anzoátegui', 'anzoátegui', 'B'),
    ('Apure', 'apure', 'C'),
    ('Aragua', 'aragua', 'D'),
    ('Barinas', 'barinas', 'E'),
    ('Bolívar', 'bolívar', 'F'),
    ('Carabobo', 'carabobo', 'G'),
    ('Cojedes', 'cojedes', 'H'),
    ('Delta Amacuro', 'delta-amacuro', 'Y'),
    ('Distrito Capital', 'distrito-capital', 'A'),
    ('Falcón', 'falcón', 'I'),
    ('Guárico', 'guárico', 'J'),
    ('La Guaira', 'la-guaira', 'X'),
    ('Lara', 'lara', 'K'),
    ('Mérida', 'mérida', 'L'),
    ('Miranda', 'miranda', 'M'),
    ('Monagas', 'monagas', 'N'),
    ('Nueva Esparta', 'nueva-esparta', 'O'),
    ('Portuguesa', 'portuguesa', 'P'),
    ('Sucre', 'sucre', 'R'),
    ('Táchira', 'táchira', 'S'),
    ('Trujillo', 'trujillo', 'T'),
    ('Venezuela', 'venezuela', 'W'),
    ('Yaracuy', 'yaracuy', 'U'),
    ('Zulia', 'zulia', 'V')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'venezuela' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 've-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Vietnam (VN) — 34 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'vn-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('An Giang', 'an-giang', '44'),
    ('Bắc Ninh', 'bắc-ninh', '56'),
    ('Cà Mau', 'cà-mau', '59'),
    ('Cần Thơ', 'cần-thơ', 'CT'),
    ('Cao Bằng', 'cao-bằng', '04'),
    ('Đà Nẵng', 'đà-nẵng', 'DN'),
    ('Đắk Lắk', 'đắk-lắk', '33'),
    ('Điện Biên', 'điện-biên', '71'),
    ('Đồng Nai', 'đồng-nai', '39'),
    ('Đồng Tháp', 'đồng-tháp', '45'),
    ('Gia Lai', 'gia-lai', '30'),
    ('Hà Nội', 'hà-nội', 'HN'),
    ('Hà Tĩnh', 'hà-tĩnh', '23'),
    ('Hải Phòng', 'hải-phòng', 'HP'),
    ('Hồ Chí Minh', 'hồ-chí-minh', 'SG'),
    ('Hưng Yên', 'hưng-yên', '66'),
    ('Khánh Hòa', 'khánh-hòa', '34'),
    ('Lai Châu', 'lai-châu', '01'),
    ('Lâm Đồng', 'lâm-đồng', '35'),
    ('Lạng Sơn', 'lạng-sơn', '09'),
    ('Lào Cai', 'lào-cai', '02'),
    ('Nghệ An', 'nghệ-an', '22'),
    ('Ninh Bình', 'ninh-bình', '18'),
    ('Phú Thọ', 'phú-thọ', '68'),
    ('Quảng Ngãi', 'quảng-ngãi', '29'),
    ('Quảng Ninh', 'quảng-ninh', '13'),
    ('Quảng Trị', 'quảng-trị', '25'),
    ('Sơn La', 'sơn-la', '05'),
    ('Tây Ninh', 'tây-ninh', '37'),
    ('Thái Nguyên', 'thái-nguyên', '69'),
    ('Thanh Hóa', 'thanh-hóa', '21'),
    ('Thừa Thiên-Huế', 'thừa-thiên-huế', '26'),
    ('Tuyên Quang', 'tuyên-quang', '07'),
    ('Vĩnh Long', 'vĩnh-long', '49')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'vietnam' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'vn-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Virgin Islands (US) (VI) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'vi-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Saint Croix', 'saint-croix', 'SC'),
    ('Saint John', 'saint-john', 'SJ'),
    ('Saint Thomas', 'saint-thomas', 'ST')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'virgin-islands-us' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'vi-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Wallis and Futuna Islands (WF) — 3 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'wf-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Alo', 'alo', 'AL'),
    ('Sigave', 'sigave', 'SG'),
    ('Uvea', 'uvea', 'UV')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'wallis-and-futuna-islands' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'wf-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Yemen (YE) — 22 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'ye-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Abyan', 'abyan', 'AB'),
    ('Ad Dali''', 'ad-dali', 'DA'),
    ('Adan', 'adan', 'AD'),
    ('Al Bayda''', 'al-bayda', 'BA'),
    ('Al Hudaydah', 'al-hudaydah', 'HU'),
    ('Al Jawf', 'al-jawf', 'JA'),
    ('Al Mahrah', 'al-mahrah', 'MR'),
    ('Al Mahwit', 'al-mahwit', 'MW'),
    ('Amanat Al Asimah', 'amanat-al-asimah', 'SA'),
    ('Amran', 'amran', 'AM'),
    ('Dhamar', 'dhamar', 'DH'),
    ('Hadhramaut', 'hadhramaut', 'HD'),
    ('Hajjah', 'hajjah', 'HJ'),
    ('Ibb', 'ibb', 'IB'),
    ('Lahij', 'lahij', 'LA'),
    ('Ma''rib', 'marib', 'MA'),
    ('Raymah', 'raymah', 'RA'),
    ('Saada', 'saada', 'SD'),
    ('Sana''a', 'sanaa', 'SN'),
    ('Shabwah', 'shabwah', 'SH'),
    ('Socotra', 'socotra', 'SU'),
    ('Ta''izz', 'taizz', 'TA')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'yemen' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'ye-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Zambia (ZM) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'zm-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Central', 'central', '02'),
    ('Copperbelt', 'copperbelt', '08'),
    ('Eastern', 'eastern', '03'),
    ('Luapula', 'luapula', '04'),
    ('Lusaka', 'lusaka', '09'),
    ('Muchinga', 'muchinga', '10'),
    ('Northern', 'northern', '05'),
    ('Northwestern', 'northwestern', '06'),
    ('Southern', 'southern', '07'),
    ('Western', 'western', '01')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'zambia' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'zm-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

-- Zimbabwe (ZW) — 10 states
INSERT INTO spaces (tenant_id, space_type_id, name, slug, geo_code, level, member_count, post_count, follower_count, visibility, posting_perm, replying_perm, commenting_perm, joining_perm, status)
SELECT
    '00000000-0000-0000-0000-000000000001',
    (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'state'),
    v.name,
    'zw-' || v.slug,
    v.code,
    'state',
    0, 0, 0,
    'public', 'members', 'members', 'members', 'open',
    'active'
FROM (VALUES
    ('Bulawayo', 'bulawayo', 'BU'),
    ('Harare', 'harare', 'HA'),
    ('Manicaland', 'manicaland', 'MA'),
    ('Mashonaland Central', 'mashonaland-central', 'MC'),
    ('Mashonaland East', 'mashonaland-east', 'ME'),
    ('Mashonaland West', 'mashonaland-west', 'MW'),
    ('Masvingo', 'masvingo', 'MV'),
    ('Matabeleland North', 'matabeleland-north', 'MN'),
    ('Matabeleland South', 'matabeleland-south', 'MS'),
    ('Midlands', 'midlands', 'MI')
) AS v(name, slug, code)
ON CONFLICT (tenant_id, slug) DO NOTHING;

INSERT INTO space_parent_spaces (space_id, parent_space_id)
SELECT s.id, p.id
FROM spaces s, spaces p
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND p.slug = 'zimbabwe' AND p.level = 'country'
  AND s.level = 'state' AND s.slug LIKE 'zw-%'
ON CONFLICT (space_id, parent_space_id) DO NOTHING;

