-- Universal Custom Field Architecture (Section 06).
-- One field definition system underlies everything on the platform:
-- Space Types, Content Types, User Profiles, Organization Profiles,
-- Forms, Tags, and Tickets all pull from custom_field_groups + custom_fields.
-- Every field is RBAC-scoped per UserGroup (visibility, editability, filterable).
-- Seven system field groups, two system forms, and the Japa Status field are seeded.

-- Custom Field Groups (e.g. "User Profile Fields", "Blog Post Fields")
CREATE TABLE custom_field_groups (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name         text NOT NULL,
    slug         text NOT NULL,
    description  text,
    is_system    boolean NOT NULL DEFAULT false,
    created_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_custom_field_groups_tenant ON custom_field_groups(tenant_id);

-- Custom Fields (the individual field definitions, namespaced by group)
CREATE TABLE custom_fields (
    id                uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id          uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE CASCADE,
    tenant_id         uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    label             text NOT NULL,
    key               text NOT NULL,
    field_type        text NOT NULL,
    options           jsonb,
    embed_platforms   text[],
    placeholder       text,
    default_value     text,
    is_required       boolean NOT NULL DEFAULT false,
    is_system         boolean NOT NULL DEFAULT false,
    show_in_frontend  boolean NOT NULL DEFAULT true,
    filterable        boolean NOT NULL DEFAULT false,
    sortable          boolean NOT NULL DEFAULT false,
    searchable        boolean NOT NULL DEFAULT false,
    position          int NOT NULL DEFAULT 0,
    created_at        timestamptz NOT NULL DEFAULT now(),
    UNIQUE(group_id, key)
);

CREATE INDEX idx_custom_fields_tenant ON custom_fields(tenant_id);
CREATE INDEX idx_custom_fields_group ON custom_fields(group_id);

-- Field Group Assignments (attach a group to any entity class)
CREATE TABLE custom_field_group_assignments (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    group_id       uuid NOT NULL REFERENCES custom_field_groups(id) ON DELETE CASCADE,
    entity_class   text NOT NULL,
    entity_id      uuid,
    position       int NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    UNIQUE(group_id, entity_class, entity_id)
);

CREATE INDEX idx_cfga_entity ON custom_field_group_assignments(entity_class, entity_id);

-- Custom Field Permissions (RBAC per field per UserGroup)
CREATE TABLE custom_field_permissions (
    field_id       uuid NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
    user_group_id  uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    can_view       boolean NOT NULL DEFAULT true,
    can_edit       boolean NOT NULL DEFAULT false,
    PRIMARY KEY (field_id, user_group_id)
);

-- Custom Field Values (one polymorphic table for all field values)
CREATE TABLE custom_field_values (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    field_id     uuid NOT NULL REFERENCES custom_fields(id) ON DELETE CASCADE,
    entity_type  text NOT NULL,
    entity_id    uuid NOT NULL,
    value_text   text,
    value_number numeric,
    value_bool   boolean,
    value_date   timestamptz,
    value_json   jsonb,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE(field_id, entity_type, entity_id)
);

CREATE INDEX idx_cfv_entity ON custom_field_values(entity_type, entity_id);
CREATE INDEX idx_cfv_filterable ON custom_field_values(field_id, value_text) WHERE value_text IS NOT NULL;

-- Forms (use the same custom_fields infrastructure via group assignments)
CREATE TABLE forms (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id       uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    name            text NOT NULL,
    slug            text NOT NULL,
    description     text,
    is_multi_step   boolean NOT NULL DEFAULT false,
    submit_label    text NOT NULL DEFAULT 'Submit',
    success_message text,
    status          text NOT NULL DEFAULT 'active',
    is_system       boolean NOT NULL DEFAULT false,
    created_at      timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX idx_forms_tenant ON forms(tenant_id);

-- Form Steps (for multi-step forms)
CREATE TABLE form_steps (
    id       uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id  uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    title    text NOT NULL,
    position int NOT NULL,
    UNIQUE(form_id, position)
);

CREATE INDEX idx_form_steps_form ON form_steps(form_id);

-- Form Submissions (field_values is a key -> value snapshot at submission time)
CREATE TABLE form_submissions (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id      uuid NOT NULL REFERENCES forms(id),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      uuid REFERENCES users(id),
    ip_address   inet,
    field_values jsonb NOT NULL DEFAULT '{}',
    status       text NOT NULL DEFAULT 'new',
    created_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_form_submissions_form ON form_submissions(form_id);
CREATE INDEX idx_form_submissions_tenant ON form_submissions(tenant_id);

-- Form Submission Triggers (workflow triggers on form submission)
CREATE TABLE form_submission_triggers (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    form_id      uuid NOT NULL REFERENCES forms(id) ON DELETE CASCADE,
    trigger_type text NOT NULL,
    config       jsonb NOT NULL DEFAULT '{}',
    position     int NOT NULL DEFAULT 0
);

CREATE INDEX idx_fst_form ON form_submission_triggers(form_id);

-- Seed the seven system default field groups for the default tenant.
-- UUIDs continue the zeros-pattern: 010/020/030 are RBAC groups, so custom
-- fields start at 040.
INSERT INTO custom_field_groups (id, tenant_id, name, slug, is_system) VALUES
    ('00000000-0000-0000-0000-000000000040', '00000000-0000-0000-0000-000000000001', 'User Profile Fields', 'user_profile_fields', true),
    ('00000000-0000-0000-0000-000000000041', '00000000-0000-0000-0000-000000000001', 'Blog Post Fields', 'blog_post_fields', true),
    ('00000000-0000-0000-0000-000000000042', '00000000-0000-0000-0000-000000000001', 'Forum Fields', 'forum_fields', true),
    ('00000000-0000-0000-0000-000000000043', '00000000-0000-0000-0000-000000000001', 'Space Fields', 'space_fields', true),
    ('00000000-0000-0000-0000-000000000044', '00000000-0000-0000-0000-000000000001', 'University Fields', 'university_fields', true),
    ('00000000-0000-0000-0000-000000000045', '00000000-0000-0000-0000-000000000001', 'Organization Fields', 'organization_fields', true),
    ('00000000-0000-0000-0000-000000000046', '00000000-0000-0000-0000-000000000001', 'Tag Fields', 'tag_fields', true);

-- Seed the two system forms for the default tenant.
INSERT INTO forms (id, tenant_id, name, slug, is_system) VALUES
    ('00000000-0000-0000-0000-000000000047', '00000000-0000-0000-0000-000000000001', 'Feedback', 'feedback', true),
    ('00000000-0000-0000-0000-000000000048', '00000000-0000-0000-0000-000000000001', 'Report', 'report', true);

-- Seed the Japa Status select field in the user_profile_fields group.
-- Admin can add/edit/remove options from the Admin Panel without a code deploy.
INSERT INTO custom_fields (id, group_id, tenant_id, label, key, field_type, options, is_system, is_required, position)
VALUES (
    '00000000-0000-0000-0000-000000000049',
    '00000000-0000-0000-0000-000000000040',
    '00000000-0000-0000-0000-000000000001',
    'Japa Status',
    'japa_status',
    'select',
    '[
        {"value": "newly_japa", "label": "Newly Japa (< 1 year)"},
        {"value": "settled", "label": "Settled Abroad"},
        {"value": "about_to_japa", "label": "About to Japa"},
        {"value": "planning", "label": "Planning to Relocate"},
        {"value": "looking_jobs", "label": "Looking for Jobs Abroad"},
        {"value": "looking_study", "label": "Looking to Study Abroad"},
        {"value": "returning", "label": "Considering Returning Home"},
        {"value": "remote_worker", "label": "Remote Worker (Nigeria-based)"}
    ]'::jsonb,
    true,
    false,
    0
);

-- Attach the user_profile_fields group to the user_profile entity class.
-- entity_id NULL means the group applies to all user_profile entities.
INSERT INTO custom_field_group_assignments (group_id, entity_class, entity_id, position)
VALUES ('00000000-0000-0000-0000-000000000040', 'user_profile', NULL, 0);
