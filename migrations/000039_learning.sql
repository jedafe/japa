-- Section 36: Learning Platform Backend
-- Tracks, modules, lessons, progress tracking, and certificates.

CREATE TABLE IF NOT EXISTS learning_tracks (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    difficulty text NOT NULL DEFAULT 'beginner',
    cover_media_id uuid,
    is_published boolean NOT NULL DEFAULT false,
    sort_order int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS learning_modules (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    track_id uuid NOT NULL REFERENCES learning_tracks(id) ON DELETE CASCADE,
    title text NOT NULL,
    slug text NOT NULL,
    description text,
    sort_order int NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS learning_lessons (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    module_id uuid NOT NULL REFERENCES learning_modules(id) ON DELETE CASCADE,
    title text NOT NULL,
    slug text NOT NULL,
    lesson_type text NOT NULL DEFAULT 'article',
    content text,
    media_id uuid,
    duration_minutes int,
    sort_order int NOT NULL DEFAULT 0,
    is_published boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(tenant_id, slug)
);

CREATE TABLE IF NOT EXISTS lesson_progress (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    lesson_id uuid NOT NULL REFERENCES learning_lessons(id) ON DELETE CASCADE,
    status text NOT NULL DEFAULT 'not_started',
    completed_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE(user_id, lesson_id)
);

CREATE TABLE IF NOT EXISTS learning_certificates (
    id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id uuid NOT NULL REFERENCES tenants(id),
    user_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id uuid NOT NULL REFERENCES learning_tracks(id),
    certificate_number text NOT NULL UNIQUE,
    media_id uuid,
    issued_at timestamptz NOT NULL DEFAULT now()
);
