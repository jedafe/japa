-- 001035_wiki_schema_realworld_coverage.up.sql
-- Snapshot of the full destination-wiki schema as configured on production:
-- every wiki type's category template, bound field groups, field definitions,
-- page templates (subpages), and the standard field-permission grant
-- (view: all user groups; edit: every non-guest group).
--
-- Covers both geo-bound types (country/state/city/education/attraction/
-- neighborhood) and entity-bound types (visa_route + 6 visa kinds, course,
-- scholarship, bank, embassy, program). Idempotent: all inserts/upserts are
-- keyed on the natural unique constraints so re-running is a no-op.
--
-- Ref: japaconnect-engineering/sections/80-destination-wiki.md

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. Field groups (all wiki_* groups bound to categories or page templates).
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_groups (name, slug, description, is_system) VALUES
    ('Facilities & Accessibility (attraction)', 'wiki_attraction_facilities', NULL, false),
    ('History & Background (attraction)', 'wiki_attraction_history', NULL, false),
    ('Nearby & Combine With (attraction)', 'wiki_attraction_nearby', NULL, false),
    ('Overview (attraction)', 'wiki_attraction_overview', NULL, true),
    ('Tickets & Pricing for {name} (attraction)', 'wiki_attraction_page_tickets', NULL, false),
    ('Plan Your Visit (attraction)', 'wiki_attraction_plan_your_visit', NULL, true),
    ('Account Types & Fees (bank)', 'wiki_bank_accounts_fees', NULL, true),
    ('Branch & ATM Coverage (bank)', 'wiki_bank_coverage', NULL, true),
    ('Digital & Mobile Banking (bank)', 'wiki_bank_digital_banking', NULL, false),
    ('Other (bank)', 'wiki_bank_other', NULL, false),
    ('Overview (bank)', 'wiki_bank_overview', NULL, true),
    ('Rates & Deposit Insurance (bank)', 'wiki_bank_rates', NULL, false),
    ('Remittance & FX (bank)', 'wiki_bank_remittance_fx', NULL, true),
    ('Requirements (Non-Resident) (bank)', 'wiki_bank_requirements', NULL, true),
    ('Application Process (business_visa)', 'wiki_business_visa_application_process', NULL, true),
    ('Dependents & Family (business_visa)', 'wiki_business_visa_dependents', NULL, false),
    ('Required Documents (business_visa)', 'wiki_business_visa_documents', NULL, false),
    ('Fees & Costs (business_visa)', 'wiki_business_visa_fees_costs', NULL, true),
    ('Other (business_visa)', 'wiki_business_visa_other', NULL, false),
    ('Overview (business_visa)', 'wiki_business_visa_overview', NULL, true),
    ('Processing Time (business_visa)', 'wiki_business_visa_processing_time', NULL, true),
    ('Refusals & Appeals (business_visa)', 'wiki_business_visa_refusals_appeals', NULL, false),
    ('Extension & Renewal (business_visa)', 'wiki_business_visa_renewal_extension', NULL, false),
    ('Requirements & Eligibility (business_visa)', 'wiki_business_visa_requirements', NULL, true),
    ('Rights & Restrictions (business_visa)', 'wiki_business_visa_rights_restrictions', NULL, false),
    ('Attractions (city)', 'wiki_city_attractions', NULL, true),
    ('Banking (city)', 'wiki_city_banking', NULL, true),
    ('Community (city)', 'wiki_city_community', NULL, true),
    ('Cost of Living (city)', 'wiki_city_cost_of_living', NULL, true),
    ('Embassies & Consulates in {name} (city)', 'wiki_city_embassies', NULL, false),
    ('Healthcare (city)', 'wiki_city_healthcare', NULL, true),
    ('Housing (city)', 'wiki_city_housing', NULL, true),
    ('Jobs & Salaries (city)', 'wiki_city_jobs', NULL, true),
    ('Lifestyle & Leisure (city)', 'wiki_city_lifestyle', NULL, false),
    ('{name} Neighborhoods (city)', 'wiki_city_neighborhoods', NULL, false),
    ('Other (city)', 'wiki_city_other', NULL, true),
    ('Overview (city)', 'wiki_city_overview', NULL, true),
    ('Clothing & Shoes Prices in {name} (city)', 'wiki_city_page_clothing_shoes', NULL, false),
    ('Rent in {name} (city)', 'wiki_city_page_rent_prices', NULL, false),
    ('Salaries & Financing in {name} (city)', 'wiki_city_page_salaries_financing', NULL, false),
    ('Prices (city)', 'wiki_city_prices', NULL, true),
    ('Childcare & School Fees (city)', 'wiki_city_prices_childcare', NULL, true),
    ('Food & Groceries Prices (city)', 'wiki_city_prices_food_groceries', NULL, true),
    ('Healthcare Prices (city)', 'wiki_city_prices_healthcare', NULL, true),
    ('Lifestyle Prices (city)', 'wiki_city_prices_lifestyle', NULL, true),
    ('Rent & Housing Prices (city)', 'wiki_city_prices_rent_housing', NULL, true),
    ('Restaurant Prices (city)', 'wiki_city_prices_restaurants', NULL, true),
    ('Transportation Prices (city)', 'wiki_city_prices_transportation', NULL, true),
    ('Utilities Prices (city)', 'wiki_city_prices_utilities', NULL, true),
    ('Safety & Crime (city)', 'wiki_city_safety', NULL, true),
    ('Schools & Education (city)', 'wiki_city_schools', NULL, true),
    ('Transportation & Commute (city)', 'wiki_city_transportation', NULL, true),
    ('Universities (city)', 'wiki_city_universities', NULL, false),
    ('Utilities & Internet (city)', 'wiki_city_utilities', NULL, true),
    ('Weather & Climate (city)', 'wiki_city_weather', NULL, true),
    ('Attractions (country)', 'wiki_country_attractions', NULL, true),
    ('Banking (country)', 'wiki_country_banking', NULL, true),
    ('Business & Entrepreneurship (country)', 'wiki_country_business', NULL, false),
    ('{name} Cities (country)', 'wiki_country_cities', NULL, true),
    ('Community (country)', 'wiki_country_community', NULL, true),
    ('Cost of Living (country)', 'wiki_country_cost_of_living', NULL, true),
    ('Culture & Lifestyle (country)', 'wiki_country_culture', NULL, false),
    ('Documents & Legalization (country)', 'wiki_country_documents', NULL, false),
    ('Education & Schools (country)', 'wiki_country_education', NULL, false),
    ('{name} Embassies (country)', 'wiki_country_embassies', NULL, false),
    ('Healthcare (country)', 'wiki_country_healthcare', NULL, true),
    ('Housing (country)', 'wiki_country_housing', NULL, true),
    ('Jobs & Economy (country)', 'wiki_country_jobs', NULL, true),
    ('Other (country)', 'wiki_country_other', NULL, true),
    ('Overview (country)', 'wiki_country_overview', NULL, true),
    ('Safety & Crime (country)', 'wiki_country_safety', NULL, true),
    ('Scholarships in {name} (country)', 'wiki_country_scholarships', NULL, false),
    ('Settling In (country)', 'wiki_country_settling_in', NULL, false),
    ('States (country)', 'wiki_country_states', NULL, false),
    ('Taxes (country)', 'wiki_country_taxes', NULL, true),
    ('Transportation (country)', 'wiki_country_transportation', NULL, false),
    ('Universities (country)', 'wiki_country_universities', NULL, false),
    ('Visa Routes (country)', 'wiki_country_visa', NULL, true),
    ('Weather & Climate (country)', 'wiki_country_weather', NULL, true),
    ('Career Outcomes (course)', 'wiki_course_career_outcomes', NULL, true),
    ('Curriculum & Structure (course)', 'wiki_course_curriculum', NULL, false),
    ('Study Mode & Schedule (course)', 'wiki_course_delivery', NULL, false),
    ('Entry Requirements (course)', 'wiki_course_entry_requirements', NULL, true),
    ('Other (course)', 'wiki_course_other', NULL, false),
    ('Overview (course)', 'wiki_course_overview', NULL, true),
    ('Tuition & Fees (course)', 'wiki_course_tuition_fees', NULL, true),
    ('Universities (course)', 'wiki_course_universities', NULL, true),
    ('Appointment Booking (embassy)', 'wiki_embassy_appointment_booking', NULL, true),
    ('Consular Services (embassy)', 'wiki_embassy_consular_services', NULL, true),
    ('Contact & Location (embassy)', 'wiki_embassy_contact_location', NULL, true),
    ('Holidays & Closures (embassy)', 'wiki_embassy_holidays', NULL, false),
    ('Jurisdiction & Coverage (embassy)', 'wiki_embassy_jurisdiction', NULL, false),
    ('Other (embassy)', 'wiki_embassy_other', NULL, false),
    ('Overview (embassy)', 'wiki_embassy_overview', NULL, true),
    ('Application Process (family_visa)', 'wiki_family_visa_application_process', NULL, true),
    ('Dependents & Family (family_visa)', 'wiki_family_visa_dependents', NULL, false),
    ('Required Documents (family_visa)', 'wiki_family_visa_documents', NULL, false),
    ('Fees & Costs (family_visa)', 'wiki_family_visa_fees_costs', NULL, true),
    ('Other (family_visa)', 'wiki_family_visa_other', NULL, false),
    ('Overview (family_visa)', 'wiki_family_visa_overview', NULL, true),
    ('Processing Time (family_visa)', 'wiki_family_visa_processing_time', NULL, true),
    ('Refusals & Appeals (family_visa)', 'wiki_family_visa_refusals_appeals', NULL, false),
    ('Extension & Renewal (family_visa)', 'wiki_family_visa_renewal_extension', NULL, false),
    ('Requirements & Eligibility (family_visa)', 'wiki_family_visa_requirements', NULL, true),
    ('Rights & Restrictions (family_visa)', 'wiki_family_visa_rights_restrictions', NULL, false),
    ('Amenities (neighborhood)', 'wiki_neighborhood_amenities', NULL, true),
    ('Who Lives Here (neighborhood)', 'wiki_neighborhood_demographics', NULL, false),
    ('Other (neighborhood)', 'wiki_neighborhood_other', NULL, false),
    ('Overview (neighborhood)', 'wiki_neighborhood_overview', NULL, true),
    ('Rent Prices (neighborhood)', 'wiki_neighborhood_rent_prices', NULL, true),
    ('Safety (neighborhood)', 'wiki_neighborhood_safety', NULL, true),
    ('Schools & Family (neighborhood)', 'wiki_neighborhood_schools_family', NULL, false),
    ('Getting Around (neighborhood)', 'wiki_neighborhood_transport', NULL, false),
    ('Application Process (pr_pathway)', 'wiki_pr_pathway_application_process', NULL, true),
    ('Dependents & Family (pr_pathway)', 'wiki_pr_pathway_dependents', NULL, false),
    ('Required Documents (pr_pathway)', 'wiki_pr_pathway_documents', NULL, false),
    ('Fees & Costs (pr_pathway)', 'wiki_pr_pathway_fees_costs', NULL, true),
    ('Other (pr_pathway)', 'wiki_pr_pathway_other', NULL, false),
    ('Overview (pr_pathway)', 'wiki_pr_pathway_overview', NULL, true),
    ('Processing Time (pr_pathway)', 'wiki_pr_pathway_processing_time', NULL, true),
    ('Refusals & Appeals (pr_pathway)', 'wiki_pr_pathway_refusals_appeals', NULL, false),
    ('Extension & Renewal (pr_pathway)', 'wiki_pr_pathway_renewal_extension', NULL, false),
    ('Requirements & Eligibility (pr_pathway)', 'wiki_pr_pathway_requirements', NULL, true),
    ('Rights & Restrictions (pr_pathway)', 'wiki_pr_pathway_rights_restrictions', NULL, false),
    ('Career Outcomes (program)', 'wiki_program_career_outcomes', NULL, true),
    ('Curriculum (program)', 'wiki_program_curriculum', NULL, false),
    ('Entry Requirements (program)', 'wiki_program_entry_requirements', NULL, false),
    ('Other (program)', 'wiki_program_other', NULL, false),
    ('Overview (program)', 'wiki_program_overview', NULL, true),
    ('Tuition & Fees (program)', 'wiki_program_tuition_fees', NULL, false),
    ('Universities (program)', 'wiki_program_universities', NULL, true),
    ('Application & Deadlines (scholarship)', 'wiki_scholarship_application_deadlines', NULL, true),
    ('Coverage & Benefits (scholarship)', 'wiki_scholarship_coverage_benefits', NULL, true),
    ('Eligibility (scholarship)', 'wiki_scholarship_eligibility', NULL, true),
    ('Other (scholarship)', 'wiki_scholarship_other', NULL, false),
    ('Overview (scholarship)', 'wiki_scholarship_overview', NULL, true),
    ('Selection & Renewal (scholarship)', 'wiki_scholarship_selection', NULL, false),
    ('Attractions (state)', 'wiki_state_attractions', NULL, true),
    ('Banking (state)', 'wiki_state_banking', NULL, true),
    ('Cities (state)', 'wiki_state_cities', NULL, false),
    ('Community (state)', 'wiki_state_community', NULL, true),
    ('Cost of Living (state)', 'wiki_state_cost_of_living', NULL, true),
    ('Healthcare (state)', 'wiki_state_healthcare', NULL, true),
    ('Housing (state)', 'wiki_state_housing', NULL, true),
    ('Jobs & Economy (state)', 'wiki_state_jobs', NULL, true),
    ('Other (state)', 'wiki_state_other', NULL, true),
    ('Overview (state)', 'wiki_state_overview', NULL, true),
    ('Safety & Crime (state)', 'wiki_state_safety', NULL, true),
    ('Schools & Education (state)', 'wiki_state_schools', NULL, true),
    ('Taxes (state)', 'wiki_state_taxes', NULL, true),
    ('Transportation (state)', 'wiki_state_transportation', NULL, false),
    ('Universities (state)', 'wiki_state_universities', NULL, false),
    ('Weather & Climate (state)', 'wiki_state_weather', NULL, true),
    ('Application Process (student_visa)', 'wiki_student_visa_application_process', NULL, true),
    ('Dependents & Family (student_visa)', 'wiki_student_visa_dependents', NULL, false),
    ('Required Documents (student_visa)', 'wiki_student_visa_documents', NULL, false),
    ('Fees & Costs (student_visa)', 'wiki_student_visa_fees_costs', NULL, true),
    ('Other (student_visa)', 'wiki_student_visa_other', NULL, false),
    ('Overview (student_visa)', 'wiki_student_visa_overview', NULL, true),
    ('Processing Time (student_visa)', 'wiki_student_visa_processing_time', NULL, true),
    ('Refusals & Appeals (student_visa)', 'wiki_student_visa_refusals_appeals', NULL, false),
    ('Extension & Renewal (student_visa)', 'wiki_student_visa_renewal_extension', NULL, false),
    ('Requirements & Eligibility (student_visa)', 'wiki_student_visa_requirements', NULL, true),
    ('Rights & Restrictions (student_visa)', 'wiki_student_visa_rights_restrictions', NULL, false),
    ('Application Process (tourist_visa)', 'wiki_tourist_visa_application_process', NULL, true),
    ('Dependents & Family (tourist_visa)', 'wiki_tourist_visa_dependents', NULL, false),
    ('Required Documents (tourist_visa)', 'wiki_tourist_visa_documents', NULL, false),
    ('Fees & Costs (tourist_visa)', 'wiki_tourist_visa_fees_costs', NULL, true),
    ('Other (tourist_visa)', 'wiki_tourist_visa_other', NULL, false),
    ('Overview (tourist_visa)', 'wiki_tourist_visa_overview', NULL, true),
    ('Processing Time (tourist_visa)', 'wiki_tourist_visa_processing_time', NULL, true),
    ('Refusals & Appeals (tourist_visa)', 'wiki_tourist_visa_refusals_appeals', NULL, false),
    ('Extension & Renewal (tourist_visa)', 'wiki_tourist_visa_renewal_extension', NULL, false),
    ('Requirements & Eligibility (tourist_visa)', 'wiki_tourist_visa_requirements', NULL, true),
    ('Rights & Restrictions (tourist_visa)', 'wiki_tourist_visa_rights_restrictions', NULL, false),
    ('Accommodation (university)', 'wiki_university_accommodation', NULL, true),
    ('Admissions (university)', 'wiki_university_admissions', NULL, true),
    ('Notable Alumni (education)', 'wiki_university_alumni', NULL, false),
    ('Campus Life (university)', 'wiki_university_campus_life', NULL, true),
    ('Careers (university)', 'wiki_university_careers', NULL, true),
    ('Campus Facilities (education)', 'wiki_university_facilities', NULL, false),
    ('International Students (education)', 'wiki_university_intl_students', NULL, false),
    ('Other (university)', 'wiki_university_other', NULL, true),
    ('Overview (university)', 'wiki_university_overview', NULL, true),
    ('Program Fees for {name} (education)', 'wiki_university_page_program_fees', NULL, false),
    ('Programs (university)', 'wiki_university_programs', NULL, true),
    ('Rankings & Reputation (university)', 'wiki_university_rankings', NULL, true),
    ('Scholarships (university)', 'wiki_university_scholarships', NULL, true),
    ('Tuition & Fees (university)', 'wiki_university_tuition_fees', NULL, true),
    ('Application Process (visa_route)', 'wiki_visa_route_application_process', NULL, true),
    ('Dependents & Family (visa_route)', 'wiki_visa_route_dependents', NULL, false),
    ('Required Documents (visa_route)', 'wiki_visa_route_documents', NULL, false),
    ('Fees & Costs (visa_route)', 'wiki_visa_route_fees_costs', NULL, true),
    ('Other (visa_route)', 'wiki_visa_route_other', NULL, false),
    ('Overview (visa_route)', 'wiki_visa_route_overview', NULL, true),
    ('Processing Time (visa_route)', 'wiki_visa_route_processing_time', NULL, true),
    ('Refusals & Appeals (visa_route)', 'wiki_visa_route_refusals_appeals', NULL, false),
    ('Extension & Renewal (visa_route)', 'wiki_visa_route_renewal_extension', NULL, false),
    ('Requirements & Eligibility (visa_route)', 'wiki_visa_route_requirements', NULL, true),
    ('Rights & Restrictions (visa_route)', 'wiki_visa_route_rights_restrictions', NULL, false),
    ('Application Process (work_visa)', 'wiki_work_visa_application_process', NULL, true),
    ('Dependents & Family (work_visa)', 'wiki_work_visa_dependents', NULL, false),
    ('Required Documents (work_visa)', 'wiki_work_visa_documents', NULL, false),
    ('Fees & Costs (work_visa)', 'wiki_work_visa_fees_costs', NULL, true),
    ('Other (work_visa)', 'wiki_work_visa_other', NULL, false),
    ('Overview (work_visa)', 'wiki_work_visa_overview', NULL, true),
    ('Processing Time (work_visa)', 'wiki_work_visa_processing_time', NULL, true),
    ('Refusals & Appeals (work_visa)', 'wiki_work_visa_refusals_appeals', NULL, false),
    ('Extension & Renewal (work_visa)', 'wiki_work_visa_renewal_extension', NULL, false),
    ('Requirements & Eligibility (work_visa)', 'wiki_work_visa_requirements', NULL, true),
    ('Rights & Restrictions (work_visa)', 'wiki_work_visa_rights_restrictions', NULL, false)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name;

