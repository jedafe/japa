-- ============================================================
-- 000225: Guide Content Type System
-- Transforms the generic "guide" content type into an evergreen
-- guide system with categories (Visa, Travel, Scholarship, etc.).
-- Each category has distinct custom field groups, tightly integrated
-- with the existing custom_field architecture and RBAC.
-- ============================================================

-- 1. Add guide_category column to contents
ALTER TABLE contents ADD COLUMN IF NOT EXISTS guide_category text;
CREATE INDEX IF NOT EXISTS idx_contents_guide_category
    ON contents(tenant_id, guide_category)
    WHERE guide_category IS NOT NULL;

-- 2. Seed guide categories as a reference table (reuses custom_field_entity_types
--    infrastructure — guide categories are entity types with value='guide_category:{slug}')
INSERT INTO custom_field_entity_types (tenant_id, value, label, is_system, sort_order)
SELECT '00000000-0000-0000-0000-000000000001', v.value, v.label, true, v.sort_order
FROM (VALUES
    ('guide_category:visa',              'Visa Guide',              1),
    ('guide_category:travel_routes',     'Travel Routes',           2),
    ('guide_category:scholarship',       'Scholarship Guide',       3),
    ('guide_category:grants',            'Grants Application',      4),
    ('guide_category:admission',         'Admission Process',       5),
    ('guide_category:accommodation',     'Accommodation Guide',     6),
    ('guide_category:banking',           'Banking & Finance',       7),
    ('guide_category:work_career',       'Work & Career',           8),
    ('guide_category:health_insurance',  'Health & Insurance',      9),
    ('guide_category:culture_lifestyle', 'Culture & Lifestyle',    10)
) AS v(value, label, sort_order)
WHERE NOT EXISTS (
    SELECT 1 FROM custom_field_entity_types
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND value = v.value
);

-- ============================================================
-- 3. RBAC Permissions for Guide management
-- ============================================================
INSERT INTO permissions (key, description) VALUES
    ('guide.create',      'Create evergreen guides'),
    ('guide.edit_own',    'Edit own guides'),
    ('guide.edit_any',    'Edit any guide (admin)'),
    ('guide.delete_own',  'Delete own guides'),
    ('guide.delete_any',  'Delete any guide (admin)'),
    ('guide.publish',     'Publish guides'),
    ('guide.feature',     'Feature/unfeature guides on the guides hub')
ON CONFLICT (key) DO NOTHING;

-- Grant guide permissions to super_admin (gets ALL permissions already via
-- the 000099 seed, but the reconciliation migration 000109 pattern ensures
-- new permissions are explicitly granted).
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'super_admin'
  AND p.key IN ('guide.create','guide.edit_own','guide.edit_any','guide.delete_own','guide.delete_any','guide.publish','guide.feature')
ON CONFLICT DO NOTHING;

-- Grant guide create/edit_own/publish/delete_own to members
INSERT INTO role_permissions (role_id, permission_id)
SELECT ur.id, p.id
FROM user_roles ur, permissions p
WHERE ur.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND ur.slug = 'member'
  AND p.key IN ('guide.create','guide.edit_own','guide.delete_own','guide.publish')
ON CONFLICT DO NOTHING;

-- ============================================================
-- 4. Seed Custom Field Groups for each Guide Category
--    Each category gets its own field group with category-specific fields.
--    The group is assigned via custom_field_group_assignments with
--    entity_class = 'content_type:guide:{category_slug}' so the backend
--    can filter field groups by guide category.
-- ============================================================

-- Helper: we use a DO block to create groups + fields + assignments in one
-- transaction with predictable IDs via slugs.
DO $$
DECLARE
    tenant_uuid uuid := '00000000-0000-0000-0000-000000000001';
    g_id uuid;
