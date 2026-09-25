-- Learning Enrollments
CREATE TABLE IF NOT EXISTS enrollments (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id    uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    track_id     uuid NOT NULL REFERENCES learning_tracks(id) ON DELETE CASCADE,
    access_type  text NOT NULL DEFAULT 'free',
    progress_pct numeric NOT NULL DEFAULT 0,
    enrolled_at  timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE(user_id, track_id)
);

CREATE INDEX IF NOT EXISTS idx_enrollments_tenant_user ON enrollments(tenant_id, user_id);
CREATE INDEX IF NOT EXISTS idx_enrollments_tenant_track ON enrollments(tenant_id, track_id);

-- Lesson Quizzes
CREATE TABLE IF NOT EXISTS lesson_quizzes (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
    lesson_id   uuid NOT NULL REFERENCES learning_lessons(id) ON DELETE CASCADE,
    question    text NOT NULL,
    options     text NOT NULL,
    answer      text NOT NULL,
    explanation text,
    position    int NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_lesson_quizzes_tenant_lesson ON lesson_quizzes(tenant_id, lesson_id);
