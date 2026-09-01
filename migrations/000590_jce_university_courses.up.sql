-- Create university course catalog and offerings tables for the JCE course resolver.
-- Ref: japaconnect-engineering/sections/39-space-types-university.md
BEGIN;

CREATE TABLE IF NOT EXISTS university_course_catalog (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    title               text NOT NULL,
    slug                text NOT NULL,
    description         text,
    degree_type         text,
    department          text,
    field_of_study      text,
    language            text NOT NULL DEFAULT 'English',
    default_duration_years numeric,
    is_published        boolean NOT NULL DEFAULT true,
    deleted_at          timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_slug ON university_course_catalog(tenant_id, slug) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_university_course_catalog_tenant_title ON university_course_catalog(tenant_id, title) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS university_course_offerings (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id           uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    course_id           uuid NOT NULL REFERENCES university_course_catalog(id) ON DELETE CASCADE,
    university_id       uuid NOT NULL REFERENCES spaces(id) ON DELETE CASCADE,
    slug                text NOT NULL,
    tuition             numeric,
    currency            text,
    duration_years      numeric,
    study_mode          text,
    intake_months       text[],
    entry_requirements  text,
    accreditation       text,
    is_published        boolean NOT NULL DEFAULT true,
    deleted_at          timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, course_id, university_id),
    UNIQUE(tenant_id, slug)
);

CREATE INDEX IF NOT EXISTS idx_university_course_offerings_university ON university_course_offerings(tenant_id, university_id) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_university_course_offerings_course ON university_course_offerings(tenant_id, course_id) WHERE deleted_at IS NULL;

COMMIT;
