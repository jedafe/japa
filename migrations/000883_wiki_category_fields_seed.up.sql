-- 000883_wiki_category_fields_seed.up.sql
-- §80 Destination Wiki: most wiki_{type}_{category} field groups were
-- created empty by the category seeds, so GET /wiki/:space/:page returns
-- no `fields` block and the item-tables/quick-facts widgets never render
-- (GetPageFields returns nil for groups with zero show_in_frontend fields).
-- Seed canonical field definitions for every empty non-directory category
-- group — directory-mode categories (states/cities/universities/
-- attractions listings) intentionally get none; their widget is the child
-- directory panel. Fields are community-editable (is_system=false,
-- show_in_frontend=true) with the standard view-all/edit-non-guest grant
-- convention from 000859/000860.
--
-- Ref: .jeos/sections/80-destination-wiki.md

BEGIN;

-- 1. Field definitions.
INSERT INTO custom_fields (group_id, key, label, field_type, options, position, is_system, show_in_frontend, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.position, false, true, m.section
FROM custom_field_groups g
JOIN (VALUES
	-- ── Country ──────────────────────────────────────────────────────
	('wiki_country_healthcare', 'healthcare_system',        'Healthcare System',          'select',      '["Public","Private","Mixed (public + private)"]',                    1, NULL),
	('wiki_country_healthcare', 'emergency_number',         'Emergency Number',           'text',        NULL,                                                                   2, NULL),
	('wiki_country_healthcare', 'health_insurance_required','Health Insurance Required',  'boolean',     NULL,                                                                   3, NULL),
	('wiki_country_healthcare', 'avg_gp_visit',             'Avg GP Visit Cost',          'price_range', NULL,                                                                   4, NULL),
	('wiki_country_healthcare', 'hospitals_note',           'Hospitals & Clinics',        'textarea',    NULL,                                                                   5, NULL),
	('wiki_country_healthcare', 'expat_health_tips',        'Expat Health Tips',          'textarea',    NULL,                                                                   6, NULL),

	('wiki_country_housing',    'rent_1br',                 'Rent — 1BR (monthly)',       'price_range', NULL,                                                                   1, NULL),
	('wiki_country_housing',    'rent_3br',                 'Rent — 3BR (monthly)',       'price_range', NULL,                                                                   2, NULL),
	('wiki_country_housing',    'deposit_norm',             'Typical Deposit',            'text',        NULL,                                                                   3, NULL),
	('wiki_country_housing',    'utilities_included',       'Utilities Usually Included', 'boolean',     NULL,                                                                   4, NULL),
	('wiki_country_housing',    'popular_expat_areas',      'Popular Expat Areas',        'repeater',    NULL,                                                                   5, NULL),
	('wiki_country_housing',    'tenant_rights_note',       'Tenant Rights',              'textarea',    NULL,                                                                   6, NULL),

	('wiki_country_banking',    'major_banks',              'Major Banks',                'repeater',    NULL,                                                                   1, NULL),
	('wiki_country_banking',    'account_requirements',     'Account Requirements',       'textarea',    NULL,                                                                   2, NULL),
	('wiki_country_banking',    'online_banking_quality',   'Online Banking Quality',     'select',      '["Excellent","Good","Basic","Poor"]',                                  3, NULL),
	('wiki_country_banking',    'intl_transfer_options',    'Intl Transfer Options',      'repeater',    NULL,                                                                   4, NULL),
	('wiki_country_banking',    'banking_note',             'Banking Notes',              'textarea',    NULL,                                                                   5, NULL),

	('wiki_country_visa',       'common_visa_types',        'Common Visa Types',          'repeater',    NULL,                                                                   1, NULL),
	('wiki_country_visa',       'processing_time',          'Typical Processing Time',    'text',        NULL,                                                                   2, NULL),
	('wiki_country_visa',       'visa_fee_range',           'Visa Fee Range',             'price_range', NULL,                                                                   3, NULL),
	('wiki_country_visa',       'work_permit_available',    'Work Permits Available',     'boolean',     NULL,                                                                   4, NULL),
	('wiki_country_visa',       'requirements_summary',     'Entry Requirements',         'textarea',    NULL,                                                                   5, NULL),
	('wiki_country_visa',       'residency_pathways',       'Residency Pathways',         'textarea',    NULL,                                                                   6, NULL),

	('wiki_country_community',  'expat_population',         'Expat Population (est.)',    'number',      NULL,                                                                   1, NULL),
	('wiki_country_community',  'popular_expat_cities',     'Popular Expat Cities',       'repeater',    NULL,                                                                   2, NULL),
	('wiki_country_community',  'community_groups',         'Community Groups & Forums',  'repeater',    NULL,                                                                   3, NULL),
	('wiki_country_community',  'languages_spoken',         'Languages Spoken',           'repeater',    NULL,                                                                   4, NULL),

	('wiki_country_other',      'driving_side',             'Driving Side',               'select',      '["Right","Left"]',                                                     1, NULL),
	('wiki_country_other',      'power_plugs',              'Power Plug Types',           'repeater',    NULL,                                                                   2, NULL),
	('wiki_country_other',      'public_holidays_note',     'Public Holidays',            'textarea',    NULL,                                                                   3, NULL),

	-- ── City ─────────────────────────────────────────────────────────
	('wiki_city_healthcare',    'major_hospitals',          'Major Hospitals',            'repeater',    NULL,                                                                   1, NULL),
	('wiki_city_healthcare',    'emergency_number',         'Emergency Number',           'text',        NULL,                                                                   2, NULL),
	('wiki_city_healthcare',    'avg_gp_visit',             'Avg GP Visit Cost',          'price_range', NULL,                                                                   3, NULL),
	('wiki_city_healthcare',    'clinics_note',             'Clinics & Specialists',      'textarea',    NULL,                                                                   4, NULL),
	('wiki_city_healthcare',    'insurance_note',           'Insurance & Coverage',       'textarea',    NULL,                                                                   5, NULL),

	('wiki_city_community',     'expat_neighborhoods',      'Expat Neighborhoods',        'repeater',    NULL,                                                                   1, NULL),
	('wiki_city_community',     'community_groups',         'Community Groups & Meetups', 'repeater',    NULL,                                                                   2, NULL),
	('wiki_city_community',     'coworking_spaces',         'Coworking Spaces',           'repeater',    NULL,                                                                   3, NULL),
	('wiki_city_community',     'community_note',           'Community Notes',            'textarea',    NULL,                                                                   4, NULL),

	('wiki_city_other',         'local_tips',               'Local Tips',                 'textarea',    NULL,                                                                   1, NULL),
	('wiki_city_other',         'useful_links',             'Useful Links',               'repeater',    NULL,                                                                   2, NULL),

	-- §80 page-template group (rent prices topic page) — assigned to
	-- space_type:city but not yet bound to a template; seeding keeps it
	-- ready for the rent-prices topic page.
	('wiki_city_page_rent_prices', 'studio',                'Studio (monthly)',           'price_range', NULL,                                                                   1, 'rent_prices'),
	('wiki_city_page_rent_prices', 'one_bedroom',           '1 Bedroom (monthly)',        'price_range', NULL,                                                                   2, 'rent_prices'),
	('wiki_city_page_rent_prices', 'two_bedroom',           '2 Bedroom (monthly)',        'price_range', NULL,                                                                   3, 'rent_prices'),
	('wiki_city_page_rent_prices', 'three_bedroom',         '3 Bedroom (monthly)',        'price_range', NULL,                                                                   4, 'rent_prices'),
	('wiki_city_page_rent_prices', 'qf_best_value_area',    'Best Value Area',            'text',        NULL,                                                                   5, 'quick_facts'),

	-- ── State ────────────────────────────────────────────────────────
	('wiki_state_overview',     'capital',                  'Capital',                    'text',        NULL,                                                                   1, NULL),
	('wiki_state_overview',     'population',               'Population',                 'number',      NULL,                                                                   2, NULL),
	('wiki_state_overview',     'major_cities',             'Major Cities',               'repeater',    NULL,                                                                   3, NULL),
	('wiki_state_overview',     'timezone',                 'Timezone',                   'text',        NULL,                                                                   4, NULL),
	('wiki_state_overview',     'official_language',        'Official Language',          'text',        NULL,                                                                   5, NULL),
	('wiki_state_overview',     'popular_places',           'Popular Places',             'repeater',    NULL,                                                                   6, NULL),

	-- Mirrors the wiki_country_cost_of_living shape: exp_* price ranges
	-- under monthly_expenses plus the quick_facts card trio.
	('wiki_state_cost_of_living', 'exp_accommodation',      'Accommodation',              'price_range', NULL,                                                                   1, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_food',               'Food',                       'price_range', NULL,                                                                   2, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_transportation',     'Transportation',             'price_range', NULL,                                                                   3, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_utilities',          'Utilities',                  'price_range', NULL,                                                                   4, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_internet',           'Internet',                   'price_range', NULL,                                                                   5, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_mobile',             'Mobile plan',                'price_range', NULL,                                                                   6, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_entertainment',      'Entertainment',              'price_range', NULL,                                                                   7, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'exp_health',             'Health insurance',           'price_range', NULL,                                                                   8, 'monthly_expenses'),
	('wiki_state_cost_of_living', 'qf_currency',            'Currency',                   'text',        NULL,                                                                   9, 'quick_facts'),
	('wiki_state_cost_of_living', 'qf_monthly_range',       'Average monthly cost',       'text',        NULL,                                                                  10, 'quick_facts'),
	('wiki_state_cost_of_living', 'qf_best_for',            'Best for',                   'text',        NULL,                                                                  11, 'quick_facts'),

	('wiki_state_healthcare',   'major_hospitals',          'Major Hospitals',            'repeater',    NULL,                                                                   1, NULL),
	('wiki_state_healthcare',   'emergency_number',         'Emergency Number',           'text',        NULL,                                                                   2, NULL),
	('wiki_state_healthcare',   'healthcare_note',          'Healthcare Notes',           'textarea',    NULL,                                                                   3, NULL),

	('wiki_state_housing',      'rent_1br',                 'Rent — 1BR (monthly)',       'price_range', NULL,                                                                   1, NULL),
	('wiki_state_housing',      'rent_3br',                 'Rent — 3BR (monthly)',       'price_range', NULL,                                                                   2, NULL),
	('wiki_state_housing',      'popular_areas',            'Popular Areas',              'repeater',    NULL,                                                                   3, NULL),
	('wiki_state_housing',      'housing_note',             'Housing Notes',              'textarea',    NULL,                                                                   4, NULL),

	('wiki_state_community',    'community_groups',         'Community Groups & Forums',  'repeater',    NULL,                                                                   1, NULL),
	('wiki_state_community',    'events_meetups',           'Events & Meetups',           'repeater',    NULL,                                                                   2, NULL),
	('wiki_state_community',    'community_note',           'Community Notes',            'textarea',    NULL,                                                                   3, NULL),

	('wiki_state_other',        'local_tips',               'Local Tips',                 'textarea',    NULL,                                                                   1, NULL),
	('wiki_state_other',        'useful_links',             'Useful Links',               'repeater',    NULL,                                                                   2, NULL),

	-- ── University ───────────────────────────────────────────────────
	('wiki_university_overview',    'founded_year',             'Founded',                    'number',      NULL,                                                              1, NULL),
	('wiki_university_overview',    'institution_type',         'Institution Type',           'select',      '["Public","Private"]',                                             2, NULL),
	('wiki_university_overview',    'student_population',       'Student Population',         'number',      NULL,                                                              3, NULL),
	('wiki_university_overview',    'international_students_pct','International Students',    'percentage',  NULL,                                                              4, NULL),
	('wiki_university_overview',    'official_website',         'Official Website',           'url',         NULL,                                                              5, NULL),
	('wiki_university_overview',    'notable_for',              'Notable For',                'text',        NULL,                                                              6, NULL),

	('wiki_university_admissions',  'acceptance_rate',          'Acceptance Rate',            'percentage',  NULL,                                                              1, NULL),
	('wiki_university_admissions',  'application_fee',          'Application Fee',            'currency',    NULL,                                                              2, NULL),
	('wiki_university_admissions',  'application_deadline',     'Application Deadline',       'text',        NULL,                                                              3, NULL),
	('wiki_university_admissions',  'ielts_minimum',            'IELTS Minimum',              'number',      NULL,                                                              4, NULL),
	('wiki_university_admissions',  'toefl_minimum',            'TOEFL Minimum (iBT)',        'number',      NULL,                                                              5, NULL),
	('wiki_university_admissions',  'application_portal',       'Application Portal',         'url',         NULL,                                                              6, NULL),
	('wiki_university_admissions',  'requirements_note',        'Entry Requirements',         'textarea',    NULL,                                                              7, NULL),

	('wiki_university_programs',    'degree_levels',            'Degree Levels',              'multiselect', '["Bachelor","Master","PhD","Diploma","Certificate"]',              1, NULL),
	('wiki_university_programs',    'popular_programs',         'Popular Programs',           'repeater',    NULL,                                                              2, NULL),
	('wiki_university_programs',    'faculties_count',          'Faculties / Schools',        'number',      NULL,                                                              3, NULL),
	('wiki_university_programs',    'catalog_url',              'Programs Catalog',           'url',         NULL,                                                              4, NULL),

	('wiki_university_tuition_fees','tuition_ug_local',         'Undergrad — Domestic (yr)',  'price_range', NULL,                                                              1, 'tuition_fees'),
	('wiki_university_tuition_fees','tuition_ug_intl',          'Undergrad — Intl (yr)',      'price_range', NULL,                                                              2, 'tuition_fees'),
	('wiki_university_tuition_fees','tuition_pg_local',         'Postgrad — Domestic (yr)',   'price_range', NULL,                                                              3, 'tuition_fees'),
	('wiki_university_tuition_fees','tuition_pg_intl',          'Postgrad — Intl (yr)',       'price_range', NULL,                                                              4, 'tuition_fees'),
	('wiki_university_tuition_fees','fees_note',                'Fees & Payment Notes',       'textarea',    NULL,                                                              5, 'tuition_fees'),

	('wiki_university_scholarships','merit_scholarships',       'Merit Scholarships',         'boolean',     NULL,                                                              1, NULL),
	('wiki_university_scholarships','need_based_aid',           'Need-Based Aid',             'boolean',     NULL,                                                              2, NULL),
	('wiki_university_scholarships','scholarship_list',         'Available Scholarships',     'repeater',    NULL,                                                              3, NULL),
	('wiki_university_scholarships','scholarships_url',         'Scholarships Portal',        'url',         NULL,                                                              4, NULL),
	('wiki_university_scholarships','scholarships_note',        'Scholarship Notes',          'textarea',    NULL,                                                              5, NULL),

	('wiki_university_accommodation','on_campus_available',     'On-Campus Housing',          'boolean',     NULL,                                                              1, NULL),
	('wiki_university_accommodation','dorm_cost',               'Dorm Cost (monthly)',        'price_range', NULL,                                                              2, NULL),
	('wiki_university_accommodation','offcampus_avg_rent',      'Off-Campus Avg Rent',        'price_range', NULL,                                                              3, NULL),
	('wiki_university_accommodation','housing_process',         'Housing Application',        'textarea',    NULL,                                                              4, NULL),

	('wiki_university_campus_life', 'student_clubs',            'Student Clubs & Societies',  'repeater',    NULL,                                                              1, NULL),
	('wiki_university_campus_life', 'sports_facilities',        'Sports Facilities',          'repeater',    NULL,                                                              2, NULL),
	('wiki_university_campus_life', 'intl_student_office',      'Intl Student Office',        'boolean',     NULL,                                                              3, NULL),
	('wiki_university_campus_life', 'campus_note',              'Campus Life Notes',          'textarea',    NULL,                                                              4, NULL),

	('wiki_university_careers',     'career_services',          'Career Services',            'boolean',     NULL,                                                              1, NULL),
	('wiki_university_careers',     'employment_rate',          'Graduate Employment Rate',   'percentage',  NULL,                                                              2, NULL),
	('wiki_university_careers',     'internship_support',       'Internship Support',         'textarea',    NULL,                                                              3, NULL),
	('wiki_university_careers',     'top_employers',            'Top Employers',              'repeater',    NULL,                                                              4, NULL),

	('wiki_university_other',       'local_tips',               'Local Tips',                 'textarea',    NULL,                                                              1, NULL),
	('wiki_university_other',       'useful_links',             'Useful Links',               'repeater',    NULL,                                                              2, NULL)
) AS m(grp, key, label, field_type, options, position, section) ON m.grp = g.slug
ON CONFLICT (group_id, key) DO NOTHING;

-- 2. Field permissions matching the 000859/000860 convention:
--    every group can view, every non-guest group can edit.
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
CROSS JOIN user_groups ug
WHERE g.slug IN (
	'wiki_country_healthcare', 'wiki_country_housing', 'wiki_country_banking',
	'wiki_country_visa', 'wiki_country_community', 'wiki_country_other',
	'wiki_city_healthcare', 'wiki_city_community', 'wiki_city_other',
	'wiki_city_page_rent_prices',
	'wiki_state_overview', 'wiki_state_cost_of_living', 'wiki_state_healthcare',
	'wiki_state_housing', 'wiki_state_community', 'wiki_state_other',
	'wiki_university_overview', 'wiki_university_admissions', 'wiki_university_programs',
	'wiki_university_tuition_fees', 'wiki_university_scholarships', 'wiki_university_accommodation',
	'wiki_university_campus_life', 'wiki_university_careers', 'wiki_university_other'
)
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