BEGIN
    -- ============================================================
    -- Visa Guide
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Visa Guide Fields', 'guide_visa_fields', 'Custom fields for visa guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_visa_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Destination Country', 'destination_country', 'select',
         '["Nigeria","Ghana","Kenya","South Africa","United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 1),
        (g_id, tenant_uuid, 'Visa Type', 'visa_type', 'select',
         '["Student Visa","Work Visa","Tourist Visa","Business Visa","Family Visa","Transit Visa","Permanent Residency","Other"]'::jsonb, true, true, 2),
        (g_id, tenant_uuid, 'Processing Time', 'processing_time', 'text', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Visa Fee (USD)', 'visa_fee_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Application Link', 'application_link', 'url', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Required Documents', 'required_documents', 'textarea', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Biometrics Required', 'biometrics_required', 'boolean', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Interview Required', 'interview_required', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Medical Exam Required', 'medical_exam_required', 'boolean', NULL, false, true, 9),
        (g_id, tenant_uuid, 'Validity Period', 'validity_period', 'text', NULL, false, true, 10)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:visa', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Travel Routes
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Travel Routes Fields', 'guide_travel_routes_fields', 'Custom fields for travel route content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_travel_routes_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Origin City', 'origin_city', 'text', NULL, true, true, 1),
        (g_id, tenant_uuid, 'Destination City', 'destination_city', 'text', NULL, true, true, 2),
        (g_id, tenant_uuid, 'Transit Hubs', 'transit_hubs', 'text', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Estimated Cost (USD)', 'estimated_cost_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Travel Duration', 'travel_duration', 'text', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Recommended Airlines', 'recommended_airlines', 'text', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Best Travel Season', 'best_travel_season', 'select',
         '["Spring (Mar-May)","Summer (Jun-Aug)","Autumn (Sep-Nov)","Winter (Dec-Feb)","Year-round"]'::jsonb, false, true, 7),
        (g_id, tenant_uuid, 'Visa on Arrival', 'visa_on_arrival', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Layover Tips', 'layover_tips', 'textarea', NULL, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:travel_routes', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Scholarship Guide
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Scholarship Guide Fields', 'guide_scholarship_fields', 'Custom fields for scholarship guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_scholarship_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Scholarship Name', 'scholarship_name', 'text', NULL, true, true, 1),
        (g_id, tenant_uuid, 'Provider/Institution', 'provider_institution', 'text', NULL, true, true, 2),
        (g_id, tenant_uuid, 'Eligibility Criteria', 'eligibility_criteria', 'textarea', NULL, true, true, 3),
        (g_id, tenant_uuid, 'Award Amount (USD)', 'award_amount_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Application Deadline', 'application_deadline', 'date', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Application Link', 'application_link', 'url', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Level of Study', 'level_of_study', 'select',
         '["Undergraduate","Masters","PhD","Postdoc","Any"]'::jsonb, false, true, 7),
        (g_id, tenant_uuid, 'Renewable', 'is_renewable', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Coverage', 'coverage', 'select',
         '["Full Tuition","Partial Tuition","Tuition + Stipend","Stipend Only","Research Grant"]'::jsonb, false, true, 9),
        (g_id, tenant_uuid, 'Countries Eligible', 'countries_eligible', 'text', NULL, false, true, 10)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:scholarship', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Grants Application
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Grants Application Fields', 'guide_grants_fields', 'Custom fields for grants application guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_grants_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Grant Name', 'grant_name', 'text', NULL, true, true, 1),
        (g_id, tenant_uuid, 'Granting Organization', 'granting_organization', 'text', NULL, true, true, 2),
        (g_id, tenant_uuid, 'Grant Amount (USD)', 'grant_amount_usd', 'number', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Application Deadline', 'application_deadline', 'date', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Application Link', 'application_link', 'url', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Eligibility', 'eligibility', 'textarea', NULL, true, true, 6),
        (g_id, tenant_uuid, 'Grant Type', 'grant_type', 'select',
         '["Research Grant","Business Grant","Education Grant","Community Grant","Emergency Grant","Other"]'::jsonb, false, true, 7),
        (g_id, tenant_uuid, 'Reporting Required', 'reporting_required', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Match Funding Required', 'match_funding_required', 'boolean', NULL, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:grants', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Admission Process
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Admission Process Fields', 'guide_admission_fields', 'Custom fields for admission process guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_admission_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Institution Name', 'institution_name', 'text', NULL, true, true, 1),
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["Nigeria","Ghana","Kenya","South Africa","United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 2),
        (g_id, tenant_uuid, 'Degree Level', 'degree_level', 'select',
         '["Undergraduate","Masters","PhD","Diploma","Certificate"]'::jsonb, true, true, 3),
        (g_id, tenant_uuid, 'Application Deadline', 'application_deadline', 'date', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Application Fee (USD)', 'application_fee_usd', 'number', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Required Exams', 'required_exams', 'text',
         NULL, false, true, 6),
        (g_id, tenant_uuid, 'Minimum GPA', 'minimum_gpa', 'text', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Language Requirements', 'language_requirements', 'text', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Application Link', 'application_link', 'url', NULL, false, true, 9),
        (g_id, tenant_uuid, 'Acceptance Rate', 'acceptance_rate', 'text', NULL, false, true, 10)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:admission', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Accommodation Guide
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Accommodation Guide Fields', 'guide_accommodation_fields', 'Custom fields for accommodation guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_accommodation_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'City', 'city', 'text', NULL, true, true, 1),
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 2),
        (g_id, tenant_uuid, 'Accommodation Type', 'accommodation_type', 'select',
         '["On-Campus","Off-Campus","Homestay","Shared Apartment","Studio","Hostel","Hotel"]'::jsonb, false, true, 3),
        (g_id, tenant_uuid, 'Monthly Cost (USD)', 'monthly_cost_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Deposit Required', 'deposit_required', 'boolean', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Lease Duration', 'lease_duration', 'text', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Furnished', 'is_furnished', 'boolean', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Utilities Included', 'utilities_included', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Booking Link', 'booking_link', 'url', NULL, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:accommodation', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Banking & Finance
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Banking & Finance Fields', 'guide_banking_fields', 'Custom fields for banking and finance guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_banking_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 1),
        (g_id, tenant_uuid, 'Bank Name', 'bank_name', 'text', NULL, true, true, 2),
        (g_id, tenant_uuid, 'Account Type', 'account_type', 'select',
         '["Student Account","Current Account","Savings Account","International Account","Digital Bank"]'::jsonb, false, true, 3),
        (g_id, tenant_uuid, 'Minimum Balance (USD)', 'minimum_balance_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Monthly Fee (USD)', 'monthly_fee_usd', 'number', NULL, false, true, 5),
        (g_id, tenant_uuid, 'International Transfer Fee', 'international_transfer_fee', 'text', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Online Banking', 'online_banking', 'boolean', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Debit Card Included', 'debit_card_included', 'boolean', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Requirements', 'requirements', 'textarea', NULL, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:banking', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Work & Career
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Work & Career Fields', 'guide_work_career_fields', 'Custom fields for work and career guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_work_career_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 1),
        (g_id, tenant_uuid, 'Work Visa Required', 'work_visa_required', 'boolean', NULL, true, true, 2),
        (g_id, tenant_uuid, 'Visa Type', 'visa_type', 'text', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Average Salary (USD)', 'average_salary_usd', 'number', NULL, false, true, 4),
        (g_id, tenant_uuid, 'In-Demand Sectors', 'in_demand_sectors', 'text', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Work Hours Limit', 'work_hours_limit', 'text', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Tax Rate', 'tax_rate', 'text', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Networking Tips', 'networking_tips', 'textarea', NULL, false, true, 8),
        (g_id, tenant_uuid, 'CV Format', 'cv_format', 'select',
         '["European (Europass)","American (Resume)","Canadian","Universal"]'::jsonb, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:work_career', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Health & Insurance
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Health & Insurance Fields', 'guide_health_insurance_fields', 'Custom fields for health and insurance guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_health_insurance_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 1),
        (g_id, tenant_uuid, 'Insurance Type', 'insurance_type', 'select',
         '["Public/Universal","Private","Student Health Insurance","Travel Insurance","Employer-Sponsored"]'::jsonb, true, true, 2),
        (g_id, tenant_uuid, 'Monthly Premium (USD)', 'monthly_premium_usd', 'number', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Coverage Details', 'coverage_details', 'textarea', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Dental Covered', 'dental_covered', 'boolean', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Vision Covered', 'vision_covered', 'boolean', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Mental Health Covered', 'mental_health_covered', 'boolean', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Pre-existing Conditions', 'pre_existing_conditions', 'text', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Registration Link', 'registration_link', 'url', NULL, false, true, 9)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:health_insurance', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

    -- ============================================================
    -- Culture & Lifestyle
    -- ============================================================
    INSERT INTO custom_field_groups (tenant_id, name, slug, description, is_system)
    VALUES (tenant_uuid, 'Culture & Lifestyle Fields', 'guide_culture_lifestyle_fields', 'Custom fields for culture and lifestyle guide content', true)
    ON CONFLICT (tenant_id, slug) DO NOTHING
    RETURNING id INTO g_id;

    IF g_id IS NULL THEN
        SELECT id INTO g_id FROM custom_field_groups WHERE tenant_id = tenant_uuid AND slug = 'guide_culture_lifestyle_fields';
    END IF;

    INSERT INTO custom_fields (group_id, tenant_id, label, key, field_type, options, is_required, is_system, position) VALUES
        (g_id, tenant_uuid, 'Country', 'country', 'select',
         '["United Kingdom","United States","Canada","Germany","Australia","Other"]'::jsonb, true, true, 1),
        (g_id, tenant_uuid, 'City', 'city', 'text', NULL, false, true, 2),
        (g_id, tenant_uuid, 'Language Spoken', 'language_spoken', 'text', NULL, false, true, 3),
        (g_id, tenant_uuid, 'Currency', 'currency', 'text', NULL, false, true, 4),
        (g_id, tenant_uuid, 'Average Living Cost (USD/month)', 'avg_living_cost_usd', 'number', NULL, false, true, 5),
        (g_id, tenant_uuid, 'Cultural Etiquette', 'cultural_etiquette', 'textarea', NULL, false, true, 6),
        (g_id, tenant_uuid, 'Religious Considerations', 'religious_considerations', 'textarea', NULL, false, true, 7),
        (g_id, tenant_uuid, 'Food & Dining', 'food_dining', 'textarea', NULL, false, true, 8),
        (g_id, tenant_uuid, 'Transportation', 'transportation', 'textarea', NULL, false, true, 9),
        (g_id, tenant_uuid, 'Social Norms', 'social_norms', 'textarea', NULL, false, true, 10)
    ON CONFLICT (group_id, key) DO NOTHING;

    INSERT INTO custom_field_group_assignments (group_id, entity_class, position)
    VALUES (g_id, 'content_type:guide:culture_lifestyle', 0)
    ON CONFLICT (group_id, entity_class, entity_id) DO NOTHING;

END $$;