-- ---------------------------------------------------------------------------
-- 2. Class assignments: each group bound to its wiki type's entity class
--    'space_type:<type_slug>' (idempotent NOT EXISTS guard).
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
SELECT g.id, m.ec, NULL::uuid, 0
FROM custom_field_groups g
JOIN (VALUES
    ('wiki_attraction_facilities','space_type:attraction'),
    ('wiki_attraction_history','space_type:attraction'),
    ('wiki_attraction_nearby','space_type:attraction'),
    ('wiki_attraction_overview','space_type:attraction'),
    ('wiki_attraction_page_tickets','space_type:attraction'),
    ('wiki_attraction_plan_your_visit','space_type:attraction'),
    ('wiki_bank_accounts_fees','space_type:bank'),
    ('wiki_bank_coverage','space_type:bank'),
    ('wiki_bank_digital_banking','space_type:bank'),
    ('wiki_bank_other','space_type:bank'),
    ('wiki_bank_overview','space_type:bank'),
    ('wiki_bank_rates','space_type:bank'),
    ('wiki_bank_remittance_fx','space_type:bank'),
    ('wiki_bank_requirements','space_type:bank'),
    ('wiki_business_visa_application_process','space_type:business_visa'),
    ('wiki_business_visa_dependents','space_type:business_visa'),
    ('wiki_business_visa_documents','space_type:business_visa'),
    ('wiki_business_visa_fees_costs','space_type:business_visa'),
    ('wiki_business_visa_other','space_type:business_visa'),
    ('wiki_business_visa_overview','space_type:business_visa'),
    ('wiki_business_visa_processing_time','space_type:business_visa'),
    ('wiki_business_visa_refusals_appeals','space_type:business_visa'),
    ('wiki_business_visa_renewal_extension','space_type:business_visa'),
    ('wiki_business_visa_requirements','space_type:business_visa'),
    ('wiki_business_visa_rights_restrictions','space_type:business_visa'),
    ('wiki_city_attractions','space_type:city'),
    ('wiki_city_banking','space_type:city'),
    ('wiki_city_community','space_type:city'),
    ('wiki_city_cost_of_living','space_type:city'),
    ('wiki_city_embassies','space_type:city'),
    ('wiki_city_healthcare','space_type:city'),
    ('wiki_city_housing','space_type:city'),
    ('wiki_city_jobs','space_type:city'),
    ('wiki_city_lifestyle','space_type:city'),
    ('wiki_city_neighborhoods','space_type:city'),
    ('wiki_city_other','space_type:city'),
    ('wiki_city_overview','space_type:city'),
    ('wiki_city_page_clothing_shoes','space_type:city'),
    ('wiki_city_page_rent_prices','space_type:city'),
    ('wiki_city_page_salaries_financing','space_type:city'),
    ('wiki_city_prices','space_type:city'),
    ('wiki_city_prices_childcare','space_type:city'),
    ('wiki_city_prices_food_groceries','space_type:city'),
    ('wiki_city_prices_healthcare','space_type:city'),
    ('wiki_city_prices_lifestyle','space_type:city'),
    ('wiki_city_prices_rent_housing','space_type:city'),
    ('wiki_city_prices_restaurants','space_type:city'),
    ('wiki_city_prices_transportation','space_type:city'),
    ('wiki_city_prices_utilities','space_type:city'),
    ('wiki_city_safety','space_type:city'),
    ('wiki_city_schools','space_type:city'),
    ('wiki_city_transportation','space_type:city'),
    ('wiki_city_universities','space_type:city'),
    ('wiki_city_utilities','space_type:city'),
    ('wiki_city_weather','space_type:city'),
    ('wiki_country_attractions','space_type:country'),
    ('wiki_country_banking','space_type:country'),
    ('wiki_country_business','space_type:country'),
    ('wiki_country_cities','space_type:country'),
    ('wiki_country_community','space_type:country'),
    ('wiki_country_cost_of_living','space_type:country'),
    ('wiki_country_culture','space_type:country'),
    ('wiki_country_documents','space_type:country'),
    ('wiki_country_education','space_type:country'),
    ('wiki_country_embassies','space_type:country'),
    ('wiki_country_healthcare','space_type:country'),
    ('wiki_country_housing','space_type:country'),
    ('wiki_country_jobs','space_type:country'),
    ('wiki_country_other','space_type:country'),
    ('wiki_country_overview','space_type:country'),
    ('wiki_country_safety','space_type:country'),
    ('wiki_country_scholarships','space_type:country'),
    ('wiki_country_settling_in','space_type:country'),
    ('wiki_country_states','space_type:country'),
    ('wiki_country_taxes','space_type:country'),
    ('wiki_country_transportation','space_type:country'),
    ('wiki_country_universities','space_type:country'),
    ('wiki_country_visa','space_type:country'),
    ('wiki_country_weather','space_type:country'),
    ('wiki_course_career_outcomes','space_type:course'),
    ('wiki_course_curriculum','space_type:course'),
    ('wiki_course_delivery','space_type:course'),
    ('wiki_course_entry_requirements','space_type:course'),
    ('wiki_course_other','space_type:course'),
    ('wiki_course_overview','space_type:course'),
    ('wiki_course_tuition_fees','space_type:course'),
    ('wiki_course_universities','space_type:course'),
    ('wiki_embassy_appointment_booking','space_type:embassy'),
    ('wiki_embassy_consular_services','space_type:embassy'),
    ('wiki_embassy_contact_location','space_type:embassy'),
    ('wiki_embassy_holidays','space_type:embassy'),
    ('wiki_embassy_jurisdiction','space_type:embassy'),
    ('wiki_embassy_other','space_type:embassy'),
    ('wiki_embassy_overview','space_type:embassy'),
    ('wiki_family_visa_application_process','space_type:family_visa'),
    ('wiki_family_visa_dependents','space_type:family_visa'),
    ('wiki_family_visa_documents','space_type:family_visa'),
    ('wiki_family_visa_fees_costs','space_type:family_visa'),
    ('wiki_family_visa_other','space_type:family_visa'),
    ('wiki_family_visa_overview','space_type:family_visa'),
    ('wiki_family_visa_processing_time','space_type:family_visa'),
    ('wiki_family_visa_refusals_appeals','space_type:family_visa'),
    ('wiki_family_visa_renewal_extension','space_type:family_visa'),
    ('wiki_family_visa_requirements','space_type:family_visa'),
    ('wiki_family_visa_rights_restrictions','space_type:family_visa'),
    ('wiki_neighborhood_amenities','space_type:neighborhood'),
    ('wiki_neighborhood_demographics','space_type:neighborhood'),
    ('wiki_neighborhood_other','space_type:neighborhood'),
    ('wiki_neighborhood_overview','space_type:neighborhood'),
    ('wiki_neighborhood_rent_prices','space_type:neighborhood'),
    ('wiki_neighborhood_safety','space_type:neighborhood'),
    ('wiki_neighborhood_schools_family','space_type:neighborhood'),
    ('wiki_neighborhood_transport','space_type:neighborhood'),
    ('wiki_pr_pathway_application_process','space_type:pr_pathway'),
    ('wiki_pr_pathway_dependents','space_type:pr_pathway'),
    ('wiki_pr_pathway_documents','space_type:pr_pathway'),
    ('wiki_pr_pathway_fees_costs','space_type:pr_pathway'),
    ('wiki_pr_pathway_other','space_type:pr_pathway'),
    ('wiki_pr_pathway_overview','space_type:pr_pathway'),
    ('wiki_pr_pathway_processing_time','space_type:pr_pathway'),
    ('wiki_pr_pathway_refusals_appeals','space_type:pr_pathway'),
    ('wiki_pr_pathway_renewal_extension','space_type:pr_pathway'),
    ('wiki_pr_pathway_requirements','space_type:pr_pathway'),
    ('wiki_pr_pathway_rights_restrictions','space_type:pr_pathway'),
    ('wiki_program_career_outcomes','space_type:program'),
    ('wiki_program_curriculum','space_type:program'),
    ('wiki_program_entry_requirements','space_type:program'),
    ('wiki_program_other','space_type:program'),
    ('wiki_program_overview','space_type:program'),
    ('wiki_program_tuition_fees','space_type:program'),
    ('wiki_program_universities','space_type:program'),
    ('wiki_scholarship_application_deadlines','space_type:scholarship'),
    ('wiki_scholarship_coverage_benefits','space_type:scholarship'),
    ('wiki_scholarship_eligibility','space_type:scholarship'),
    ('wiki_scholarship_other','space_type:scholarship'),
    ('wiki_scholarship_overview','space_type:scholarship'),
    ('wiki_scholarship_selection','space_type:scholarship'),
    ('wiki_state_attractions','space_type:state'),
    ('wiki_state_banking','space_type:state'),
    ('wiki_state_cities','space_type:state'),
    ('wiki_state_community','space_type:state'),
    ('wiki_state_cost_of_living','space_type:state'),
    ('wiki_state_healthcare','space_type:state'),
    ('wiki_state_housing','space_type:state'),
    ('wiki_state_jobs','space_type:state'),
    ('wiki_state_other','space_type:state'),
    ('wiki_state_overview','space_type:state'),
    ('wiki_state_safety','space_type:state'),
    ('wiki_state_schools','space_type:state'),
    ('wiki_state_taxes','space_type:state'),
    ('wiki_state_transportation','space_type:state'),
    ('wiki_state_universities','space_type:state'),
    ('wiki_state_weather','space_type:state'),
    ('wiki_student_visa_application_process','space_type:student_visa'),
    ('wiki_student_visa_dependents','space_type:student_visa'),
    ('wiki_student_visa_documents','space_type:student_visa'),
    ('wiki_student_visa_fees_costs','space_type:student_visa'),
    ('wiki_student_visa_other','space_type:student_visa'),
    ('wiki_student_visa_overview','space_type:student_visa'),
    ('wiki_student_visa_processing_time','space_type:student_visa'),
    ('wiki_student_visa_refusals_appeals','space_type:student_visa'),
    ('wiki_student_visa_renewal_extension','space_type:student_visa'),
    ('wiki_student_visa_requirements','space_type:student_visa'),
    ('wiki_student_visa_rights_restrictions','space_type:student_visa'),
    ('wiki_tourist_visa_application_process','space_type:tourist_visa'),
    ('wiki_tourist_visa_dependents','space_type:tourist_visa'),
    ('wiki_tourist_visa_documents','space_type:tourist_visa'),
    ('wiki_tourist_visa_fees_costs','space_type:tourist_visa'),
    ('wiki_tourist_visa_other','space_type:tourist_visa'),
    ('wiki_tourist_visa_overview','space_type:tourist_visa'),
    ('wiki_tourist_visa_processing_time','space_type:tourist_visa'),
    ('wiki_tourist_visa_refusals_appeals','space_type:tourist_visa'),
    ('wiki_tourist_visa_renewal_extension','space_type:tourist_visa'),
    ('wiki_tourist_visa_requirements','space_type:tourist_visa'),
    ('wiki_tourist_visa_rights_restrictions','space_type:tourist_visa'),
    ('wiki_university_accommodation','space_type:education'),
    ('wiki_university_admissions','space_type:education'),
    ('wiki_university_alumni','space_type:education'),
    ('wiki_university_campus_life','space_type:education'),
    ('wiki_university_careers','space_type:education'),
    ('wiki_university_facilities','space_type:education'),
    ('wiki_university_intl_students','space_type:education'),
    ('wiki_university_other','space_type:education'),
    ('wiki_university_overview','space_type:education'),
    ('wiki_university_page_program_fees','space_type:education'),
    ('wiki_university_programs','space_type:education'),
    ('wiki_university_rankings','space_type:education'),
    ('wiki_university_scholarships','space_type:education'),
    ('wiki_university_tuition_fees','space_type:education'),
    ('wiki_visa_route_application_process','space_type:visa_route'),
    ('wiki_visa_route_dependents','space_type:visa_route'),
    ('wiki_visa_route_documents','space_type:visa_route'),
    ('wiki_visa_route_fees_costs','space_type:visa_route'),
    ('wiki_visa_route_other','space_type:visa_route'),
    ('wiki_visa_route_overview','space_type:visa_route'),
    ('wiki_visa_route_processing_time','space_type:visa_route'),
    ('wiki_visa_route_refusals_appeals','space_type:visa_route'),
    ('wiki_visa_route_renewal_extension','space_type:visa_route'),
    ('wiki_visa_route_requirements','space_type:visa_route'),
    ('wiki_visa_route_rights_restrictions','space_type:visa_route'),
    ('wiki_work_visa_application_process','space_type:work_visa'),
    ('wiki_work_visa_dependents','space_type:work_visa'),
    ('wiki_work_visa_documents','space_type:work_visa'),
    ('wiki_work_visa_fees_costs','space_type:work_visa'),
    ('wiki_work_visa_other','space_type:work_visa'),
    ('wiki_work_visa_overview','space_type:work_visa'),
    ('wiki_work_visa_processing_time','space_type:work_visa'),
    ('wiki_work_visa_refusals_appeals','space_type:work_visa'),
    ('wiki_work_visa_renewal_extension','space_type:work_visa'),
    ('wiki_work_visa_requirements','space_type:work_visa'),
    ('wiki_work_visa_rights_restrictions','space_type:work_visa')
) AS m(gs, ec) ON g.slug = m.gs
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_group_assignments a
    WHERE a.group_id = g.id AND a.entity_class = m.ec AND a.entity_id IS NULL
);

-- ---------------------------------------------------------------------------
-- 3. Category templates (upsert on (space_type_id, slug)).
-- ---------------------------------------------------------------------------
INSERT INTO wiki_type_categories
    (space_type_id, slug, category, label, field_group_id, position, is_system, rollup, icon)
