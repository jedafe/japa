# Section 36 — Learning Platform (learn.japaconnect.com)

**Status:** ✅ Locked

## Architecture

Served at `learn.japaconnect.com` as a fourth SvelteKit app (SSR, adapter-node) sharing the same Go API backend. Access gated by the existing payment system (Section 17): `course_access` purpose type, dual-access model — Pro membership subscribers get all courses; à la carte buyers purchase individual course access.

## Schema

```sql
CREATE TABLE learning_tracks (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  title           text NOT NULL,
  slug            text NOT NULL UNIQUE,
  description     text,
  thumbnail_id    uuid REFERENCES media(id),
  level           text NOT NULL DEFAULT 'beginner',  -- 'beginner'|'intermediate'|'advanced'
  estimated_hours numeric,
  price_kobo      bigint,         -- NULL = Pro-only
  status          text NOT NULL DEFAULT 'draft',
  published_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE learning_modules (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  track_id    uuid NOT NULL REFERENCES learning_tracks(id) ON DELETE CASCADE,
  title       text NOT NULL,
  description text,
  position    int NOT NULL,
  UNIQUE(track_id, position)
);

CREATE TABLE learning_lessons (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  module_id       uuid NOT NULL REFERENCES learning_modules(id) ON DELETE CASCADE,
  title           text NOT NULL,
  slug            text NOT NULL,
  content         jsonb NOT NULL DEFAULT '{}',   -- TipTap JSON (same editor as platform)
  lesson_type     text NOT NULL DEFAULT 'article',  -- 'article'|'video'|'quiz'|'checklist'
  video_media_id  uuid REFERENCES media(id),
  duration_mins   int,
  is_preview      boolean NOT NULL DEFAULT false,   -- accessible without purchase
  position        int NOT NULL,
  UNIQUE(module_id, position)
);

CREATE TABLE lesson_quizzes (
  id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  lesson_id   uuid NOT NULL REFERENCES learning_lessons(id) ON DELETE CASCADE,
  question    text NOT NULL,
  options     jsonb NOT NULL,  -- [{text, is_correct}]
  explanation text,
  position    int NOT NULL
);

CREATE TABLE lesson_progress (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  lesson_id       uuid NOT NULL REFERENCES learning_lessons(id) ON DELETE CASCADE,
  status          text NOT NULL DEFAULT 'started',  -- 'started'|'completed'
  quiz_passed     boolean,
  completed_at    timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, lesson_id)
);

CREATE TABLE track_certificates (
  id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  track_id        uuid NOT NULL REFERENCES learning_tracks(id),
  certificate_url text NOT NULL,
  issued_at       timestamptz NOT NULL DEFAULT now(),
  UNIQUE(user_id, track_id)
);

-- Access control: payments table with purpose_type='course_access', purpose_id=track_id
-- OR user has active Pro subscription (checked via subscriptions table)
```

## Access Check

```go
func CanAccessTrack(ctx context.Context, userID, trackID uuid.UUID) (bool, error) {
  // 1. Pro subscriber?
  if hasPro, _ := hasActiveSubscription(ctx, userID); hasPro {
    return true, nil
  }
  // 2. Purchased this track?
  return hasPaymentFor(ctx, userID, "course_access", trackID)
}
```

## Certificate Generation

On 100% track completion, the `issue_certificate` Asynq task (low tier) generates a PDF certificate using a Go HTML template + headless Chrome (or `go-wkhtmltopdf`), stores to MinIO, and inserts a `track_certificates` row. Notified via the Notification System (Section 35).

## Asynq Tasks

- `learning:issue_certificate` (low tier)
- `learning:update_track_completion` (default tier) — recalculates completion % on lesson completion

## See Also

Section 17 (course_access payment purpose), Section 28 (Credit as course payment method). ADR-036.
