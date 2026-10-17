-- 000883_wiki_category_fields_seed.down.sql
-- Removes the seeded wiki category field definitions. Field permissions
-- are deleted by the custom_fields ON DELETE CASCADE.

BEGIN;

DELETE FROM custom_fields f
USING custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug IN (
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
  AND f.key IN (
	'healthcare_system','emergency_number','health_insurance_required','avg_gp_visit','hospitals_note','expat_health_tips',
	'rent_1br','rent_3br','deposit_norm','utilities_included','popular_expat_areas','tenant_rights_note',
	'major_banks','account_requirements','online_banking_quality','intl_transfer_options','banking_note',
	'common_visa_types','processing_time','visa_fee_range','work_permit_available','requirements_summary','residency_pathways',
	'expat_population','popular_expat_cities','community_groups','languages_spoken',
	'driving_side','power_plugs','public_holidays_note',
	'major_hospitals','clinics_note','insurance_note',
	'expat_neighborhoods','coworking_spaces','community_note',
	'local_tips','useful_links',
	'studio','one_bedroom','two_bedroom','three_bedroom','qf_best_value_area',
	'capital','population','major_cities','timezone','official_language','popular_places',
	'exp_accommodation','exp_food','exp_transportation','exp_utilities','exp_internet','exp_mobile','exp_entertainment','exp_health',
	'qf_currency','qf_monthly_range','qf_best_for',
	'healthcare_note','popular_areas','housing_note','events_meetups',
	'founded_year','institution_type','student_population','international_students_pct','official_website','notable_for',
	'acceptance_rate','application_fee','application_deadline','ielts_minimum','toefl_minimum','application_portal','requirements_note',
	'degree_levels','popular_programs','faculties_count','catalog_url',
	'tuition_ug_local','tuition_ug_intl','tuition_pg_local','tuition_pg_intl','fees_note',
	'merit_scholarships','need_based_aid','scholarship_list','scholarships_url','scholarships_note',
	'on_campus_available','dorm_cost','offcampus_avg_rent','housing_process',
	'student_clubs','sports_facilities','intl_student_office','campus_note',
	'career_services','employment_rate','internship_support','top_employers'
  );

COMMIT;