SELECT m.tid::uuid, m.slug, m.category, m.label, g.id, m.position, m.is_system, m.rollup::jsonb, m.icon
FROM (VALUES
    ('00000000-0000-0000-0000-000000000071','facilities','facilities','Facilities & Accessibility','wiki_attraction_facilities',4,false,NULL,'accessibility'),
    ('00000000-0000-0000-0000-000000000071','history','history','History & Background','wiki_attraction_history',3,false,NULL,'scroll'),
    ('00000000-0000-0000-0000-000000000071','nearby','nearby','Nearby & Combine With','wiki_attraction_nearby',5,false,NULL,'map-pin'),
    ('00000000-0000-0000-0000-000000000071','overview','overview','Overview','wiki_attraction_overview',1,true,NULL,'info'),
    ('00000000-0000-0000-0000-000000000071','plan-your-visit','plan_your_visit','Plan Your Visit','wiki_attraction_plan_your_visit',2,true,NULL,'map'),
    ('00000000-0000-0000-0000-00000000007c','accounts-fees','accounts_fees','Account Types & Fees','wiki_bank_accounts_fees',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007c','coverage','coverage','Branch & ATM Coverage','wiki_bank_coverage',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007c','digital','digital_banking','Digital & Mobile Banking','wiki_bank_digital_banking',6,false,NULL,'smartphone'),
    ('00000000-0000-0000-0000-00000000007c','other','other','Other','wiki_bank_other',7,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-00000000007c','overview','overview','Overview','wiki_bank_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007c','rates','rates','Rates & Deposit Insurance','wiki_bank_rates',7,false,NULL,'percent'),
    ('00000000-0000-0000-0000-00000000007c','remittance-fx','remittance_fx','Remittance & FX','wiki_bank_remittance_fx',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007c','requirements','requirements','Requirements (Non-Resident)','wiki_bank_requirements',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','application-process','application_process','Application Process','wiki_business_visa_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','dependents','dependents','Dependents & Family','wiki_business_visa_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000078','documents','documents','Required Documents','wiki_business_visa_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000078','fees-costs','fees_costs','Fees & Costs','wiki_business_visa_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','other','other','Other','wiki_business_visa_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000078','overview','overview','Overview','wiki_business_visa_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','processing-time','processing_time','Processing Time','wiki_business_visa_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','refusals','refusals_appeals','Refusals & Appeals','wiki_business_visa_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000078','renewal','renewal_extension','Extension & Renewal','wiki_business_visa_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000078','requirements','requirements','Requirements & Eligibility','wiki_business_visa_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000078','rights','rights_restrictions','Rights & Restrictions','wiki_business_visa_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-000000000062','attractions','attractions','{name} Attractions','wiki_city_attractions',7,true,'{"mode":"directory","levels":["attraction"],"enabled":true}'::jsonb,'landmark'),
    ('00000000-0000-0000-0000-000000000062','banking','banking','Banking','wiki_city_banking',14,true,'{"mode":"directory","levels":["bank"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000062','community','community','Community','wiki_city_community',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','cost_of_living','Cost of Living','wiki_city_cost_of_living',2,true,'{"mode":"pages","pages":["food-groceries-prices","restaurant-prices","transportation-prices","utilities-prices","lifestyle-prices","rent-housing-prices","healthcare-prices","childcare-school-fees","clothing-shoes","salaries-financing"],"enabled":true,"metrics":["min","avg","max"],"include_self":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000062','embassies','embassies','Embassies & Consulates in {name}','wiki_city_embassies',16,false,'{"mode":"directory","levels":["embassy"],"enabled":true}'::jsonb,'building-2'),
    ('00000000-0000-0000-0000-000000000062','healthcare','healthcare','Healthcare','wiki_city_healthcare',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','housing','housing','Housing','wiki_city_housing',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','jobs','jobs','Jobs & Salaries','wiki_city_jobs',9,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','lifestyle','lifestyle','Lifestyle & Leisure','wiki_city_lifestyle',16,false,NULL,'music'),
    ('00000000-0000-0000-0000-000000000062','neighborhoods','neighborhoods','{name} Neighborhoods','wiki_city_neighborhoods',15,false,'{"mode":"directory","levels":["neighborhood"],"enabled":true}'::jsonb,'map-pin'),
    ('00000000-0000-0000-0000-000000000062','other','other','Other','wiki_city_other',6,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','overview','overview','Overview','wiki_city_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','safety','safety','Safety & Crime','wiki_city_safety',8,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','schools','schools','{name} Schools & Education','wiki_city_schools',10,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','transportation','transportation','Transportation & Commute','wiki_city_transportation',11,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','universities','universities','{name} Universities','wiki_city_universities',10,false,'{"mode":"directory","levels":["university","education"],"enabled":true,"metrics":[]}'::jsonb,'graduation-cap'),
    ('00000000-0000-0000-0000-000000000062','utilities','utilities','Utilities & Internet','wiki_city_utilities',12,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000062','weather','weather','Weather & Climate','wiki_city_weather',13,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','attractions','attractions','{name} Attractions','wiki_country_attractions',9,true,'{"mode":"directory","levels":["attraction"],"enabled":true}'::jsonb,'landmark'),
    ('00000000-0000-0000-0000-000000000060','banking','banking','Banking','wiki_country_banking',5,true,'{"mode":"directory","levels":["bank"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000060','business','business','Business & Entrepreneurship','wiki_country_business',19,false,NULL,'briefcase'),
    ('00000000-0000-0000-0000-000000000060','cities','cities','{name} Cities','wiki_country_cities',15,true,'{"mode":"directory","levels":["city"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000060','community','community','Community','wiki_country_community',6,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','cost-of-living','cost_of_living','Cost of Living','wiki_country_cost_of_living',3,true,'{"levels":["state","city"],"enabled":true,"metrics":["min","avg","max"],"section_limit":10}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000060','culture','culture','Culture & Lifestyle','wiki_country_culture',18,false,NULL,'palette'),
    ('00000000-0000-0000-0000-000000000060','documents','documents','Documents & Legalization','wiki_country_documents',23,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000060','education','education','Education & Schools','wiki_country_education',16,false,NULL,'book-open'),
    ('00000000-0000-0000-0000-000000000060','embassies','embassies','{name} Embassies','wiki_country_embassies',20,false,'{"mode":"directory","levels":["embassy"],"enabled":true}'::jsonb,'building-2'),
    ('00000000-0000-0000-0000-000000000060','healthcare','healthcare','Healthcare','wiki_country_healthcare',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','housing','housing','Housing','wiki_country_housing',7,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','jobs','jobs','Jobs & Economy','wiki_country_jobs',11,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','other','other','Other','wiki_country_other',8,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','overview','overview','Overview','wiki_country_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','safety','safety','Safety & Crime','wiki_country_safety',10,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','scholarships','scholarships','Scholarships in {name}','wiki_country_scholarships',21,false,'{"mode":"directory","levels":["scholarship"],"enabled":true}'::jsonb,'award'),
    ('00000000-0000-0000-0000-000000000060','settling-in','settling_in','Settling In','wiki_country_settling_in',22,false,NULL,'map-pin'),
    ('00000000-0000-0000-0000-000000000060','states','states','States','wiki_country_states',0,false,'{"mode":"directory","levels":["state"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000060','taxes','taxes','Taxes','wiki_country_taxes',12,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000060','transportation','transportation','Transportation','wiki_country_transportation',17,false,NULL,'train'),
    ('00000000-0000-0000-0000-000000000060','universities','universities','{name} Universities','wiki_country_universities',11,false,'{"mode":"directory","levels":["university","education"],"enabled":true}'::jsonb,'graduation-cap'),
    ('00000000-0000-0000-0000-000000000060','visa-routes','visa','{name} Visa Routes','wiki_country_visa',2,true,'{"mode":"directory","levels":["visa_route"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000060','weather','weather','Weather & Climate','wiki_country_weather',13,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007a','career-outcomes','career_outcomes','Career Outcomes','wiki_course_career_outcomes',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007a','curriculum','curriculum','Curriculum & Structure','wiki_course_curriculum',6,false,NULL,'list'),
    ('00000000-0000-0000-0000-00000000007a','delivery','delivery','Study Mode & Schedule','wiki_course_delivery',7,false,NULL,'calendar'),
    ('00000000-0000-0000-0000-00000000007a','entry-requirements','entry_requirements','Entry Requirements','wiki_course_entry_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007a','other','other','Other','wiki_course_other',8,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-00000000007a','overview','overview','Overview','wiki_course_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007a','tuition-fees','tuition_fees','Tuition & Fees','wiki_course_tuition_fees',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007a','universities','universities','Universities','wiki_course_universities',5,true,'{"mode":"related","enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000066','accommodation','accommodation','Accommodation','wiki_university_accommodation',6,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','admissions','admissions','Admissions','wiki_university_admissions',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','alumni','alumni','Notable Alumni','wiki_university_alumni',12,false,NULL,'award'),
    ('00000000-0000-0000-0000-000000000066','campus-life','campus_life','Campus Life','wiki_university_campus_life',7,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','careers','careers','Careers','wiki_university_careers',8,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','facilities','facilities','Campus Facilities','wiki_university_facilities',11,false,NULL,'building'),
    ('00000000-0000-0000-0000-000000000066','intl-students','intl_students','International Students','wiki_university_intl_students',13,false,NULL,'globe'),
    ('00000000-0000-0000-0000-000000000066','other','other','Other','wiki_university_other',9,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','overview','overview','Overview','wiki_university_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','programs','programs','Programs','wiki_university_programs',3,true,'{"mode":"directory","levels":["course"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000066','rankings','rankings','Rankings & Reputation','wiki_university_rankings',10,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','scholarships','scholarships','Scholarships','wiki_university_scholarships',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000066','tuition-fees','tuition_fees','Tuition & Fees','wiki_university_tuition_fees',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007d','appointment-booking','appointment_booking','Appointment Booking','wiki_embassy_appointment_booking',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007d','consular-services','consular_services','Consular Services','wiki_embassy_consular_services',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007d','contact-location','contact_location','Contact & Location','wiki_embassy_contact_location',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007d','holidays','holidays','Holidays & Closures','wiki_embassy_holidays',6,false,NULL,'calendar'),
    ('00000000-0000-0000-0000-00000000007d','jurisdiction','jurisdiction','Jurisdiction & Coverage','wiki_embassy_jurisdiction',5,false,NULL,'map'),
    ('00000000-0000-0000-0000-00000000007d','other','other','Other','wiki_embassy_other',6,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-00000000007d','overview','overview','Overview','wiki_embassy_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','application-process','application_process','Application Process','wiki_family_visa_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','dependents','dependents','Dependents & Family','wiki_family_visa_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000076','documents','documents','Required Documents','wiki_family_visa_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000076','fees-costs','fees_costs','Fees & Costs','wiki_family_visa_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','other','other','Other','wiki_family_visa_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000076','overview','overview','Overview','wiki_family_visa_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','processing-time','processing_time','Processing Time','wiki_family_visa_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','refusals','refusals_appeals','Refusals & Appeals','wiki_family_visa_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000076','renewal','renewal_extension','Extension & Renewal','wiki_family_visa_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000076','requirements','requirements','Requirements & Eligibility','wiki_family_visa_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000076','rights','rights_restrictions','Rights & Restrictions','wiki_family_visa_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-000000000072','amenities','amenities','Amenities','wiki_neighborhood_amenities',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000072','demographics','demographics','Who Lives Here','wiki_neighborhood_demographics',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000072','other','other','Other','wiki_neighborhood_other',7,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000072','overview','overview','Overview','wiki_neighborhood_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000072','rent-prices','rent_prices','Rent Prices','wiki_neighborhood_rent_prices',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000072','safety','safety','Safety','wiki_neighborhood_safety',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000072','schools','schools_family','Schools & Family','wiki_neighborhood_schools_family',6,false,NULL,'baby'),
    ('00000000-0000-0000-0000-000000000072','transport','transport','Getting Around','wiki_neighborhood_transport',5,false,NULL,'bus'),
    ('00000000-0000-0000-0000-000000000079','application-process','application_process','Application Process','wiki_pr_pathway_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000079','dependents','dependents','Dependents & Family','wiki_pr_pathway_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000079','documents','documents','Required Documents','wiki_pr_pathway_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000079','fees-costs','fees_costs','Fees & Costs','wiki_pr_pathway_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000079','other','other','Other','wiki_pr_pathway_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000079','overview','overview','Overview','wiki_pr_pathway_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000079','processing-time','processing_time','Processing Time','wiki_pr_pathway_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000079','refusals','refusals_appeals','Refusals & Appeals','wiki_pr_pathway_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000079','renewal','renewal_extension','Extension & Renewal','wiki_pr_pathway_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000079','requirements','requirements','Requirements & Eligibility','wiki_pr_pathway_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000079','rights','rights_restrictions','Rights & Restrictions','wiki_pr_pathway_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-00000000007e','career-outcomes','career_outcomes','Career Outcomes','wiki_program_career_outcomes',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007e','curriculum','curriculum','Curriculum','wiki_program_curriculum',5,false,NULL,'list'),
    ('00000000-0000-0000-0000-00000000007e','entry-requirements','entry_requirements','Entry Requirements','wiki_program_entry_requirements',4,false,NULL,'clipboard-list'),
    ('00000000-0000-0000-0000-00000000007e','other','other','Other','wiki_program_other',7,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-00000000007e','overview','overview','Overview','wiki_program_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007e','tuition-fees','tuition_fees','Tuition & Fees','wiki_program_tuition_fees',6,false,NULL,'dollar-sign'),
    ('00000000-0000-0000-0000-00000000007e','universities','universities','Universities','wiki_program_universities',2,true,'{"mode":"related","enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-00000000007b','application-deadlines','application_deadlines','Application & Deadlines','wiki_scholarship_application_deadlines',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007b','coverage-benefits','coverage_benefits','Coverage & Benefits','wiki_scholarship_coverage_benefits',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007b','eligibility','eligibility','Eligibility','wiki_scholarship_eligibility',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007b','other','other','Other','wiki_scholarship_other',6,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-00000000007b','overview','overview','Overview','wiki_scholarship_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-00000000007b','selection','selection','Selection & Renewal','wiki_scholarship_selection',5,false,NULL,'check-square'),
    ('00000000-0000-0000-0000-000000000061','attractions','attractions','{name} Attractions','wiki_state_attractions',7,true,'{"mode":"directory","levels":["attraction"],"enabled":true}'::jsonb,'landmark'),
    ('00000000-0000-0000-0000-000000000061','banking','banking','Banking','wiki_state_banking',13,true,'{"mode":"directory","levels":["bank"],"enabled":true}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000061','cities','cities','Cities','wiki_state_cities',1,false,'{"mode":"directory","levels":["city"],"enabled":true,"metrics":[]}'::jsonb,'map-pin'),
    ('00000000-0000-0000-0000-000000000061','community','community','Community','wiki_state_community',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','cost-of-living','cost_of_living','Cost of Living','wiki_state_cost_of_living',2,true,'{"levels":["city"],"enabled":true,"metrics":["min","avg","max"]}'::jsonb,NULL),
    ('00000000-0000-0000-0000-000000000061','healthcare','healthcare','Healthcare','wiki_state_healthcare',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','housing','housing','Housing','wiki_state_housing',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','jobs','jobs','Jobs & Economy','wiki_state_jobs',9,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','other','other','Other','wiki_state_other',11,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','overview','overview','Overview','wiki_state_overview',0,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','safety','safety','Safety & Crime','wiki_state_safety',8,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','schools','schools','{name} Schools & Education','wiki_state_schools',10,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','taxes','taxes','Taxes','wiki_state_taxes',11,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000061','transportation','transportation','Transportation','wiki_state_transportation',14,false,NULL,'train'),
    ('00000000-0000-0000-0000-000000000061','universities','universities','{name} Universities','wiki_state_universities',6,false,'{"mode":"directory","levels":["university","education"],"enabled":true}'::jsonb,'graduation-cap'),
    ('00000000-0000-0000-0000-000000000061','weather','weather','Weather & Climate','wiki_state_weather',12,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','application-process','application_process','Application Process','wiki_student_visa_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','dependents','dependents','Dependents & Family','wiki_student_visa_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000074','documents','documents','Required Documents','wiki_student_visa_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000074','fees-costs','fees_costs','Fees & Costs','wiki_student_visa_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','other','other','Other','wiki_student_visa_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000074','overview','overview','Overview','wiki_student_visa_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','processing-time','processing_time','Processing Time','wiki_student_visa_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','refusals','refusals_appeals','Refusals & Appeals','wiki_student_visa_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000074','renewal','renewal_extension','Extension & Renewal','wiki_student_visa_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000074','requirements','requirements','Requirements & Eligibility','wiki_student_visa_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000074','rights','rights_restrictions','Rights & Restrictions','wiki_student_visa_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-000000000077','application-process','application_process','Application Process','wiki_tourist_visa_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000077','dependents','dependents','Dependents & Family','wiki_tourist_visa_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000077','documents','documents','Required Documents','wiki_tourist_visa_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000077','fees-costs','fees_costs','Fees & Costs','wiki_tourist_visa_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000077','other','other','Other','wiki_tourist_visa_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000077','overview','overview','Overview','wiki_tourist_visa_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000077','processing-time','processing_time','Processing Time','wiki_tourist_visa_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000077','refusals','refusals_appeals','Refusals & Appeals','wiki_tourist_visa_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000077','renewal','renewal_extension','Extension & Renewal','wiki_tourist_visa_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000077','requirements','requirements','Requirements & Eligibility','wiki_tourist_visa_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000077','rights','rights_restrictions','Rights & Restrictions','wiki_tourist_visa_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-000000000073','application-process','application_process','Application Process','wiki_visa_route_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000073','dependents','dependents','Dependents & Family','wiki_visa_route_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000073','documents','documents','Required Documents','wiki_visa_route_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000073','fees-costs','fees_costs','Fees & Costs','wiki_visa_route_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000073','other','other','Other','wiki_visa_route_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000073','overview','overview','Overview','wiki_visa_route_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000073','processing-time','processing_time','Processing Time','wiki_visa_route_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000073','refusals','refusals_appeals','Refusals & Appeals','wiki_visa_route_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000073','renewal','renewal_extension','Extension & Renewal','wiki_visa_route_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000073','requirements','requirements','Requirements & Eligibility','wiki_visa_route_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000073','rights','rights_restrictions','Rights & Restrictions','wiki_visa_route_rights_restrictions',8,false,NULL,'shield-check'),
    ('00000000-0000-0000-0000-000000000075','application-process','application_process','Application Process','wiki_work_visa_application_process',5,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000075','dependents','dependents','Dependents & Family','wiki_work_visa_dependents',7,false,NULL,'users'),
    ('00000000-0000-0000-0000-000000000075','documents','documents','Required Documents','wiki_work_visa_documents',6,false,NULL,'file-text'),
    ('00000000-0000-0000-0000-000000000075','fees-costs','fees_costs','Fees & Costs','wiki_work_visa_fees_costs',3,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000075','other','other','Other','wiki_work_visa_other',11,false,NULL,'more-horizontal'),
    ('00000000-0000-0000-0000-000000000075','overview','overview','Overview','wiki_work_visa_overview',1,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000075','processing-time','processing_time','Processing Time','wiki_work_visa_processing_time',4,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000075','refusals','refusals_appeals','Refusals & Appeals','wiki_work_visa_refusals_appeals',10,false,NULL,'alert-triangle'),
    ('00000000-0000-0000-0000-000000000075','renewal','renewal_extension','Extension & Renewal','wiki_work_visa_renewal_extension',9,false,NULL,'refresh-cw'),
    ('00000000-0000-0000-0000-000000000075','requirements','requirements','Requirements & Eligibility','wiki_work_visa_requirements',2,true,NULL,NULL),
    ('00000000-0000-0000-0000-000000000075','rights','rights_restrictions','Rights & Restrictions','wiki_work_visa_rights_restrictions',8,false,NULL,'shield-check')
) AS m(tid, slug, category, label, gslug, position, is_system, rollup, icon)
JOIN custom_field_groups g ON g.slug = m.gslug
ON CONFLICT (space_type_id, slug) DO UPDATE SET
    label = EXCLUDED.label,
    field_group_id = EXCLUDED.field_group_id,
    position = EXCLUDED.position,
    rollup = EXCLUDED.rollup,
    icon = EXCLUDED.icon;

-- ---------------------------------------------------------------------------
-- 4. Page templates (subpages) — upsert on (category_id, slug).
-- ---------------------------------------------------------------------------
INSERT INTO wiki_type_page_templates
    (category_id, slug, label, field_group_id, position, is_system, section_label, preview_limit)
SELECT c.id, m.slug, m.label, g.id, m.position, m.is_system, m.section_label, m.preview_limit
FROM (VALUES
    ('00000000-0000-0000-0000-000000000062','cost-of-living','food-groceries-prices','Food & Groceries Prices in {name}','wiki_city_prices_food_groceries',1,true,'Food & Groceries',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','restaurant-prices','Restaurant Prices in {name}','wiki_city_prices_restaurants',2,true,'Restaurants',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','transportation-prices','Transportation Prices in {name}','wiki_city_prices_transportation',3,true,'Transportation',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','utilities-prices','Utilities Prices in {name}','wiki_city_prices_utilities',4,true,'Utilities',4),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','lifestyle-prices','Lifestyle Prices in {name}','wiki_city_prices_lifestyle',5,true,'Lifestyle',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','rent-housing-prices','Rent & Housing Prices in {name}','wiki_city_prices_rent_housing',6,true,'Rent & Housing',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','healthcare-prices','Healthcare Prices in {name}','wiki_city_prices_healthcare',7,true,'Healthcare',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','childcare-school-fees','Childcare & School Fees in {name}','wiki_city_prices_childcare',8,true,'Childcare & School',5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','clothing-shoes','Clothing & Shoes Prices in {name}','wiki_city_page_clothing_shoes',9,false,NULL,5),
    ('00000000-0000-0000-0000-000000000062','cost-of-living','salaries-financing','Salaries & Financing in {name}','wiki_city_page_salaries_financing',10,false,NULL,5),
    ('00000000-0000-0000-0000-000000000066','tuition-fees','program-fees','Program Fees for {name}','wiki_university_page_program_fees',99,false,NULL,5),
    ('00000000-0000-0000-0000-000000000071','plan-your-visit','tickets','Tickets & Pricing for {name}','wiki_attraction_page_tickets',99,false,NULL,5)
) AS m(tid, cslug, slug, label, gslug, position, is_system, section_label, preview_limit)
JOIN wiki_type_categories c ON c.space_type_id = m.tid::uuid AND c.slug = m.cslug
JOIN custom_field_groups g ON g.slug = m.gslug
ON CONFLICT (category_id, slug) DO UPDATE SET
    label = EXCLUDED.label,
    field_group_id = EXCLUDED.field_group_id,
    position = EXCLUDED.position,
    section_label = EXCLUDED.section_label,
    preview_limit = EXCLUDED.preview_limit;

-- ---------------------------------------------------------------------------
-- 5. Field definitions (upsert on (group_id, key)).
-- ---------------------------------------------------------------------------

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('wheelchair_accessible','Wheelchair Accessible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('guided_tours','Guided Tours Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('audio_guide','Audio Guide Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('restrooms_cafe','Restrooms & Café On-Site','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('parking','Parking Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('photo_policy','Photography Policy','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('facilities_note','Facilities Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_facilities'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('established','Established / Built','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('heritage_status','Heritage Status','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('historical_note','Historical Significance','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_history'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('nearby_attractions','Nearby Attractions','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('nearest_transit','Nearest Transit Stop','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('day_trip_note','Day-Trip Combinations','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_nearby'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('kind','Kind','select','[{"label":"Museum","value":"museum"},{"label":"Landmark","value":"landmark"},{"label":"Monument","value":"monument"},{"label":"Historic Site","value":"historic-site"},{"label":"Market","value":"market"},{"label":"Waterfall","value":"waterfall"},{"label":"Mountain","value":"mountain"},{"label":"Rock Formation","value":"rock-formation"},{"label":"Lake","value":"lake"},{"label":"Cave","value":"cave"},{"label":"Beach","value":"beach"},{"label":"Forest Reserve","value":"forest-reserve"},{"label":"Canyon","value":"canyon"},{"label":"Hot Spring","value":"hot-spring"},{"label":"National Park","value":"national-park"},{"label":"Garden","value":"garden"},{"label":"Zoo","value":"zoo"},{"label":"Theme Park","value":"theme-park"},{"label":"Other","value":"other"}]'::jsonb,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('address','Address','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,2,NULL),
    ('coordinates','Coordinates','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,3,NULL),
    ('opening_hours','Opening Hours','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('entry_fee','Entry Fee','price_range',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('website','Website','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,6,NULL),
    ('wikidata_id','Wikidata ID','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,7,NULL),
    ('gallery','Gallery','image_multi','{"max_files":12,"max_size_mb":8}'::jsonb,NULL,NULL,NULL,false,true,true,false,false,true,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('ticket_adult','Adult Ticket','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('ticket_child','Child Ticket','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('ticket_senior','Senior Ticket','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('ticket_student','Student Ticket','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('ticket_family','Family Ticket','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('guided_tour','Guided Tour','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('audio_guide','Audio Guide','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('annual_pass','Annual / Season Pass','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_page_tickets'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('getting_there','Getting There','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,1,NULL),
    ('best_time','Best Time','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,2,NULL),
    ('tips','Tips','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_attraction_plan_your_visit'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('account_types','Account Types','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('monthly_fee','Monthly Maintenance Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('min_opening_deposit','Minimum Opening Deposit','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('atm_fee_own','Own-Network ATM Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('atm_fee_other','Other-Bank ATM Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('debit_card_fee','Debit Card Annual Fee','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('accounts_note','Accounts Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_accounts_fees'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('branch_count','Number of Branches','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('atm_count','Number of ATMs','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('coverage_note','Coverage Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_coverage'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('app_rating','Mobile App Rating','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('online_features','Online Banking Features','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('ussd_available','USSD Banking Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('instant_transfers','Instant Transfers','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('virtual_cards','Virtual Cards','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('digital_note','Digital Banking Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_digital_banking'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('bank_type','Bank Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('account_requirements','Account Requirements','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('maintenance_fees','Maintenance Fees','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('international_features','International Features','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('website','Website','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('savings_apy','Savings APY','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('deposit_insurance','Deposit Insurance Scheme','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('insured_amount','Insured Amount per Depositor','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('loan_rates','Typical Loan / Mortgage Rates','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('rates_note','Rates Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_rates'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('intl_transfer_fee','International Transfer Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('swift_available','SWIFT Transfers Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('fx_margin','FX Margin vs Mid-Market','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('supported_currencies','Supported Currencies / Corridors','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('transfer_speed','Typical Transfer Speed','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('remittance_note','Remittance Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_remittance_fx'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('non_resident_can_open','Non-Residents Can Open','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('documents_required','Required Documents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('min_deposit','Minimum Deposit','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('in_person_required','In-Person Visit Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('online_opening','Online Opening Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('opening_time','Account Opening Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_bank_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL),
    ('investment_threshold','Minimum Investment','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,13,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_business_visa_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_banks','Major Local Banks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('intl_banks','International Banks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('neobanks_fintech','Neobanks & Fintech','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('atm_coverage','ATM Coverage','select','[{"label":"Excellent","value":"Excellent"},{"label":"Good","value":"Good"},{"label":"Limited","value":"Limited"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('cash_vs_card','Cash vs Card Culture','select','[{"label":"Card Accepted Everywhere","value":"Card Accepted Everywhere"},{"label":"Mixed","value":"Mixed"},{"label":"Cash-Heavy","value":"Cash-Heavy"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('banking_note','Banking Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_banking'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('expat_neighborhoods','Expat Neighborhoods','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('community_groups','Community Groups & Meetups','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('coworking_spaces','Coworking Spaces','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('community_note','Community Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_community'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('monthly_total','Single person (monthly)','price_range','{"computed":{"metric":"sum","sources":[{"key":"milk_1l","group":"wiki_city_prices_food_groceries"},{"key":"bread_500g","group":"wiki_city_prices_food_groceries"},{"key":"rice_1kg","group":"wiki_city_prices_food_groceries"},{"key":"eggs_12","group":"wiki_city_prices_food_groceries"},{"key":"chicken_1kg","group":"wiki_city_prices_food_groceries"},{"key":"beef_1kg","group":"wiki_city_prices_food_groceries"},{"key":"apples_1kg","group":"wiki_city_prices_food_groceries"},{"key":"bananas_1kg","group":"wiki_city_prices_food_groceries"},{"key":"tomatoes_1kg","group":"wiki_city_prices_food_groceries"},{"key":"potatoes_1kg","group":"wiki_city_prices_food_groceries"},{"key":"ttc_monthly_adult","group":"wiki_city_prices_transportation"},{"key":"utilities_85sqm","group":"wiki_city_prices_utilities"},{"key":"internet_60","group":"wiki_city_prices_utilities"},{"key":"mobile_10gb","group":"wiki_city_prices_utilities"},{"key":"gym_monthly","group":"wiki_city_prices_lifestyle"},{"key":"movie_ticket","group":"wiki_city_prices_lifestyle"},{"key":"netflix_standard","group":"wiki_city_prices_lifestyle"},{"key":"rent_1br","group":"wiki_city_housing"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,'summary'),
    ('family_total','Family of 4 (monthly)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,'summary'),
    ('rent_1br','Rent — 1BR city centre (monthly)','price_range','{"computed":{"metric":"avg","sources":[{"key":"rent_1br","group":"wiki_city_housing"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,'accommodation'),
    ('exp_accommodation','Accommodation','currency','{"computed":{"metric":"avg","sources":[{"key":"rent_1br","group":"wiki_city_housing"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,10,'monthly_expenses'),
    ('exp_food','Food','currency','{"computed":{"metric":"sum","sources":[{"key":"milk_1l","group":"wiki_city_prices_food_groceries"},{"key":"bread_500g","group":"wiki_city_prices_food_groceries"},{"key":"rice_1kg","group":"wiki_city_prices_food_groceries"},{"key":"eggs_12","group":"wiki_city_prices_food_groceries"},{"key":"chicken_1kg","group":"wiki_city_prices_food_groceries"},{"key":"beef_1kg","group":"wiki_city_prices_food_groceries"},{"key":"apples_1kg","group":"wiki_city_prices_food_groceries"},{"key":"bananas_1kg","group":"wiki_city_prices_food_groceries"},{"key":"tomatoes_1kg","group":"wiki_city_prices_food_groceries"},{"key":"potatoes_1kg","group":"wiki_city_prices_food_groceries"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,11,'monthly_expenses'),
    ('exp_transportation','Transportation','currency','{"computed":{"metric":"avg","sources":[{"key":"ttc_monthly_adult","group":"wiki_city_prices_transportation"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,12,'monthly_expenses'),
    ('exp_utilities','Utilities','currency','{"computed":{"metric":"avg","sources":[{"key":"utilities_85sqm","group":"wiki_city_prices_utilities"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,13,'monthly_expenses'),
    ('exp_internet','Internet','currency','{"computed":{"metric":"avg","sources":[{"key":"internet_60","group":"wiki_city_prices_utilities"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,14,'monthly_expenses'),
    ('exp_mobile','Mobile plan','currency','{"computed":{"metric":"avg","sources":[{"key":"mobile_10gb","group":"wiki_city_prices_utilities"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,15,'monthly_expenses'),
    ('exp_entertainment','Entertainment','currency','{"computed":{"metric":"sum","sources":[{"key":"gym_monthly","group":"wiki_city_prices_lifestyle"},{"key":"movie_ticket","group":"wiki_city_prices_lifestyle"},{"key":"netflix_standard","group":"wiki_city_prices_lifestyle"}]}}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,16,'monthly_expenses'),
    ('exp_health','Health insurance','currency','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,17,'monthly_expenses'),
    ('qf_currency','Currency','text','{"default_source":"space_currency"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,20,'quick_facts'),
    ('qf_monthly_cost','Average monthly cost','currency','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,21,'quick_facts'),
    ('qf_population','Population (2024)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,22,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,23,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_cost_of_living'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('major_hospitals','Major Hospitals','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('emergency_number','Emergency Number','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_gp_visit','Avg GP Visit Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('clinics_note','Clinics & Specialists','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('insurance_note','Insurance & Coverage','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_healthcare'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rent_1br','Rent — 1BR apartment (monthly)','price_range','{}'::jsonb,NULL,NULL,NULL,false,true,true,false,false,false,1,NULL),
    ('rent_3br','Rent — 3BR apartment (monthly)','price_range','{}'::jsonb,NULL,NULL,NULL,false,true,true,false,false,false,2,NULL),
    ('buy_sqm_centre','Buy — price per m², city centre','price_range','{}'::jsonb,NULL,NULL,NULL,false,true,true,false,false,false,3,NULL),
    ('buy_sqm_outside','Buy — price per m², outside centre','price_range','{}'::jsonb,NULL,NULL,NULL,false,true,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_housing'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('avg_salary','Average Salary (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('minimum_wage','Minimum Wage (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('unemployment_rate','Unemployment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('top_industries','Top Industries','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('major_employers','Major Employers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('job_portals','Popular Job Portals','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('job_market_note','Job Market Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_jobs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('nightlife_areas','Nightlife Areas','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('sports_fitness','Gyms & Sports','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('parks_outdoors','Parks & Outdoor Activities','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('cultural_venues','Cultural Venues','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('annual_events','Annual Events & Festivals','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('lifestyle_note','Lifestyle Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_lifestyle'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('population','Population','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('province_state','Province / State','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('timezone','Timezone','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('official_language','Official Language','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('popular_places','Popular Places','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('main_airport','Main Airport','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('jeans','Jeans (mid-range)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('dress','Summer Dress (chain store)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('sneakers','Sneakers (mid-range)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('business_shoes','Business Shoes','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('school_uniform','School Uniform Set','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_page_clothing_shoes'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('studio','Studio (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,'rent_prices'),
    ('one_bedroom','1 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,'rent_prices'),
    ('two_bedroom','2 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,'rent_prices'),
    ('three_bedroom','3 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,'rent_prices'),
    ('qf_best_value_area','Best Value Area','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_page_rent_prices'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('avg_monthly_salary','Average Monthly Salary (after tax)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('median_monthly_salary','Median Monthly Salary','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('minimum_wage_monthly','Minimum Wage (monthly)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('mortgage_rate','Mortgage Interest Rate (20yr, annual %)','price_parts','{"parts":[{"key":"price","kind":"number","label":"Rate %"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('salary_note','Salary & Financing Notes','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_page_salaries_financing'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('qf_currency','Currency','text','{"default_source":"space_currency"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,20,'quick_facts'),
    ('qf_monthly_cost','Average monthly cost','currency','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,21,'quick_facts'),
    ('qf_population','Population (2024)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,22,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,23,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('daycare_monthly','Daycare / Nursery (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('preschool_monthly','Preschool / Kindergarten (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('nanny_monthly','Full-time Nanny (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('babysitter_hourly','Babysitter (hourly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('intl_primary_annual','Intl Primary School (annual)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('intl_secondary_annual','Intl Secondary School (annual)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('public_school_fees','Public School Fees','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_childcare'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('milk_1l','Milk (1L)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('bread_500g','Bread (500g)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('rice_1kg','Rice (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('eggs_12','Eggs (12)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('chicken_1kg','Chicken fillets (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('beef_1kg','Beef (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('apples_1kg','Apples (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('bananas_1kg','Bananas (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('tomatoes_1kg','Tomatoes (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('potatoes_1kg','Potatoes (1kg)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_food_groceries'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('gp_visit','GP / Doctor Visit','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('specialist_visit','Specialist Visit','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dental_checkup','Dental Checkup & Cleaning','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('eye_exam','Eye Exam','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('therapy_session','Therapy / Counselling Session','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_monthly','Private Health Insurance (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('prescription_common','Common Prescription','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('er_visit','Emergency Room Visit (uninsured)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_healthcare'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('gym_monthly','Gym membership (1 month)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('movie_ticket','Movie ticket','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('netflix_standard','Netflix (Standard plan)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_lifestyle'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rent_room_shared','Room in Shared Flat (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,'rent'),
    ('rent_studio','Studio Apartment (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,'rent'),
    ('rent_1br_centre','1BR City Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,'rent'),
    ('rent_1br_outside','1BR Outside Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,'rent'),
    ('rent_2br_centre','2BR City Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,'rent'),
    ('rent_2br_outside','2BR Outside Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,'rent'),
    ('rent_3br_centre','3BR City Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,'rent'),
    ('rent_3br_outside','3BR Outside Centre (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,'rent'),
    ('buy_sqm_centre','Buy — Price per m² Centre','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,'purchase'),
    ('buy_sqm_outside','Buy — Price per m² Outside','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,'purchase'),
    ('deposit_months','Typical Deposit (months)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,'purchase'),
    ('agent_fee_pct','Agent Fee (% of annual rent)','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,'purchase')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_rent_housing'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('meal_inexpensive','Inexpensive meal','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('meal_two_midrange','Mid-range meal for two','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('mcmeal','McMeal (or similar combo)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('cappuccino','Cappuccino (regular)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('beer_05l','Beer (0.5L, restaurant)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('wine_midrange','Bottle of wine (mid-range)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_restaurants'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('ttc_oneway_adult','One-way Ticket (Adult)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,'ttc'),
    ('ttc_day_pass','One-day Pass','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,'ttc'),
    ('ttc_weekly_pass','Weekly Pass','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,'ttc'),
    ('ttc_monthly_adult','Monthly Pass (Adult)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,'ttc'),
    ('ttc_monthly_student','Monthly Pass (Student)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,'ttc'),
    ('ttc_monthly_senior','Monthly Pass (Senior)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,'ttc'),
    ('ttc_presto_card','PRESTO Card','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,7,'ttc'),
    ('taxi_traditional','Taxi (Traditional)','price_parts','{"parts":[{"key":"base_fare","kind":"money","label":"Base Fare"},{"key":"per_km","kind":"money","label":"Price per km"},{"key":"per_min","kind":"money","label":"Price per min"},{"key":"minimum","kind":"money","label":"Minimum Fare"}],"primary_part":"per_km"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,10,'taxis'),
    ('taxi_uberx','UberX','price_parts','{"parts":[{"key":"base_fare","kind":"money","label":"Base Fare"},{"key":"per_km","kind":"money","label":"Price per km"},{"key":"per_min","kind":"money","label":"Price per min"},{"key":"minimum","kind":"money","label":"Minimum Fare"}],"primary_part":"per_km"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,11,'taxis'),
    ('taxi_uberxl','UberXL','price_parts','{"parts":[{"key":"base_fare","kind":"money","label":"Base Fare"},{"key":"per_km","kind":"money","label":"Price per km"},{"key":"per_min","kind":"money","label":"Price per min"},{"key":"minimum","kind":"money","label":"Minimum Fare"}],"primary_part":"per_km"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,12,'taxis'),
    ('taxi_lyft','Lyft','price_parts','{"parts":[{"key":"base_fare","kind":"money","label":"Base Fare"},{"key":"per_km","kind":"money","label":"Price per km"},{"key":"per_min","kind":"money","label":"Price per min"},{"key":"minimum","kind":"money","label":"Minimum Fare"}],"primary_part":"per_km"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,13,'taxis'),
    ('car_loan','Car Loan Payment','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,20,'car_ownership'),
    ('car_insurance','Insurance','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,21,'car_ownership'),
    ('car_fuel','Fuel','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,22,'car_ownership'),
    ('car_parking','Parking','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,23,'car_ownership'),
    ('car_maintenance','Maintenance & Repairs','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,24,'car_ownership'),
    ('car_other','Other Costs (wash, etc.)','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,25,'car_ownership'),
    ('car_total','Total Estimated Monthly Cost','price_parts','{"parts":[{"key":"monthly_min","kind":"money","label":"Monthly (min)"},{"key":"monthly_max","kind":"money","label":"Monthly (max)"},{"key":"details","kind":"text","label":"Details"}],"emphasize":true,"primary_part":"monthly_min"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,26,'car_ownership'),
    ('bike_single','Bike Share Toronto (Single Ride)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,30,'biking'),
    ('bike_day_pass','Bike Share Toronto (24-Hour Pass)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,31,'biking'),
    ('escooter','Electric Scooter (Bird/Lime)','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,32,'biking'),
    ('ebike_monthly','Monthly E-bike Rental','price_parts','{"parts":[{"key":"price","kind":"money","label":"Price"},{"key":"details","kind":"text","label":"Details"}],"primary_part":"price"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,33,'biking'),
    ('qf_currency','Currency','text','{"default_source":"space_currency"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,40,'quick_facts'),
    ('qf_transit_monthly','Typical monthly transit cost','currency','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,41,'quick_facts'),
    ('qf_popular_transit','Most popular transit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,42,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,43,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_transportation'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('utilities_85sqm','Basic utilities (85m² apartment)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('internet_60','Internet (60 Mbps+)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('mobile_10gb','Mobile plan (10GB+)','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_prices_utilities'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('safety_index','Safety Index (0-100)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('crime_level','Crime Level','select','[{"label":"Very Low","value":"Very Low"},{"label":"Low","value":"Low"},{"label":"Moderate","value":"Moderate"},{"label":"High","value":"High"},{"label":"Very High","value":"Very High"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('safe_neighborhoods','Safest Neighborhoods','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('areas_to_avoid','Areas to Avoid','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('common_scams','Common Scams','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('safety_tips','Safety Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_safety'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('top_schools','Top-Rated Schools','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('intl_schools','International Schools','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('school_quality','Overall School Quality','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('daycare_cost','Daycare Cost (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('enrollment_note','Enrollment Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_schools'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('transit_authority','Main Transit Operator','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('metro_available','Metro / Subway Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('transit_card','Transit Card / Pass System','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('monthly_pass','Monthly Transit Pass','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('airport_name','Main Airport','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('airport_distance','Airport Distance to Centre','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('ride_hailing','Ride-Hailing Services','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('bike_share','Bike / Scooter Share','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('walkability_score','Walk Score (0-100)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('commute_note','Commute Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_transportation'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('utility_providers','Utility Providers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('internet_providers','Internet Providers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('internet_speed','Avg Broadband Speed','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('internet_cost','Internet Cost (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('mobile_operators','Mobile Operators','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('power_reliability','Power Reliability','select','[{"label":"Reliable","value":"Reliable"},{"label":"Occasional Outages","value":"Occasional Outages"},{"label":"Frequent Outages","value":"Frequent Outages"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('utilities_note','Utilities Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_utilities'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('climate_type','Climate Type','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_temp_summer','Avg Summer Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_temp_winter','Avg Winter Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('annual_rainfall','Annual Rainfall','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('rainy_season','Rainy Season','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('best_months','Best Months to Visit','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('weather_note','Weather Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_city_weather'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('major_banks','Major Banks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('account_requirements','Account Requirements','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('online_banking_quality','Online Banking Quality','select','["Excellent","Good","Basic","Poor"]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('intl_transfer_options','Intl Transfer Options','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('banking_note','Banking Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_banking'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('ease_of_business','Ease of Doing Business','select','[{"label":"Very Easy","value":"Very Easy"},{"label":"Moderate","value":"Moderate"},{"label":"Difficult","value":"Difficult"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('corporate_tax','Corporate Tax Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('business_registration','How to Register a Business','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('startup_visa','Startup / Entrepreneur Visa Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('business_note','Business Environment Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_business'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('expat_population','Expat Population (est.)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('popular_expat_cities','Popular Expat Cities','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('community_groups','Community Groups & Forums','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('languages_spoken','Languages Spoken','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_community'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('exp_accommodation','Accommodation','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,'monthly_expenses'),
    ('exp_food','Food','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,'monthly_expenses'),
    ('exp_transportation','Transportation','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,'monthly_expenses'),
    ('exp_utilities','Utilities','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,'monthly_expenses'),
    ('exp_internet','Internet','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,'monthly_expenses'),
    ('exp_mobile','Mobile plan','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,'monthly_expenses'),
    ('exp_entertainment','Entertainment','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,7,'monthly_expenses'),
    ('exp_health','Health insurance','price_range','{}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,8,'monthly_expenses'),
    ('qf_currency','Currency','text','{"default_source":"space_currency"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,20,'quick_facts'),
    ('qf_monthly_range','Average monthly cost','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,21,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,22,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_cost_of_living'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('tipping_custom','Tipping Custom','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('religions','Major Religions','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('cuisine_staples','Popular Dishes','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('festivals_events','Major Festivals & Events','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('expat_friendliness','Expat Friendliness','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('cultural_etiquette','Etiquette & Norms','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_culture'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('attestation_process','Document Attestation Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('apostille_available','Apostille Convention Member','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('document_sources','Where to Get Key Documents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('legalization_note','Legalization Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('education_system','Education System Structure','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('literacy_rate','Literacy Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('school_year','School Year (start-end)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('public_school_free','Public School Free','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('language_of_instruction','Language of Instruction','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('intl_schools','International School Options','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('homeschooling_legal','Homeschooling Legal','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('education_note','Education Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_education'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('healthcare_system','Healthcare System','select','["Public","Private","Mixed (public + private)"]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('emergency_number','Emergency Number','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('avg_gp_visit','Avg GP Visit Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('hospitals_note','Hospitals & Clinics','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('expat_health_tips','Expat Health Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_healthcare'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rent_1br','Rent — 1BR (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('rent_3br','Rent — 3BR (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('deposit_norm','Typical Deposit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('utilities_included','Utilities Usually Included','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('popular_expat_areas','Popular Expat Areas','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('tenant_rights_note','Tenant Rights','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_housing'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('unemployment_rate','Unemployment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('minimum_wage','Minimum Wage (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_salary','Average Salary (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('in_demand_sectors','In-Demand Sectors','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('major_employers','Major Employers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('job_portals','Popular Job Portals','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('economy_note','Economy & Work Culture','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_jobs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('driving_side','Driving Side','select','["Right","Left"]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('power_plugs','Power Plug Types','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('public_holidays_note','Public Holidays','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('currency','Currency','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,0,NULL),
    ('currency_symbol','Currency Symbol','currency',NULL,NULL,NULL,NULL,false,false,true,false,false,false,0,NULL),
    ('popular_places','Popular Places','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,0,'basic_information'),
    ('population','Population','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,0,NULL),
    ('capital','Capital City','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('timezones','Time Zones','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('calling_code','Calling Code','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('official_language','Official Language','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('safety_index','Safety Index (0-100)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('crime_level','Crime Level','select','[{"label":"Very Low","value":"Very Low"},{"label":"Low","value":"Low"},{"label":"Moderate","value":"Moderate"},{"label":"High","value":"High"},{"label":"Very High","value":"Very High"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('common_crimes','Common Crimes & Scams','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('regions_to_avoid','Regions to Avoid','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('emergency_number','General Emergency Number','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('natural_disaster_risks','Natural Disaster Risks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('travel_advisory','Travel Advisory Summary','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('safety_tips','Safety Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_safety'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('first_week_checklist','First-Week Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('sim_card_note','Getting a SIM Card','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('bank_setup_note','Opening a Bank Account','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('transit_card_note','Transit Card Setup','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('id_registration','ID / Tax Number Registration','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_registration','Health Registration (GP/insurance)','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('settling_note','Settling-In Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_settling_in'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('income_tax_range','Income Tax Brackets','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('top_marginal_rate','Top Marginal Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('sales_tax_vat','VAT / Sales Tax','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('social_security_rate','Social Security Contributions','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('tax_residency_rule','Tax Residency Rule','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('tax_treaties','Tax Treaties','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('expat_tax_note','Expat Tax Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_taxes'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rail_network','Rail Network','select','[{"label":"Extensive","value":"Extensive"},{"label":"Moderate","value":"Moderate"},{"label":"Limited","value":"Limited"},{"label":"None","value":"None"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('major_airports','Major International Airports','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('domestic_flights','Domestic Flights Common','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('intercity_buses','Major Bus Operators','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('road_quality','Road Quality','select','[{"label":"Excellent","value":"Excellent"},{"label":"Good","value":"Good"},{"label":"Variable","value":"Variable"},{"label":"Poor","value":"Poor"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('driving_note','Driving & License Requirements','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('transport_note','Transport Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_transportation'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_visa_types','Common Visa Types','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('processing_time','Typical Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('visa_fee_range','Visa Fee Range','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('work_permit_available','Work Permits Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('requirements_summary','Entry Requirements','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('residency_pathways','Residency Pathways','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_visa'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('climate_zones','Climate Zones','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_temp_summer','Avg Summer Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_temp_winter','Avg Winter Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('rainy_season','Rainy Season','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('best_months','Best Months to Visit','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('weather_note','Weather Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_country_weather'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('employment_rate','Graduate Employment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_starting_salary','Avg Starting Salary','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('common_roles','Common Job Titles','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('top_employers','Top Employers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('licensing_note','Professional Licensure','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_career_outcomes'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('credits_required','Credits Required','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('core_modules','Core Modules','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('elective_options','Elective Options','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('thesis_required','Thesis / Capstone Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('internship_included','Internship / Co-op Included','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('curriculum_note','Curriculum Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_curriculum'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('study_modes','Study Modes','multiselect','[{"label":"Full-time","value":"Full-time"},{"label":"Part-time","value":"Part-time"},{"label":"Online","value":"Online"},{"label":"Hybrid","value":"Hybrid"},{"label":"Evening","value":"Evening"},{"label":"Weekend","value":"Weekend"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('duration_fulltime','Duration (Full-time)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('duration_parttime','Duration (Part-time)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('intake_months','Intake Months','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('schedule_note','Schedule Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_delivery'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('gpa_requirement','GPA Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('language_tests','Language Test Requirements','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('prerequisites','Prerequisite Courses','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('entrance_exam','Entrance Exam (GRE/GMAT/other)','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('portfolio_required','Portfolio Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_entry_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('degree_type','Degree Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('duration','Duration','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('tuition_note','Tuition Note','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('entry_requirements','Entry Requirements','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('career_outcomes','Career Outcomes','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('tuition_local','Tuition — Domestic Students','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('tuition_intl','Tuition — International Students','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('per_credit_cost','Per-Credit Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('other_fees','Other Mandatory Fees','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('deposit_required','Deposit Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('payment_plan','Payment Plan Available','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_course_tuition_fees'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('appointment_required','Appointment Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('booking_method','Booking Method','select','[{"label":"Online","value":"Online"},{"label":"Phone","value":"Phone"},{"label":"Email","value":"Email"},{"label":"Walk-in","value":"Walk-in"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('booking_url','Booking Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('typical_wait','Typical Appointment Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('walk_ins_allowed','Walk-ins Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('appointment_note','Appointment Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_appointment_booking'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('services_offered','Services Offered','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('visa_processing','Processes Visa Applications','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('emergency_passport','Emergency Travel Documents','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('notarial_services','Notarial & Document Services','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('citizen_registration','Citizen Registration Services','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('services_note','Services Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('service_processing_times','Per-Service Processing Times','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_consular_services'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('address','Full Address','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('phone','Phone','phone',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('emergency_phone','Emergency Phone','phone',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('email','Email','email',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('website','Website','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('opening_hours','Opening Hours','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('nearest_transit','Nearest Transit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('location_note','Location Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_contact_location'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('observed_holidays','Observed Holidays','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('closure_note','Closure Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('emergency_duty','Emergency / Duty Officer','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_holidays'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('consular_district','Consular District / Regions Covered','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('honorary_consulates','Honorary Consulates','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('jurisdiction_note','Jurisdiction Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_jurisdiction'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('services','Services','textarea',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('appointment_link','Appointment Link','url',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('address','Address','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,3,NULL),
    ('phone','Phone','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_embassy_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_family_visa_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('supermarkets','Supermarkets & Markets','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('dining_scene','Dining & Café Scene','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('parks','Parks & Green Spaces','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('healthcare_nearby','Clinics & Pharmacies','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('amenities_note','Amenities Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_amenities'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('typical_residents','Typical Residents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_age','Average Age','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('household_types','Household Types','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('expat_presence','Expat / Diaspora Presence','select','[{"label":"High","value":"High"},{"label":"Moderate","value":"Moderate"},{"label":"Low","value":"Low"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('demographics_note','Demographics Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_demographics'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('character','Character','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('safety_note','Safety Note','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('typical_rent','Typical Rent','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('walkability','Walkability','text',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rent_room','Room in Shared Flat (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,'rent'),
    ('rent_studio','Studio (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,'rent'),
    ('rent_1br','1 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,'rent'),
    ('rent_2br','2 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,'rent'),
    ('rent_3br','3 Bedroom (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,'rent'),
    ('rent_trend','Rent Trend','select','[{"label":"Rising","value":"Rising"},{"label":"Stable","value":"Stable"},{"label":"Falling","value":"Falling"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,6,'market'),
    ('deposit_norm','Typical Deposit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,'market'),
    ('rent_note','Rent Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,'market')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_rent_prices'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('safety_rating','Safety Rating','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('common_concerns','Common Concerns','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('police_presence','Police / Security Presence','select','[{"label":"Strong","value":"Strong"},{"label":"Moderate","value":"Moderate"},{"label":"Low","value":"Low"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('safety_tips','Safety Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_safety'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('schools_nearby','Schools Nearby','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('family_friendly','Family Friendliness','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('childcare_note','Childcare & Playgrounds','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_schools_family'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('nearest_transit','Nearest Transit Stop','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('transit_lines','Transit Lines Serving Area','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('commute_to_centre','Commute to City Centre','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('walkability','Walkability','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('parking_situation','Parking Situation','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_neighborhood_transport'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL),
    ('residency_years','Residency Years Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,13,NULL),
    ('points_breakdown','Points Breakdown','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,14,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_pr_pathway_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('employment_rate','Graduate Employment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_starting_salary','Avg Starting Salary','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('common_roles','Common Job Titles','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('professional_licensure','Professional Licensure','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('outlook_note','Career Outlook Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_career_outcomes'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('core_subjects','Core Subjects','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('typical_credits','Typical Credits','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('specializations','Specializations','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('thesis_typical','Thesis Typically Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('internship_typical','Internship Typically Included','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('curriculum_note','Curriculum Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_curriculum'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('gpa_requirement','GPA Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('language_tests','Language Test Requirements','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('prerequisites','Prerequisites','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('entrance_exams','Entrance Exams','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_entry_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('program_level','Program Level','select','[{"label":"Associate","value":"Associate"},{"label":"Bachelor''s","value":"Bachelor''s"},{"label":"Master''s","value":"Master''s"},{"label":"PhD","value":"PhD"},{"label":"Diploma","value":"Diploma"},{"label":"Certificate","value":"Certificate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('typical_duration','Typical Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('field_category','Field Category','select','[{"label":"STEM","value":"STEM"},{"label":"Business","value":"Business"},{"label":"Health","value":"Health"},{"label":"Humanities","value":"Humanities"},{"label":"Social Sciences","value":"Social Sciences"},{"label":"Arts","value":"Arts"},{"label":"Law","value":"Law"},{"label":"Other","value":"Other"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('demand_level','Job Market Demand','select','[{"label":"High","value":"High"},{"label":"Moderate","value":"Moderate"},{"label":"Low","value":"Low"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('overview_note','Overview Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('tuition_local','Tuition — Domestic Students','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('tuition_intl','Tuition — International Students','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('funding_note','Funding & Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_program_tuition_fees'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_opens','Application Opens','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('deadline','Application Deadline','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('frequency','Application Frequency','select','[{"label":"Annual","value":"Annual"},{"label":"Rolling","value":"Rolling"},{"label":"Multiple Rounds","value":"Multiple Rounds"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('application_url','Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_required','Required Documents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('essay_required','Essay Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('results_timing','Results Announced','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_application_deadlines'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('tuition_covered','Tuition Coverage','select','[{"label":"Full","value":"Full"},{"label":"Partial","value":"Partial"},{"label":"Fixed Amount","value":"Fixed Amount"},{"label":"None","value":"None"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('coverage_amount','Award Amount','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('living_stipend','Living Stipend','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('accommodation_covered','Accommodation Covered','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('travel_allowance','Travel / Flights Allowance','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance','Health Insurance Covered','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('books_materials','Books & Materials Allowance','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('funding_duration','Funding Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('benefits_note','Benefits Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('service_obligation','Service Obligation / Bond (e.g. return-home rule)','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_coverage_benefits'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('eligible_nationalities','Eligible Nationalities','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('min_education','Minimum Education Level','select','[{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('gpa_requirement','GPA / Grade Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('merit_based','Merit-Based','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('need_based','Financial-Need Based','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('eligible_programs','Eligible Programs / Fields','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('eligibility_note','Eligibility Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_eligibility'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('award_amount','Award Amount','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('coverage','Coverage','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,3,NULL),
    ('deadline','Deadline','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,4,NULL),
    ('application_link','Application Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('selection_criteria','Selection Criteria','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('maintain_gpa','Min GPA to Renew','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_conditions','Renewal Conditions','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('selection_note','Selection Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_scholarship_selection'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('major_banks','Major Banks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('credit_unions','Credit Unions','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('branch_coverage','Branch & ATM Coverage','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('banking_note','Banking Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_banking'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('community_groups','Community Groups & Forums','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('events_meetups','Events & Meetups','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('community_note','Community Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_community'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('exp_accommodation','Accommodation','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,'monthly_expenses'),
    ('exp_food','Food','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,'monthly_expenses'),
    ('exp_transportation','Transportation','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,'monthly_expenses'),
    ('exp_utilities','Utilities','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,'monthly_expenses'),
    ('exp_internet','Internet','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,'monthly_expenses'),
    ('exp_mobile','Mobile plan','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,'monthly_expenses'),
    ('exp_entertainment','Entertainment','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,'monthly_expenses'),
    ('exp_health','Health insurance','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,'monthly_expenses'),
    ('qf_currency','Currency','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,'quick_facts'),
    ('qf_monthly_range','Average monthly cost','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,'quick_facts'),
    ('qf_best_for','Best for','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,'quick_facts')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_cost_of_living'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('major_hospitals','Major Hospitals','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('emergency_number','Emergency Number','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('healthcare_note','Healthcare Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_healthcare'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('rent_1br','Rent — 1BR (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('rent_3br','Rent — 3BR (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('popular_areas','Popular Areas','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('housing_note','Housing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_housing'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('unemployment_rate','Unemployment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('minimum_wage','Minimum Wage (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_salary','Average Salary (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('in_demand_sectors','In-Demand Sectors','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('major_employers','Major Employers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('job_market_note','Job Market Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_jobs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('capital','Capital','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('population','Population','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('major_cities','Major Cities','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('timezone','Timezone','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('official_language','Official Language','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('popular_places','Popular Places','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('safety_index','Safety Index (0-100)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('crime_level','Crime Level','select','[{"label":"Very Low","value":"Very Low"},{"label":"Low","value":"Low"},{"label":"Moderate","value":"Moderate"},{"label":"High","value":"High"},{"label":"Very High","value":"Very High"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('common_crimes','Common Crimes & Scams','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('areas_to_avoid','Areas to Avoid','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('natural_disaster_risks','Natural Disaster Risks','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('safety_tips','Safety Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_safety'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('top_districts','Top School Districts','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('public_school_rating','Public School Quality','rating',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('intl_schools','International Schools','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('homeschooling_legal','Homeschooling Legal','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('enrollment_note','Enrollment Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('education_note','Education Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_schools'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('income_tax_rate','State Income Tax Rate','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('sales_tax','State Sales Tax','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('property_tax_range','Property Tax Range','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('tax_note','State Tax Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_taxes'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('transit_coverage','Public Transit Coverage','select','[{"label":"Extensive","value":"Extensive"},{"label":"Good","value":"Good"},{"label":"Limited","value":"Limited"},{"label":"Minimal","value":"Minimal"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('major_highways','Major Highways','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('intercity_options','Intercity Travel Options','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('airports','Airports','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('driving_note','Driving & License Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_transportation'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('climate_type','Climate Type','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('avg_temp_summer','Avg Summer Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('avg_temp_winter','Avg Winter Temp','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('rainy_season','Rainy Season','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('best_months','Best Months to Visit','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('weather_note','Weather Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_state_weather'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL),
    ('enrollment_proof','Enrollment Proof Required (CAS/CoE/LOA)','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,13,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_student_visa_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL),
    ('max_stay_per_visit','Max Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,13,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_tourist_visa_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('on_campus_available','On-Campus Housing','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('dorm_cost','Dorm Cost (monthly)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('offcampus_avg_rent','Off-Campus Avg Rent','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('housing_process','Housing Application','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_accommodation'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('acceptance_rate','Acceptance Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('application_fee','Application Fee','currency',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('application_deadline','Application Deadline','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('ielts_minimum','IELTS Minimum','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('toefl_minimum','TOEFL Minimum (iBT)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('application_portal','Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('requirements_note','Entry Requirements','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_admissions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('notable_alumni','Notable Alumni','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('alumni_network_size','Alumni Network Size','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('linkedin_url','Alumni LinkedIn Page','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('alumni_note','Alumni Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_alumni'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('student_clubs','Student Clubs & Societies','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('sports_facilities','Sports Facilities','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('intl_student_office','Intl Student Office','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('campus_note','Campus Life Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_campus_life'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('career_services','Career Services','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('employment_rate','Graduate Employment Rate','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('internship_support','Internship Support','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('top_employers','Top Employers','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_careers'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('libraries','Libraries','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('research_centers','Research Centers & Labs','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('sports_recreation','Sports & Recreation','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('health_services','On-Campus Health Services','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('facilities_note','Facilities Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_facilities'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('intl_office','International Student Office','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('orientation_program','Intl Orientation Program','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('visa_support','Visa & Immigration Support','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('buddy_program','Buddy / Mentor Program','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('language_support','Language Support','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('intl_note','International Student Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_intl_students'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('founded_year','Founded','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('institution_type','Institution Type','select','["Public","Private"]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('student_population','Student Population','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('international_students_pct','International Students','percentage',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('official_website','Official Website','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('notable_for','Notable For','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('bachelor_programs','Bachelor''s Programs (per year)','price_parts','{"parts":[{"key":"local","kind":"money","label":"Domestic"},{"key":"intl","kind":"money","label":"International"}],"primary_part":"intl"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('master_programs','Master''s Programs (per year)','price_parts','{"parts":[{"key":"local","kind":"money","label":"Domestic"},{"key":"intl","kind":"money","label":"International"}],"primary_part":"intl"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('doctoral_programs','Doctoral Programs (per year)','price_parts','{"parts":[{"key":"local","kind":"money","label":"Domestic"},{"key":"intl","kind":"money","label":"International"}],"primary_part":"intl"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('professional_programs','Professional Programs — MBA/Med/Law (per year)','price_parts','{"parts":[{"key":"local","kind":"money","label":"Domestic"},{"key":"intl","kind":"money","label":"International"}],"primary_part":"intl"}'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_page_program_fees'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('degree_levels','Degree Levels','multiselect','["Bachelor","Master","PhD","Diploma","Certificate"]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('popular_programs','Popular Programs','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('faculties_count','Faculties / Schools','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('catalog_url','Programs Catalog','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_programs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('qs_rank','QS World Ranking','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('the_rank','THE World Ranking','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('national_rank','National Ranking','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('subject_ranks','Notable Subject Rankings','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('accreditations','Accreditations','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('ranking_note','Ranking Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_rankings'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('merit_scholarships','Merit Scholarships','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('need_based_aid','Need-Based Aid','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('scholarship_list','Available Scholarships','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('scholarships_url','Scholarships Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('scholarships_note','Scholarship Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_scholarships'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('tuition_ug_local','Undergrad — Domestic (yr)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,'tuition_fees'),
    ('tuition_ug_intl','Undergrad — Intl (yr)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,'tuition_fees'),
    ('tuition_pg_local','Postgrad — Domestic (yr)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,'tuition_fees'),
    ('tuition_pg_intl','Postgrad — Intl (yr)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,'tuition_fees'),
    ('fees_note','Fees & Payment Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,'tuition_fees')
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_university_tuition_fees'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_visa_route_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('apply_method','How to Apply','select','[{"label":"Online","value":"Online"},{"label":"Embassy or Consulate","value":"Embassy or Consulate"},{"label":"Visa Application Centre","value":"Visa Application Centre"},{"label":"On Arrival","value":"On Arrival"},{"label":"Hybrid","value":"Hybrid"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('steps_summary','Step-by-Step Overview','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('biometrics_required','Biometrics Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('interview_required','Interview Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('apply_url','Official Application Portal','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('process_note','Process Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_application_process'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('dependents_allowed','Dependents Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('eligible_dependents','Eligible Dependents','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('dependent_work_rights','Dependent Work Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('dependent_study_rights','Dependent Study Rights','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('dependent_fee','Dependent Fee (each)','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('dependents_note','Dependents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_dependents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('documents_list','Document Checklist','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('certified_translation','Certified Translation Needed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('attestation_note','Attestation / Legalization','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('photo_requirements','Photo Requirements','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('documents_note','Documents Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_documents'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('application_fee','Application Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('biometrics_fee','Biometrics Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('health_surcharge','Health / Insurance Surcharge','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('priority_fee','Priority / Expedited Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('agent_fees','Typical Agent / Lawyer Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('total_estimate','Estimated Total Cost','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('fees_note','Fees Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_fees_costs'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('local_tips','Local Tips','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('useful_links','Useful Links','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_other'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('route_type','Route Type','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,1,NULL),
    ('processing_time','Processing Time','text',NULL,NULL,NULL,NULL,false,true,true,true,false,true,2,NULL),
    ('application_fee','Application Fee','number',NULL,NULL,NULL,NULL,false,true,true,true,false,true,3,NULL),
    ('eligibility','Eligibility','textarea',NULL,NULL,NULL,NULL,false,true,true,false,false,true,4,NULL),
    ('official_link','Official Link','url',NULL,NULL,NULL,NULL,false,true,true,false,false,true,5,NULL),
    ('validity_duration','Visa Validity / Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('max_stay','Maximum Stay per Visit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_overview'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('standard_time','Standard Processing Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('priority_time','Priority / Expedited Time','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('current_backlog','Current Backlog','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('factors','Factors Affecting Time','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('track_url','Track Application URL','url',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('processing_note','Processing Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_processing_time'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('common_refusal_reasons','Common Refusal Reasons','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('appeal_possible','Appeal Possible','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('appeal_process','Appeal / Review Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('reapply_wait','Reapplication Wait','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('refusal_note','Refusal Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_refusals_appeals'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('extendable','Extendable','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('max_duration','Maximum Total Duration','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('renewal_fee','Renewal / Extension Fee','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('renewal_process','Renewal Process','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('counts_toward_pr','Counts Toward PR','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('path_to_citizenship','Path to Citizenship','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_renewal_extension'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('min_education','Minimum Education','select','[{"label":"None","value":"None"},{"label":"Secondary","value":"Secondary"},{"label":"Diploma","value":"Diploma"},{"label":"Bachelor''s Degree","value":"Bachelor''s Degree"},{"label":"Master''s Degree","value":"Master''s Degree"},{"label":"Doctorate","value":"Doctorate"}]'::jsonb,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('language_requirement','Language Requirement','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('work_experience','Work Experience Required','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('sponsor_required','Sponsorship Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('proof_of_funds','Proof of Funds','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('health_insurance_required','Health Insurance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('medical_exam_required','Medical Exam Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL),
    ('police_clearance_required','Police Clearance Required','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,8,NULL),
    ('age_limit','Age Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,9,NULL),
    ('annual_cap','Annual Cap / Ballot','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,10,NULL),
    ('points_threshold','Points Threshold (if points-based)','number',NULL,NULL,NULL,NULL,false,false,true,false,false,false,11,NULL),
    ('requirements_note','Requirements Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,12,NULL),
    ('salary_threshold','Salary Threshold','price_range',NULL,NULL,NULL,NULL,false,false,true,false,false,false,13,NULL),
    ('eligible_occupations','Eligible Occupations','repeater',NULL,NULL,NULL,NULL,false,false,true,false,false,false,14,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_requirements'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

INSERT INTO custom_fields
    (group_id, key, label, field_type, options, embed_platforms, placeholder,
     default_value, is_required, is_system, show_in_frontend, filterable,
     sortable, searchable, position, section)
SELECT g.id, m.key, m.label, m.field_type, m.options::jsonb, m.embed_platforms::text[],
       m.placeholder, m.default_value, m.is_required, m.is_system,
       m.show_in_frontend, m.filterable, m.sortable, m.searchable, m.position, m.section
FROM custom_field_groups g
JOIN (VALUES
    ('work_allowed','Work Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,1,NULL),
    ('work_hours_limit','Work Hours Limit','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,2,NULL),
    ('study_allowed','Study Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,3,NULL),
    ('self_employment','Self-Employment Allowed','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,4,NULL),
    ('public_funds_access','Access to Public Funds','text',NULL,NULL,NULL,NULL,false,false,true,false,false,false,5,NULL),
    ('multiple_entry','Multiple Entry','boolean',NULL,NULL,NULL,NULL,false,false,true,false,false,false,6,NULL),
    ('rights_note','Rights & Restrictions Notes','textarea',NULL,NULL,NULL,NULL,false,false,true,false,false,false,7,NULL)
) AS m(key, label, field_type, options, embed_platforms, placeholder,
        default_value, is_required, is_system, show_in_frontend, filterable,
        sortable, searchable, position, section) ON true
WHERE g.slug = 'wiki_work_visa_rights_restrictions'
ON CONFLICT (group_id, key) DO UPDATE SET
    label = EXCLUDED.label, field_type = EXCLUDED.field_type,
    options = EXCLUDED.options, embed_platforms = EXCLUDED.embed_platforms,
    placeholder = EXCLUDED.placeholder, default_value = EXCLUDED.default_value,
    is_required = EXCLUDED.is_required, show_in_frontend = EXCLUDED.show_in_frontend,
    filterable = EXCLUDED.filterable, sortable = EXCLUDED.sortable,
    searchable = EXCLUDED.searchable, position = EXCLUDED.position,
    section = EXCLUDED.section;

-- ---------------------------------------------------------------------------
-- 6. Field permissions for fields introduced by this migration — the standard
--    grant: every group can view, every non-guest group can edit. Fields that
--    pre-date this migration keep the grants their seed migrations set.
-- ---------------------------------------------------------------------------
INSERT INTO custom_field_permissions (field_id, user_group_id, can_view, can_edit)
SELECT f.id, ug.id, true, ug.slug <> 'guest'
FROM custom_fields f
JOIN custom_field_groups g ON g.id = f.group_id
JOIN (VALUES
    ('wiki_attraction_facilities','wheelchair_accessible'),
    ('wiki_attraction_facilities','guided_tours'),
    ('wiki_attraction_facilities','audio_guide'),
    ('wiki_attraction_facilities','restrooms_cafe'),
    ('wiki_attraction_facilities','parking'),
    ('wiki_attraction_facilities','photo_policy'),
    ('wiki_attraction_facilities','facilities_note'),
    ('wiki_attraction_history','established'),
    ('wiki_attraction_history','heritage_status'),
    ('wiki_attraction_history','historical_note'),
    ('wiki_attraction_nearby','nearby_attractions'),
    ('wiki_attraction_nearby','nearest_transit'),
    ('wiki_attraction_nearby','day_trip_note'),
    ('wiki_attraction_page_tickets','ticket_adult'),
    ('wiki_attraction_page_tickets','ticket_child'),
    ('wiki_attraction_page_tickets','ticket_senior'),
    ('wiki_attraction_page_tickets','ticket_student'),
    ('wiki_attraction_page_tickets','ticket_family'),
    ('wiki_attraction_page_tickets','guided_tour'),
    ('wiki_attraction_page_tickets','audio_guide'),
    ('wiki_attraction_page_tickets','annual_pass'),
    ('wiki_bank_accounts_fees','account_types'),
    ('wiki_bank_accounts_fees','monthly_fee'),
    ('wiki_bank_accounts_fees','min_opening_deposit'),
    ('wiki_bank_accounts_fees','atm_fee_own'),
    ('wiki_bank_accounts_fees','atm_fee_other'),
    ('wiki_bank_accounts_fees','debit_card_fee'),
    ('wiki_bank_accounts_fees','accounts_note'),
    ('wiki_bank_coverage','branch_count'),
    ('wiki_bank_coverage','atm_count'),
    ('wiki_bank_coverage','coverage_note'),
    ('wiki_bank_digital_banking','app_rating'),
    ('wiki_bank_digital_banking','online_features'),
    ('wiki_bank_digital_banking','ussd_available'),
    ('wiki_bank_digital_banking','instant_transfers'),
    ('wiki_bank_digital_banking','virtual_cards'),
    ('wiki_bank_digital_banking','digital_note'),
    ('wiki_bank_other','local_tips'),
    ('wiki_bank_other','useful_links'),
    ('wiki_bank_rates','savings_apy'),
    ('wiki_bank_rates','deposit_insurance'),
    ('wiki_bank_rates','insured_amount'),
    ('wiki_bank_rates','loan_rates'),
    ('wiki_bank_rates','rates_note'),
    ('wiki_bank_remittance_fx','intl_transfer_fee'),
    ('wiki_bank_remittance_fx','swift_available'),
    ('wiki_bank_remittance_fx','fx_margin'),
    ('wiki_bank_remittance_fx','supported_currencies'),
    ('wiki_bank_remittance_fx','transfer_speed'),
    ('wiki_bank_remittance_fx','remittance_note'),
    ('wiki_bank_requirements','non_resident_can_open'),
    ('wiki_bank_requirements','documents_required'),
    ('wiki_bank_requirements','min_deposit'),
    ('wiki_bank_requirements','in_person_required'),
    ('wiki_bank_requirements','online_opening'),
    ('wiki_bank_requirements','opening_time'),
    ('wiki_bank_requirements','requirements_note'),
    ('wiki_business_visa_application_process','apply_method'),
    ('wiki_business_visa_application_process','steps_summary'),
    ('wiki_business_visa_application_process','biometrics_required'),
    ('wiki_business_visa_application_process','interview_required'),
    ('wiki_business_visa_application_process','apply_url'),
    ('wiki_business_visa_application_process','process_note'),
    ('wiki_business_visa_dependents','dependents_allowed'),
    ('wiki_business_visa_dependents','eligible_dependents'),
    ('wiki_business_visa_dependents','dependent_work_rights'),
    ('wiki_business_visa_dependents','dependent_study_rights'),
    ('wiki_business_visa_dependents','dependent_fee'),
    ('wiki_business_visa_dependents','dependents_note'),
    ('wiki_business_visa_documents','documents_list'),
    ('wiki_business_visa_documents','certified_translation'),
    ('wiki_business_visa_documents','attestation_note'),
    ('wiki_business_visa_documents','photo_requirements'),
    ('wiki_business_visa_documents','documents_note'),
    ('wiki_business_visa_fees_costs','application_fee'),
    ('wiki_business_visa_fees_costs','biometrics_fee'),
    ('wiki_business_visa_fees_costs','health_surcharge'),
    ('wiki_business_visa_fees_costs','priority_fee'),
    ('wiki_business_visa_fees_costs','agent_fees'),
    ('wiki_business_visa_fees_costs','total_estimate'),
    ('wiki_business_visa_fees_costs','fees_note'),
    ('wiki_business_visa_other','local_tips'),
    ('wiki_business_visa_other','useful_links'),
    ('wiki_business_visa_overview','validity_duration'),
    ('wiki_business_visa_overview','max_stay'),
    ('wiki_business_visa_processing_time','standard_time'),
    ('wiki_business_visa_processing_time','priority_time'),
    ('wiki_business_visa_processing_time','current_backlog'),
    ('wiki_business_visa_processing_time','factors'),
    ('wiki_business_visa_processing_time','track_url'),
    ('wiki_business_visa_processing_time','processing_note'),
    ('wiki_business_visa_refusals_appeals','common_refusal_reasons'),
    ('wiki_business_visa_refusals_appeals','appeal_possible'),
    ('wiki_business_visa_refusals_appeals','appeal_process'),
    ('wiki_business_visa_refusals_appeals','reapply_wait'),
    ('wiki_business_visa_refusals_appeals','refusal_note'),
    ('wiki_business_visa_renewal_extension','extendable'),
    ('wiki_business_visa_renewal_extension','max_duration'),
    ('wiki_business_visa_renewal_extension','renewal_fee'),
    ('wiki_business_visa_renewal_extension','renewal_process'),
    ('wiki_business_visa_renewal_extension','counts_toward_pr'),
    ('wiki_business_visa_renewal_extension','path_to_citizenship'),
    ('wiki_business_visa_requirements','min_education'),
    ('wiki_business_visa_requirements','language_requirement'),
    ('wiki_business_visa_requirements','work_experience'),
    ('wiki_business_visa_requirements','sponsor_required'),
    ('wiki_business_visa_requirements','proof_of_funds'),
    ('wiki_business_visa_requirements','health_insurance_required'),
    ('wiki_business_visa_requirements','medical_exam_required'),
    ('wiki_business_visa_requirements','police_clearance_required'),
    ('wiki_business_visa_requirements','age_limit'),
    ('wiki_business_visa_requirements','annual_cap'),
    ('wiki_business_visa_requirements','points_threshold'),
    ('wiki_business_visa_requirements','requirements_note'),
    ('wiki_business_visa_requirements','investment_threshold'),
    ('wiki_business_visa_rights_restrictions','work_allowed'),
    ('wiki_business_visa_rights_restrictions','work_hours_limit'),
    ('wiki_business_visa_rights_restrictions','study_allowed'),
    ('wiki_business_visa_rights_restrictions','self_employment'),
    ('wiki_business_visa_rights_restrictions','public_funds_access'),
    ('wiki_business_visa_rights_restrictions','multiple_entry'),
    ('wiki_business_visa_rights_restrictions','rights_note'),
    ('wiki_city_banking','local_banks'),
    ('wiki_city_banking','intl_banks'),
    ('wiki_city_banking','neobanks_fintech'),
    ('wiki_city_banking','atm_coverage'),
    ('wiki_city_banking','cash_vs_card'),
    ('wiki_city_banking','banking_note'),
    ('wiki_city_jobs','avg_salary'),
    ('wiki_city_jobs','minimum_wage'),
    ('wiki_city_jobs','unemployment_rate'),
    ('wiki_city_jobs','top_industries'),
    ('wiki_city_jobs','major_employers'),
    ('wiki_city_jobs','job_portals'),
    ('wiki_city_jobs','job_market_note'),
    ('wiki_city_lifestyle','nightlife_areas'),
    ('wiki_city_lifestyle','sports_fitness'),
    ('wiki_city_lifestyle','parks_outdoors'),
    ('wiki_city_lifestyle','cultural_venues'),
    ('wiki_city_lifestyle','annual_events'),
    ('wiki_city_lifestyle','lifestyle_note'),
    ('wiki_city_page_clothing_shoes','jeans'),
    ('wiki_city_page_clothing_shoes','dress'),
    ('wiki_city_page_clothing_shoes','sneakers'),
    ('wiki_city_page_clothing_shoes','business_shoes'),
    ('wiki_city_page_clothing_shoes','school_uniform'),
    ('wiki_city_page_rent_prices','studio'),
    ('wiki_city_page_rent_prices','one_bedroom'),
    ('wiki_city_page_rent_prices','two_bedroom'),
    ('wiki_city_page_rent_prices','three_bedroom'),
    ('wiki_city_page_rent_prices','qf_best_value_area'),
    ('wiki_city_page_salaries_financing','avg_monthly_salary'),
    ('wiki_city_page_salaries_financing','median_monthly_salary'),
    ('wiki_city_page_salaries_financing','minimum_wage_monthly'),
    ('wiki_city_page_salaries_financing','mortgage_rate'),
    ('wiki_city_page_salaries_financing','salary_note'),
    ('wiki_city_prices','qf_currency'),
    ('wiki_city_prices','qf_monthly_cost'),
    ('wiki_city_prices','qf_population'),
    ('wiki_city_prices','qf_best_for'),
    ('wiki_city_prices_childcare','daycare_monthly'),
    ('wiki_city_prices_childcare','preschool_monthly'),
    ('wiki_city_prices_childcare','nanny_monthly'),
    ('wiki_city_prices_childcare','babysitter_hourly'),
    ('wiki_city_prices_childcare','intl_primary_annual'),
    ('wiki_city_prices_childcare','intl_secondary_annual'),
    ('wiki_city_prices_childcare','public_school_fees'),
    ('wiki_city_prices_healthcare','gp_visit'),
    ('wiki_city_prices_healthcare','specialist_visit'),
    ('wiki_city_prices_healthcare','dental_checkup'),
    ('wiki_city_prices_healthcare','eye_exam'),
    ('wiki_city_prices_healthcare','therapy_session'),
    ('wiki_city_prices_healthcare','health_insurance_monthly'),
    ('wiki_city_prices_healthcare','prescription_common'),
    ('wiki_city_prices_healthcare','er_visit'),
    ('wiki_city_prices_rent_housing','rent_room_shared'),
    ('wiki_city_prices_rent_housing','rent_studio'),
    ('wiki_city_prices_rent_housing','rent_1br_centre'),
    ('wiki_city_prices_rent_housing','rent_1br_outside'),
    ('wiki_city_prices_rent_housing','rent_2br_centre'),
    ('wiki_city_prices_rent_housing','rent_2br_outside'),
    ('wiki_city_prices_rent_housing','rent_3br_centre'),
    ('wiki_city_prices_rent_housing','rent_3br_outside'),
    ('wiki_city_prices_rent_housing','buy_sqm_centre'),
    ('wiki_city_prices_rent_housing','buy_sqm_outside'),
    ('wiki_city_prices_rent_housing','deposit_months'),
    ('wiki_city_prices_rent_housing','agent_fee_pct'),
    ('wiki_city_safety','safety_index'),
    ('wiki_city_safety','crime_level'),
    ('wiki_city_safety','safe_neighborhoods'),
    ('wiki_city_safety','areas_to_avoid'),
    ('wiki_city_safety','common_scams'),
    ('wiki_city_safety','safety_tips'),
    ('wiki_city_schools','top_schools'),
    ('wiki_city_schools','intl_schools'),
    ('wiki_city_schools','school_quality'),
    ('wiki_city_schools','daycare_cost'),
    ('wiki_city_schools','enrollment_note'),
    ('wiki_city_transportation','transit_authority'),
    ('wiki_city_transportation','metro_available'),
    ('wiki_city_transportation','transit_card'),
    ('wiki_city_transportation','monthly_pass'),
    ('wiki_city_transportation','airport_name'),
    ('wiki_city_transportation','airport_distance'),
    ('wiki_city_transportation','ride_hailing'),
    ('wiki_city_transportation','bike_share'),
    ('wiki_city_transportation','walkability_score'),
    ('wiki_city_transportation','commute_note'),
    ('wiki_city_utilities','utility_providers'),
    ('wiki_city_utilities','internet_providers'),
    ('wiki_city_utilities','internet_speed'),
    ('wiki_city_utilities','internet_cost'),
    ('wiki_city_utilities','mobile_operators'),
    ('wiki_city_utilities','power_reliability'),
    ('wiki_city_utilities','utilities_note'),
    ('wiki_city_weather','climate_type'),
    ('wiki_city_weather','avg_temp_summer'),
    ('wiki_city_weather','avg_temp_winter'),
    ('wiki_city_weather','annual_rainfall'),
    ('wiki_city_weather','rainy_season'),
    ('wiki_city_weather','best_months'),
    ('wiki_city_weather','weather_note'),
    ('wiki_country_business','ease_of_business'),
    ('wiki_country_business','corporate_tax'),
    ('wiki_country_business','business_registration'),
    ('wiki_country_business','startup_visa'),
    ('wiki_country_business','business_note'),
    ('wiki_country_culture','tipping_custom'),
    ('wiki_country_culture','religions'),
    ('wiki_country_culture','cuisine_staples'),
    ('wiki_country_culture','festivals_events'),
    ('wiki_country_culture','expat_friendliness'),
    ('wiki_country_culture','cultural_etiquette'),
    ('wiki_country_documents','attestation_process'),
    ('wiki_country_documents','apostille_available'),
    ('wiki_country_documents','document_sources'),
    ('wiki_country_documents','certified_translation'),
    ('wiki_country_documents','legalization_note'),
    ('wiki_country_education','education_system'),
    ('wiki_country_education','literacy_rate'),
    ('wiki_country_education','school_year'),
    ('wiki_country_education','public_school_free'),
    ('wiki_country_education','language_of_instruction'),
    ('wiki_country_education','intl_schools'),
    ('wiki_country_education','homeschooling_legal'),
    ('wiki_country_education','education_note'),
    ('wiki_country_jobs','unemployment_rate'),
    ('wiki_country_jobs','minimum_wage'),
    ('wiki_country_jobs','avg_salary'),
    ('wiki_country_jobs','in_demand_sectors'),
    ('wiki_country_jobs','major_employers'),
    ('wiki_country_jobs','job_portals'),
    ('wiki_country_jobs','economy_note'),
    ('wiki_country_overview','capital'),
    ('wiki_country_overview','timezones'),
    ('wiki_country_overview','calling_code'),
    ('wiki_country_overview','official_language'),
    ('wiki_country_safety','safety_index'),
    ('wiki_country_safety','crime_level'),
    ('wiki_country_safety','common_crimes'),
    ('wiki_country_safety','regions_to_avoid'),
    ('wiki_country_safety','emergency_number'),
    ('wiki_country_safety','natural_disaster_risks'),
    ('wiki_country_safety','travel_advisory'),
    ('wiki_country_safety','safety_tips'),
    ('wiki_country_settling_in','first_week_checklist'),
    ('wiki_country_settling_in','sim_card_note'),
    ('wiki_country_settling_in','bank_setup_note'),
    ('wiki_country_settling_in','transit_card_note'),
    ('wiki_country_settling_in','id_registration'),
    ('wiki_country_settling_in','health_registration'),
    ('wiki_country_settling_in','settling_note'),
    ('wiki_country_taxes','income_tax_range'),
    ('wiki_country_taxes','top_marginal_rate'),
    ('wiki_country_taxes','sales_tax_vat'),
    ('wiki_country_taxes','social_security_rate'),
    ('wiki_country_taxes','tax_residency_rule'),
    ('wiki_country_taxes','tax_treaties'),
    ('wiki_country_taxes','expat_tax_note'),
    ('wiki_country_transportation','rail_network'),
    ('wiki_country_transportation','major_airports'),
    ('wiki_country_transportation','domestic_flights'),
    ('wiki_country_transportation','intercity_buses'),
    ('wiki_country_transportation','road_quality'),
    ('wiki_country_transportation','driving_note'),
    ('wiki_country_transportation','transport_note'),
    ('wiki_country_weather','climate_zones'),
    ('wiki_country_weather','avg_temp_summer'),
    ('wiki_country_weather','avg_temp_winter'),
    ('wiki_country_weather','rainy_season'),
    ('wiki_country_weather','best_months'),
    ('wiki_country_weather','weather_note'),
    ('wiki_course_career_outcomes','employment_rate'),
    ('wiki_course_career_outcomes','avg_starting_salary'),
    ('wiki_course_career_outcomes','common_roles'),
    ('wiki_course_career_outcomes','top_employers'),
    ('wiki_course_career_outcomes','licensing_note'),
    ('wiki_course_curriculum','credits_required'),
    ('wiki_course_curriculum','core_modules'),
    ('wiki_course_curriculum','elective_options'),
    ('wiki_course_curriculum','thesis_required'),
    ('wiki_course_curriculum','internship_included'),
    ('wiki_course_curriculum','curriculum_note'),
    ('wiki_course_delivery','study_modes'),
    ('wiki_course_delivery','duration_fulltime'),
    ('wiki_course_delivery','duration_parttime'),
    ('wiki_course_delivery','intake_months'),
    ('wiki_course_delivery','schedule_note'),
    ('wiki_course_entry_requirements','min_education'),
    ('wiki_course_entry_requirements','gpa_requirement'),
    ('wiki_course_entry_requirements','language_tests'),
    ('wiki_course_entry_requirements','prerequisites'),
    ('wiki_course_entry_requirements','entrance_exam'),
    ('wiki_course_entry_requirements','portfolio_required'),
    ('wiki_course_entry_requirements','work_experience'),
    ('wiki_course_entry_requirements','requirements_note'),
    ('wiki_course_other','local_tips'),
    ('wiki_course_other','useful_links'),
    ('wiki_course_tuition_fees','tuition_local'),
    ('wiki_course_tuition_fees','tuition_intl'),
    ('wiki_course_tuition_fees','per_credit_cost'),
    ('wiki_course_tuition_fees','other_fees'),
    ('wiki_course_tuition_fees','deposit_required'),
    ('wiki_course_tuition_fees','payment_plan'),
    ('wiki_course_tuition_fees','fees_note'),
    ('wiki_embassy_appointment_booking','appointment_required'),
    ('wiki_embassy_appointment_booking','booking_method'),
    ('wiki_embassy_appointment_booking','booking_url'),
    ('wiki_embassy_appointment_booking','typical_wait'),
    ('wiki_embassy_appointment_booking','walk_ins_allowed'),
    ('wiki_embassy_appointment_booking','appointment_note'),
    ('wiki_embassy_consular_services','services_offered'),
    ('wiki_embassy_consular_services','visa_processing'),
    ('wiki_embassy_consular_services','emergency_passport'),
    ('wiki_embassy_consular_services','notarial_services'),
    ('wiki_embassy_consular_services','citizen_registration'),
    ('wiki_embassy_consular_services','services_note'),
    ('wiki_embassy_consular_services','service_processing_times'),
    ('wiki_embassy_contact_location','address'),
    ('wiki_embassy_contact_location','phone'),
    ('wiki_embassy_contact_location','emergency_phone'),
    ('wiki_embassy_contact_location','email'),
    ('wiki_embassy_contact_location','website'),
    ('wiki_embassy_contact_location','opening_hours'),
    ('wiki_embassy_contact_location','nearest_transit'),
    ('wiki_embassy_contact_location','location_note'),
    ('wiki_embassy_holidays','observed_holidays'),
    ('wiki_embassy_holidays','closure_note'),
    ('wiki_embassy_holidays','emergency_duty'),
    ('wiki_embassy_jurisdiction','consular_district'),
    ('wiki_embassy_jurisdiction','honorary_consulates'),
    ('wiki_embassy_jurisdiction','jurisdiction_note'),
    ('wiki_embassy_other','local_tips'),
    ('wiki_embassy_other','useful_links'),
    ('wiki_family_visa_application_process','apply_method'),
    ('wiki_family_visa_application_process','steps_summary'),
    ('wiki_family_visa_application_process','biometrics_required'),
    ('wiki_family_visa_application_process','interview_required'),
    ('wiki_family_visa_application_process','apply_url'),
    ('wiki_family_visa_application_process','process_note'),
    ('wiki_family_visa_dependents','dependents_allowed'),
    ('wiki_family_visa_dependents','eligible_dependents'),
    ('wiki_family_visa_dependents','dependent_work_rights'),
    ('wiki_family_visa_dependents','dependent_study_rights'),
    ('wiki_family_visa_dependents','dependent_fee'),
    ('wiki_family_visa_dependents','dependents_note'),
    ('wiki_family_visa_documents','documents_list'),
    ('wiki_family_visa_documents','certified_translation'),
    ('wiki_family_visa_documents','attestation_note'),
    ('wiki_family_visa_documents','photo_requirements'),
    ('wiki_family_visa_documents','documents_note'),
    ('wiki_family_visa_fees_costs','application_fee'),
    ('wiki_family_visa_fees_costs','biometrics_fee'),
    ('wiki_family_visa_fees_costs','health_surcharge'),
    ('wiki_family_visa_fees_costs','priority_fee'),
    ('wiki_family_visa_fees_costs','agent_fees'),
    ('wiki_family_visa_fees_costs','total_estimate'),
    ('wiki_family_visa_fees_costs','fees_note'),
    ('wiki_family_visa_other','local_tips'),
    ('wiki_family_visa_other','useful_links'),
    ('wiki_family_visa_overview','validity_duration'),
    ('wiki_family_visa_overview','max_stay'),
    ('wiki_family_visa_processing_time','standard_time'),
    ('wiki_family_visa_processing_time','priority_time'),
    ('wiki_family_visa_processing_time','current_backlog'),
    ('wiki_family_visa_processing_time','factors'),
    ('wiki_family_visa_processing_time','track_url'),
    ('wiki_family_visa_processing_time','processing_note'),
    ('wiki_family_visa_refusals_appeals','common_refusal_reasons'),
    ('wiki_family_visa_refusals_appeals','appeal_possible'),
    ('wiki_family_visa_refusals_appeals','appeal_process'),
    ('wiki_family_visa_refusals_appeals','reapply_wait'),
    ('wiki_family_visa_refusals_appeals','refusal_note'),
    ('wiki_family_visa_renewal_extension','extendable'),
    ('wiki_family_visa_renewal_extension','max_duration'),
    ('wiki_family_visa_renewal_extension','renewal_fee'),
    ('wiki_family_visa_renewal_extension','renewal_process'),
    ('wiki_family_visa_renewal_extension','counts_toward_pr'),
    ('wiki_family_visa_renewal_extension','path_to_citizenship'),
    ('wiki_family_visa_requirements','min_education'),
    ('wiki_family_visa_requirements','language_requirement'),
    ('wiki_family_visa_requirements','work_experience'),
    ('wiki_family_visa_requirements','sponsor_required'),
    ('wiki_family_visa_requirements','proof_of_funds'),
    ('wiki_family_visa_requirements','health_insurance_required'),
    ('wiki_family_visa_requirements','medical_exam_required'),
    ('wiki_family_visa_requirements','police_clearance_required'),
    ('wiki_family_visa_requirements','age_limit'),
    ('wiki_family_visa_requirements','annual_cap'),
    ('wiki_family_visa_requirements','points_threshold'),
    ('wiki_family_visa_requirements','requirements_note'),
    ('wiki_family_visa_rights_restrictions','work_allowed'),
    ('wiki_family_visa_rights_restrictions','work_hours_limit'),
    ('wiki_family_visa_rights_restrictions','study_allowed'),
    ('wiki_family_visa_rights_restrictions','self_employment'),
    ('wiki_family_visa_rights_restrictions','public_funds_access'),
    ('wiki_family_visa_rights_restrictions','multiple_entry'),
    ('wiki_family_visa_rights_restrictions','rights_note'),
    ('wiki_neighborhood_amenities','supermarkets'),
    ('wiki_neighborhood_amenities','dining_scene'),
    ('wiki_neighborhood_amenities','parks'),
    ('wiki_neighborhood_amenities','healthcare_nearby'),
    ('wiki_neighborhood_amenities','amenities_note'),
    ('wiki_neighborhood_demographics','typical_residents'),
    ('wiki_neighborhood_demographics','avg_age'),
    ('wiki_neighborhood_demographics','household_types'),
    ('wiki_neighborhood_demographics','expat_presence'),
    ('wiki_neighborhood_demographics','demographics_note'),
    ('wiki_neighborhood_other','local_tips'),
    ('wiki_neighborhood_other','useful_links'),
    ('wiki_neighborhood_rent_prices','rent_room'),
    ('wiki_neighborhood_rent_prices','rent_studio'),
    ('wiki_neighborhood_rent_prices','rent_1br'),
    ('wiki_neighborhood_rent_prices','rent_2br'),
    ('wiki_neighborhood_rent_prices','rent_3br'),
    ('wiki_neighborhood_rent_prices','rent_trend'),
    ('wiki_neighborhood_rent_prices','deposit_norm'),
    ('wiki_neighborhood_rent_prices','rent_note'),
    ('wiki_neighborhood_safety','safety_rating'),
    ('wiki_neighborhood_safety','common_concerns'),
    ('wiki_neighborhood_safety','police_presence'),
    ('wiki_neighborhood_safety','safety_tips'),
    ('wiki_neighborhood_schools_family','schools_nearby'),
    ('wiki_neighborhood_schools_family','family_friendly'),
    ('wiki_neighborhood_schools_family','childcare_note'),
    ('wiki_neighborhood_transport','nearest_transit'),
    ('wiki_neighborhood_transport','transit_lines'),
    ('wiki_neighborhood_transport','commute_to_centre'),
    ('wiki_neighborhood_transport','walkability'),
    ('wiki_neighborhood_transport','parking_situation'),
    ('wiki_pr_pathway_application_process','apply_method'),
    ('wiki_pr_pathway_application_process','steps_summary'),
    ('wiki_pr_pathway_application_process','biometrics_required'),
    ('wiki_pr_pathway_application_process','interview_required'),
    ('wiki_pr_pathway_application_process','apply_url'),
    ('wiki_pr_pathway_application_process','process_note'),
    ('wiki_pr_pathway_dependents','dependents_allowed'),
    ('wiki_pr_pathway_dependents','eligible_dependents'),
    ('wiki_pr_pathway_dependents','dependent_work_rights'),
    ('wiki_pr_pathway_dependents','dependent_study_rights'),
    ('wiki_pr_pathway_dependents','dependent_fee'),
    ('wiki_pr_pathway_dependents','dependents_note'),
    ('wiki_pr_pathway_documents','documents_list'),
    ('wiki_pr_pathway_documents','certified_translation'),
    ('wiki_pr_pathway_documents','attestation_note'),
    ('wiki_pr_pathway_documents','photo_requirements'),
    ('wiki_pr_pathway_documents','documents_note'),
    ('wiki_pr_pathway_fees_costs','application_fee'),
    ('wiki_pr_pathway_fees_costs','biometrics_fee'),
    ('wiki_pr_pathway_fees_costs','health_surcharge'),
    ('wiki_pr_pathway_fees_costs','priority_fee'),
    ('wiki_pr_pathway_fees_costs','agent_fees'),
    ('wiki_pr_pathway_fees_costs','total_estimate'),
    ('wiki_pr_pathway_fees_costs','fees_note'),
    ('wiki_pr_pathway_other','local_tips'),
    ('wiki_pr_pathway_other','useful_links'),
    ('wiki_pr_pathway_overview','validity_duration'),
    ('wiki_pr_pathway_overview','max_stay'),
    ('wiki_pr_pathway_processing_time','standard_time'),
    ('wiki_pr_pathway_processing_time','priority_time'),
    ('wiki_pr_pathway_processing_time','current_backlog'),
    ('wiki_pr_pathway_processing_time','factors'),
    ('wiki_pr_pathway_processing_time','track_url'),
    ('wiki_pr_pathway_processing_time','processing_note'),
    ('wiki_pr_pathway_refusals_appeals','common_refusal_reasons'),
    ('wiki_pr_pathway_refusals_appeals','appeal_possible'),
    ('wiki_pr_pathway_refusals_appeals','appeal_process'),
    ('wiki_pr_pathway_refusals_appeals','reapply_wait'),
    ('wiki_pr_pathway_refusals_appeals','refusal_note'),
    ('wiki_pr_pathway_renewal_extension','extendable'),
    ('wiki_pr_pathway_renewal_extension','max_duration'),
    ('wiki_pr_pathway_renewal_extension','renewal_fee'),
    ('wiki_pr_pathway_renewal_extension','renewal_process'),
    ('wiki_pr_pathway_renewal_extension','counts_toward_pr'),
    ('wiki_pr_pathway_renewal_extension','path_to_citizenship'),
    ('wiki_pr_pathway_requirements','min_education'),
    ('wiki_pr_pathway_requirements','language_requirement'),
    ('wiki_pr_pathway_requirements','work_experience'),
    ('wiki_pr_pathway_requirements','sponsor_required'),
    ('wiki_pr_pathway_requirements','proof_of_funds'),
    ('wiki_pr_pathway_requirements','health_insurance_required'),
    ('wiki_pr_pathway_requirements','medical_exam_required'),
    ('wiki_pr_pathway_requirements','police_clearance_required'),
    ('wiki_pr_pathway_requirements','age_limit'),
    ('wiki_pr_pathway_requirements','annual_cap'),
    ('wiki_pr_pathway_requirements','points_threshold'),
    ('wiki_pr_pathway_requirements','requirements_note'),
    ('wiki_pr_pathway_requirements','residency_years'),
    ('wiki_pr_pathway_requirements','points_breakdown'),
    ('wiki_pr_pathway_rights_restrictions','work_allowed'),
    ('wiki_pr_pathway_rights_restrictions','work_hours_limit'),
    ('wiki_pr_pathway_rights_restrictions','study_allowed'),
    ('wiki_pr_pathway_rights_restrictions','self_employment'),
    ('wiki_pr_pathway_rights_restrictions','public_funds_access'),
    ('wiki_pr_pathway_rights_restrictions','multiple_entry'),
    ('wiki_pr_pathway_rights_restrictions','rights_note'),
    ('wiki_program_career_outcomes','employment_rate'),
    ('wiki_program_career_outcomes','avg_starting_salary'),
    ('wiki_program_career_outcomes','common_roles'),
    ('wiki_program_career_outcomes','professional_licensure'),
    ('wiki_program_career_outcomes','outlook_note'),
    ('wiki_program_curriculum','core_subjects'),
    ('wiki_program_curriculum','typical_credits'),
    ('wiki_program_curriculum','specializations'),
    ('wiki_program_curriculum','thesis_typical'),
    ('wiki_program_curriculum','internship_typical'),
    ('wiki_program_curriculum','curriculum_note'),
    ('wiki_program_entry_requirements','min_education'),
    ('wiki_program_entry_requirements','gpa_requirement'),
    ('wiki_program_entry_requirements','language_tests'),
    ('wiki_program_entry_requirements','prerequisites'),
    ('wiki_program_entry_requirements','entrance_exams'),
    ('wiki_program_entry_requirements','requirements_note'),
    ('wiki_program_other','local_tips'),
    ('wiki_program_other','useful_links'),
    ('wiki_program_overview','program_level'),
    ('wiki_program_overview','typical_duration'),
    ('wiki_program_overview','field_category'),
    ('wiki_program_overview','demand_level'),
    ('wiki_program_overview','overview_note'),
    ('wiki_program_tuition_fees','tuition_local'),
    ('wiki_program_tuition_fees','tuition_intl'),
    ('wiki_program_tuition_fees','funding_note'),
    ('wiki_scholarship_application_deadlines','application_opens'),
    ('wiki_scholarship_application_deadlines','deadline'),
    ('wiki_scholarship_application_deadlines','frequency'),
    ('wiki_scholarship_application_deadlines','application_url'),
    ('wiki_scholarship_application_deadlines','documents_required'),
    ('wiki_scholarship_application_deadlines','essay_required'),
    ('wiki_scholarship_application_deadlines','interview_required'),
    ('wiki_scholarship_application_deadlines','results_timing'),
    ('wiki_scholarship_coverage_benefits','tuition_covered'),
    ('wiki_scholarship_coverage_benefits','coverage_amount'),
    ('wiki_scholarship_coverage_benefits','living_stipend'),
    ('wiki_scholarship_coverage_benefits','accommodation_covered'),
    ('wiki_scholarship_coverage_benefits','travel_allowance'),
    ('wiki_scholarship_coverage_benefits','health_insurance'),
    ('wiki_scholarship_coverage_benefits','books_materials'),
    ('wiki_scholarship_coverage_benefits','funding_duration'),
    ('wiki_scholarship_coverage_benefits','benefits_note'),
    ('wiki_scholarship_coverage_benefits','service_obligation'),
    ('wiki_scholarship_eligibility','eligible_nationalities'),
    ('wiki_scholarship_eligibility','min_education'),
    ('wiki_scholarship_eligibility','gpa_requirement'),
    ('wiki_scholarship_eligibility','language_requirement'),
    ('wiki_scholarship_eligibility','merit_based'),
    ('wiki_scholarship_eligibility','need_based'),
    ('wiki_scholarship_eligibility','age_limit'),
    ('wiki_scholarship_eligibility','eligible_programs'),
    ('wiki_scholarship_eligibility','eligibility_note'),
    ('wiki_scholarship_other','local_tips'),
    ('wiki_scholarship_other','useful_links'),
    ('wiki_scholarship_selection','selection_criteria'),
    ('wiki_scholarship_selection','maintain_gpa'),
    ('wiki_scholarship_selection','renewal_conditions'),
    ('wiki_scholarship_selection','selection_note'),
    ('wiki_state_banking','major_banks'),
    ('wiki_state_banking','credit_unions'),
    ('wiki_state_banking','branch_coverage'),
    ('wiki_state_banking','banking_note'),
    ('wiki_state_jobs','unemployment_rate'),
    ('wiki_state_jobs','minimum_wage'),
    ('wiki_state_jobs','avg_salary'),
    ('wiki_state_jobs','in_demand_sectors'),
    ('wiki_state_jobs','major_employers'),
    ('wiki_state_jobs','job_market_note'),
    ('wiki_state_safety','safety_index'),
    ('wiki_state_safety','crime_level'),
    ('wiki_state_safety','common_crimes'),
    ('wiki_state_safety','areas_to_avoid'),
    ('wiki_state_safety','natural_disaster_risks'),
    ('wiki_state_safety','safety_tips'),
    ('wiki_state_schools','top_districts'),
    ('wiki_state_schools','public_school_rating'),
    ('wiki_state_schools','intl_schools'),
    ('wiki_state_schools','homeschooling_legal'),
    ('wiki_state_schools','enrollment_note'),
    ('wiki_state_schools','education_note'),
    ('wiki_state_taxes','income_tax_rate'),
    ('wiki_state_taxes','sales_tax'),
    ('wiki_state_taxes','property_tax_range'),
    ('wiki_state_taxes','tax_note'),
    ('wiki_state_transportation','transit_coverage'),
    ('wiki_state_transportation','major_highways'),
    ('wiki_state_transportation','intercity_options'),
    ('wiki_state_transportation','airports'),
    ('wiki_state_transportation','driving_note'),
    ('wiki_state_weather','climate_type'),
    ('wiki_state_weather','avg_temp_summer'),
    ('wiki_state_weather','avg_temp_winter'),
    ('wiki_state_weather','rainy_season'),
    ('wiki_state_weather','best_months'),
    ('wiki_state_weather','weather_note'),
    ('wiki_student_visa_application_process','apply_method'),
    ('wiki_student_visa_application_process','steps_summary'),
    ('wiki_student_visa_application_process','biometrics_required'),
    ('wiki_student_visa_application_process','interview_required'),
    ('wiki_student_visa_application_process','apply_url'),
    ('wiki_student_visa_application_process','process_note'),
    ('wiki_student_visa_dependents','dependents_allowed'),
    ('wiki_student_visa_dependents','eligible_dependents'),
    ('wiki_student_visa_dependents','dependent_work_rights'),
    ('wiki_student_visa_dependents','dependent_study_rights'),
    ('wiki_student_visa_dependents','dependent_fee'),
    ('wiki_student_visa_dependents','dependents_note'),
    ('wiki_student_visa_documents','documents_list'),
    ('wiki_student_visa_documents','certified_translation'),
    ('wiki_student_visa_documents','attestation_note'),
    ('wiki_student_visa_documents','photo_requirements'),
    ('wiki_student_visa_documents','documents_note'),
    ('wiki_student_visa_fees_costs','application_fee'),
    ('wiki_student_visa_fees_costs','biometrics_fee'),
    ('wiki_student_visa_fees_costs','health_surcharge'),
    ('wiki_student_visa_fees_costs','priority_fee'),
    ('wiki_student_visa_fees_costs','agent_fees'),
    ('wiki_student_visa_fees_costs','total_estimate'),
    ('wiki_student_visa_fees_costs','fees_note'),
    ('wiki_student_visa_other','local_tips'),
    ('wiki_student_visa_other','useful_links'),
    ('wiki_student_visa_overview','validity_duration'),
    ('wiki_student_visa_overview','max_stay'),
    ('wiki_student_visa_processing_time','standard_time'),
    ('wiki_student_visa_processing_time','priority_time'),
    ('wiki_student_visa_processing_time','current_backlog'),
    ('wiki_student_visa_processing_time','factors'),
    ('wiki_student_visa_processing_time','track_url'),
    ('wiki_student_visa_processing_time','processing_note'),
    ('wiki_student_visa_refusals_appeals','common_refusal_reasons'),
    ('wiki_student_visa_refusals_appeals','appeal_possible'),
    ('wiki_student_visa_refusals_appeals','appeal_process'),
    ('wiki_student_visa_refusals_appeals','reapply_wait'),
    ('wiki_student_visa_refusals_appeals','refusal_note'),
    ('wiki_student_visa_renewal_extension','extendable'),
    ('wiki_student_visa_renewal_extension','max_duration'),
    ('wiki_student_visa_renewal_extension','renewal_fee'),
    ('wiki_student_visa_renewal_extension','renewal_process'),
    ('wiki_student_visa_renewal_extension','counts_toward_pr'),
    ('wiki_student_visa_renewal_extension','path_to_citizenship'),
    ('wiki_student_visa_requirements','min_education'),
    ('wiki_student_visa_requirements','language_requirement'),
    ('wiki_student_visa_requirements','work_experience'),
    ('wiki_student_visa_requirements','sponsor_required'),
    ('wiki_student_visa_requirements','proof_of_funds'),
    ('wiki_student_visa_requirements','health_insurance_required'),
    ('wiki_student_visa_requirements','medical_exam_required'),
    ('wiki_student_visa_requirements','police_clearance_required'),
    ('wiki_student_visa_requirements','age_limit'),
    ('wiki_student_visa_requirements','annual_cap'),
    ('wiki_student_visa_requirements','points_threshold'),
    ('wiki_student_visa_requirements','requirements_note'),
    ('wiki_student_visa_requirements','enrollment_proof'),
    ('wiki_student_visa_rights_restrictions','work_allowed'),
    ('wiki_student_visa_rights_restrictions','work_hours_limit'),
    ('wiki_student_visa_rights_restrictions','study_allowed'),
    ('wiki_student_visa_rights_restrictions','self_employment'),
    ('wiki_student_visa_rights_restrictions','public_funds_access'),
    ('wiki_student_visa_rights_restrictions','multiple_entry'),
    ('wiki_student_visa_rights_restrictions','rights_note'),
    ('wiki_tourist_visa_application_process','apply_method'),
    ('wiki_tourist_visa_application_process','steps_summary'),
    ('wiki_tourist_visa_application_process','biometrics_required'),
    ('wiki_tourist_visa_application_process','interview_required'),
    ('wiki_tourist_visa_application_process','apply_url'),
    ('wiki_tourist_visa_application_process','process_note'),
    ('wiki_tourist_visa_dependents','dependents_allowed'),
    ('wiki_tourist_visa_dependents','eligible_dependents'),
    ('wiki_tourist_visa_dependents','dependent_work_rights'),
    ('wiki_tourist_visa_dependents','dependent_study_rights'),
    ('wiki_tourist_visa_dependents','dependent_fee'),
    ('wiki_tourist_visa_dependents','dependents_note'),
    ('wiki_tourist_visa_documents','documents_list'),
    ('wiki_tourist_visa_documents','certified_translation'),
    ('wiki_tourist_visa_documents','attestation_note'),
    ('wiki_tourist_visa_documents','photo_requirements'),
    ('wiki_tourist_visa_documents','documents_note'),
    ('wiki_tourist_visa_fees_costs','application_fee'),
    ('wiki_tourist_visa_fees_costs','biometrics_fee'),
    ('wiki_tourist_visa_fees_costs','health_surcharge'),
    ('wiki_tourist_visa_fees_costs','priority_fee'),
    ('wiki_tourist_visa_fees_costs','agent_fees'),
    ('wiki_tourist_visa_fees_costs','total_estimate'),
    ('wiki_tourist_visa_fees_costs','fees_note'),
    ('wiki_tourist_visa_other','local_tips'),
    ('wiki_tourist_visa_other','useful_links'),
    ('wiki_tourist_visa_overview','validity_duration'),
    ('wiki_tourist_visa_overview','max_stay'),
    ('wiki_tourist_visa_processing_time','standard_time'),
    ('wiki_tourist_visa_processing_time','priority_time'),
    ('wiki_tourist_visa_processing_time','current_backlog'),
    ('wiki_tourist_visa_processing_time','factors'),
    ('wiki_tourist_visa_processing_time','track_url'),
    ('wiki_tourist_visa_processing_time','processing_note'),
    ('wiki_tourist_visa_refusals_appeals','common_refusal_reasons'),
    ('wiki_tourist_visa_refusals_appeals','appeal_possible'),
    ('wiki_tourist_visa_refusals_appeals','appeal_process'),
    ('wiki_tourist_visa_refusals_appeals','reapply_wait'),
    ('wiki_tourist_visa_refusals_appeals','refusal_note'),
    ('wiki_tourist_visa_renewal_extension','extendable'),
    ('wiki_tourist_visa_renewal_extension','max_duration'),
    ('wiki_tourist_visa_renewal_extension','renewal_fee'),
    ('wiki_tourist_visa_renewal_extension','renewal_process'),
    ('wiki_tourist_visa_renewal_extension','counts_toward_pr'),
    ('wiki_tourist_visa_renewal_extension','path_to_citizenship'),
    ('wiki_tourist_visa_requirements','min_education'),
    ('wiki_tourist_visa_requirements','language_requirement'),
    ('wiki_tourist_visa_requirements','work_experience'),
    ('wiki_tourist_visa_requirements','sponsor_required'),
    ('wiki_tourist_visa_requirements','proof_of_funds'),
    ('wiki_tourist_visa_requirements','health_insurance_required'),
    ('wiki_tourist_visa_requirements','medical_exam_required'),
    ('wiki_tourist_visa_requirements','police_clearance_required'),
    ('wiki_tourist_visa_requirements','age_limit'),
    ('wiki_tourist_visa_requirements','annual_cap'),
    ('wiki_tourist_visa_requirements','points_threshold'),
    ('wiki_tourist_visa_requirements','requirements_note'),
    ('wiki_tourist_visa_requirements','max_stay_per_visit'),
    ('wiki_tourist_visa_rights_restrictions','work_allowed'),
    ('wiki_tourist_visa_rights_restrictions','work_hours_limit'),
    ('wiki_tourist_visa_rights_restrictions','study_allowed'),
    ('wiki_tourist_visa_rights_restrictions','self_employment'),
    ('wiki_tourist_visa_rights_restrictions','public_funds_access'),
    ('wiki_tourist_visa_rights_restrictions','multiple_entry'),
    ('wiki_tourist_visa_rights_restrictions','rights_note'),
    ('wiki_university_alumni','notable_alumni'),
    ('wiki_university_alumni','alumni_network_size'),
    ('wiki_university_alumni','linkedin_url'),
    ('wiki_university_alumni','alumni_note'),
    ('wiki_university_facilities','libraries'),
    ('wiki_university_facilities','research_centers'),
    ('wiki_university_facilities','sports_recreation'),
    ('wiki_university_facilities','health_services'),
    ('wiki_university_facilities','facilities_note'),
    ('wiki_university_intl_students','intl_office'),
    ('wiki_university_intl_students','orientation_program'),
    ('wiki_university_intl_students','visa_support'),
    ('wiki_university_intl_students','buddy_program'),
    ('wiki_university_intl_students','language_support'),
    ('wiki_university_intl_students','intl_note'),
    ('wiki_university_page_program_fees','bachelor_programs'),
    ('wiki_university_page_program_fees','master_programs'),
    ('wiki_university_page_program_fees','doctoral_programs'),
    ('wiki_university_page_program_fees','professional_programs'),
    ('wiki_university_rankings','qs_rank'),
    ('wiki_university_rankings','the_rank'),
    ('wiki_university_rankings','national_rank'),
    ('wiki_university_rankings','subject_ranks'),
    ('wiki_university_rankings','accreditations'),
    ('wiki_university_rankings','ranking_note'),
    ('wiki_visa_route_application_process','apply_method'),
    ('wiki_visa_route_application_process','steps_summary'),
    ('wiki_visa_route_application_process','biometrics_required'),
    ('wiki_visa_route_application_process','interview_required'),
    ('wiki_visa_route_application_process','apply_url'),
    ('wiki_visa_route_application_process','process_note'),
    ('wiki_visa_route_dependents','dependents_allowed'),
    ('wiki_visa_route_dependents','eligible_dependents'),
    ('wiki_visa_route_dependents','dependent_work_rights'),
    ('wiki_visa_route_dependents','dependent_study_rights'),
    ('wiki_visa_route_dependents','dependent_fee'),
    ('wiki_visa_route_dependents','dependents_note'),
    ('wiki_visa_route_documents','documents_list'),
    ('wiki_visa_route_documents','certified_translation'),
    ('wiki_visa_route_documents','attestation_note'),
    ('wiki_visa_route_documents','photo_requirements'),
    ('wiki_visa_route_documents','documents_note'),
    ('wiki_visa_route_fees_costs','application_fee'),
    ('wiki_visa_route_fees_costs','biometrics_fee'),
    ('wiki_visa_route_fees_costs','health_surcharge'),
    ('wiki_visa_route_fees_costs','priority_fee'),
    ('wiki_visa_route_fees_costs','agent_fees'),
    ('wiki_visa_route_fees_costs','total_estimate'),
    ('wiki_visa_route_fees_costs','fees_note'),
    ('wiki_visa_route_other','local_tips'),
    ('wiki_visa_route_other','useful_links'),
    ('wiki_visa_route_overview','validity_duration'),
    ('wiki_visa_route_overview','max_stay'),
    ('wiki_visa_route_processing_time','standard_time'),
    ('wiki_visa_route_processing_time','priority_time'),
    ('wiki_visa_route_processing_time','current_backlog'),
    ('wiki_visa_route_processing_time','factors'),
    ('wiki_visa_route_processing_time','track_url'),
    ('wiki_visa_route_processing_time','processing_note'),
    ('wiki_visa_route_refusals_appeals','common_refusal_reasons'),
    ('wiki_visa_route_refusals_appeals','appeal_possible'),
    ('wiki_visa_route_refusals_appeals','appeal_process'),
    ('wiki_visa_route_refusals_appeals','reapply_wait'),
    ('wiki_visa_route_refusals_appeals','refusal_note'),
    ('wiki_visa_route_renewal_extension','extendable'),
    ('wiki_visa_route_renewal_extension','max_duration'),
    ('wiki_visa_route_renewal_extension','renewal_fee'),
    ('wiki_visa_route_renewal_extension','renewal_process'),
    ('wiki_visa_route_renewal_extension','counts_toward_pr'),
    ('wiki_visa_route_renewal_extension','path_to_citizenship'),
    ('wiki_visa_route_requirements','min_education'),
    ('wiki_visa_route_requirements','language_requirement'),
    ('wiki_visa_route_requirements','work_experience'),
    ('wiki_visa_route_requirements','sponsor_required'),
    ('wiki_visa_route_requirements','proof_of_funds'),
    ('wiki_visa_route_requirements','health_insurance_required'),
    ('wiki_visa_route_requirements','medical_exam_required'),
    ('wiki_visa_route_requirements','police_clearance_required'),
    ('wiki_visa_route_requirements','age_limit'),
    ('wiki_visa_route_requirements','annual_cap'),
    ('wiki_visa_route_requirements','points_threshold'),
    ('wiki_visa_route_requirements','requirements_note'),
    ('wiki_visa_route_rights_restrictions','work_allowed'),
    ('wiki_visa_route_rights_restrictions','work_hours_limit'),
    ('wiki_visa_route_rights_restrictions','study_allowed'),
    ('wiki_visa_route_rights_restrictions','self_employment'),
    ('wiki_visa_route_rights_restrictions','public_funds_access'),
    ('wiki_visa_route_rights_restrictions','multiple_entry'),
    ('wiki_visa_route_rights_restrictions','rights_note'),
    ('wiki_work_visa_application_process','apply_method'),
    ('wiki_work_visa_application_process','steps_summary'),
    ('wiki_work_visa_application_process','biometrics_required'),
    ('wiki_work_visa_application_process','interview_required'),
    ('wiki_work_visa_application_process','apply_url'),
    ('wiki_work_visa_application_process','process_note'),
    ('wiki_work_visa_dependents','dependents_allowed'),
    ('wiki_work_visa_dependents','eligible_dependents'),
    ('wiki_work_visa_dependents','dependent_work_rights'),
    ('wiki_work_visa_dependents','dependent_study_rights'),
    ('wiki_work_visa_dependents','dependent_fee'),
    ('wiki_work_visa_dependents','dependents_note'),
    ('wiki_work_visa_documents','documents_list'),
    ('wiki_work_visa_documents','certified_translation'),
    ('wiki_work_visa_documents','attestation_note'),
    ('wiki_work_visa_documents','photo_requirements'),
    ('wiki_work_visa_documents','documents_note'),
    ('wiki_work_visa_fees_costs','application_fee'),
    ('wiki_work_visa_fees_costs','biometrics_fee'),
    ('wiki_work_visa_fees_costs','health_surcharge'),
    ('wiki_work_visa_fees_costs','priority_fee'),
    ('wiki_work_visa_fees_costs','agent_fees'),
    ('wiki_work_visa_fees_costs','total_estimate'),
    ('wiki_work_visa_fees_costs','fees_note'),
    ('wiki_work_visa_other','local_tips'),
    ('wiki_work_visa_other','useful_links'),
    ('wiki_work_visa_overview','validity_duration'),
    ('wiki_work_visa_overview','max_stay'),
    ('wiki_work_visa_processing_time','standard_time'),
    ('wiki_work_visa_processing_time','priority_time'),
    ('wiki_work_visa_processing_time','current_backlog'),
    ('wiki_work_visa_processing_time','factors'),
    ('wiki_work_visa_processing_time','track_url'),
    ('wiki_work_visa_processing_time','processing_note'),
    ('wiki_work_visa_refusals_appeals','common_refusal_reasons'),
    ('wiki_work_visa_refusals_appeals','appeal_possible'),
    ('wiki_work_visa_refusals_appeals','appeal_process'),
    ('wiki_work_visa_refusals_appeals','reapply_wait'),
    ('wiki_work_visa_refusals_appeals','refusal_note'),
    ('wiki_work_visa_renewal_extension','extendable'),
    ('wiki_work_visa_renewal_extension','max_duration'),
    ('wiki_work_visa_renewal_extension','renewal_fee'),
    ('wiki_work_visa_renewal_extension','renewal_process'),
    ('wiki_work_visa_renewal_extension','counts_toward_pr'),
    ('wiki_work_visa_renewal_extension','path_to_citizenship'),
    ('wiki_work_visa_requirements','min_education'),
    ('wiki_work_visa_requirements','language_requirement'),
    ('wiki_work_visa_requirements','work_experience'),
    ('wiki_work_visa_requirements','sponsor_required'),
    ('wiki_work_visa_requirements','proof_of_funds'),
    ('wiki_work_visa_requirements','health_insurance_required'),
    ('wiki_work_visa_requirements','medical_exam_required'),
    ('wiki_work_visa_requirements','police_clearance_required'),
    ('wiki_work_visa_requirements','age_limit'),
    ('wiki_work_visa_requirements','annual_cap'),
    ('wiki_work_visa_requirements','points_threshold'),
    ('wiki_work_visa_requirements','requirements_note'),
    ('wiki_work_visa_requirements','salary_threshold'),
    ('wiki_work_visa_requirements','eligible_occupations'),
    ('wiki_work_visa_rights_restrictions','work_allowed'),
    ('wiki_work_visa_rights_restrictions','work_hours_limit'),
    ('wiki_work_visa_rights_restrictions','study_allowed'),
    ('wiki_work_visa_rights_restrictions','self_employment'),
    ('wiki_work_visa_rights_restrictions','public_funds_access'),
    ('wiki_work_visa_rights_restrictions','multiple_entry'),
    ('wiki_work_visa_rights_restrictions','rights_note')
) AS m(gs, k) ON g.slug = m.gs AND f.key = m.k
CROSS JOIN user_groups ug
ON CONFLICT (field_id, user_group_id) DO NOTHING;

COMMIT;
