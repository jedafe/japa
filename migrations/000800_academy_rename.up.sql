-- ============================================================================
-- 000800: Learn + Japa University -> Japa Academy (product rename)
-- ----------------------------------------------------------------------------
-- Renames the learning/instructor-marketplace schema objects and every seeded
-- identifier (permission nodes, settings keys, AI tool names, menus, ad slots,
-- notification types, i18n keys) to the unified "academy" namespace.
--
-- NOT renamed (real-world university domain — spaces, study-abroad catalog):
--   university_course_catalog, university_course_offerings, universities data,
--   scholarships.university, the university.get AI tool, university:fetch_logo.
-- ============================================================================

-- ── 1. Table renames ─────────────────────────────────────────────────────────
ALTER TABLE public.learning_tracks             RENAME TO academy_tracks;
ALTER TABLE public.learning_modules            RENAME TO academy_modules;
ALTER TABLE public.learning_lessons            RENAME TO academy_lessons;
ALTER TABLE public.learning_certificates       RENAME TO academy_certificates;
ALTER TABLE public.lesson_quizzes              RENAME TO academy_quizzes;
ALTER TABLE public.lesson_progress             RENAME TO academy_lesson_progress;
ALTER TABLE public.enrollments                 RENAME TO academy_enrollments;
ALTER TABLE public.instructor_profiles         RENAME TO academy_instructor_profiles;
ALTER TABLE public.instructor_payouts          RENAME TO academy_instructor_payouts;
ALTER TABLE public.course_access_grants        RENAME TO academy_course_access_grants;
ALTER TABLE public.subscription_plan_tracks    RENAME TO academy_subscription_plan_tracks;
ALTER TABLE public.university_track_categories RENAME TO academy_track_categories;

-- ── 2. Marketplace column renames on academy_tracks ──────────────────────────
ALTER TABLE public.academy_tracks RENAME COLUMN is_published_to_university   TO is_published_to_academy;
ALTER TABLE public.academy_tracks RENAME COLUMN university_submission_status TO academy_submission_status;
ALTER TABLE public.academy_tracks RENAME COLUMN university_submitted_at      TO academy_submitted_at;
ALTER TABLE public.academy_tracks RENAME COLUMN university_reviewed_at       TO academy_reviewed_at;
ALTER TABLE public.academy_tracks RENAME COLUMN university_rejection_reason  TO academy_rejection_reason;
ALTER TABLE public.academy_tracks RENAME COLUMN university_featured          TO academy_featured;

-- ── 3. Index + constraint name renames (dependent objects) ───────────────────
DO $academy$
DECLARE
    r        record;
    new_name text;
    pairs    text[][] := ARRAY[
        ['university_track_categories', 'academy_track_categories'],
        ['learning_certificates',       'academy_certificates'],
        ['learning_tracks',             'academy_tracks'],
        ['learning_modules',            'academy_modules'],
        ['learning_lessons',            'academy_lessons'],
        ['lesson_progress',             'academy_lesson_progress'],
        ['lesson_quizzes',              'academy_quizzes'],
        ['instructor_profiles',         'academy_instructor_profiles'],
        ['instructor_payouts',          'academy_instructor_payouts'],
        ['course_access_grants',        'academy_course_access_grants'],
        ['subscription_plan_tracks',    'academy_subscription_plan_tracks'],
        ['enrollments',                 'academy_enrollments'],
        ['university',                  'academy']
    ];
    i int;
    sel text := 'learning_|lesson_progress|lesson_quizzes|instructor_profiles|instructor_payouts'
              '|course_access_grants|subscription_plan_tracks|university_track_categories'
              '|\menrollments\M|is_published_to_university';
BEGIN
    FOR r IN
        SELECT indexname AS n FROM pg_indexes
        WHERE schemaname = 'public' AND indexname ~ sel
    LOOP
        new_name := r.n;
        FOR i IN 1..array_length(pairs, 1) LOOP
            new_name := replace(new_name, pairs[i][1], pairs[i][2]);
        END LOOP;
        IF new_name <> r.n AND NOT EXISTS (
            SELECT 1 FROM pg_indexes WHERE schemaname = 'public' AND indexname = new_name
        ) THEN
            EXECUTE format('ALTER INDEX public.%I RENAME TO %I', r.n, new_name);
        END IF;
    END LOOP;

    FOR r IN
        SELECT c.conname AS n, cls.relname AS tbl
        FROM pg_constraint c
        JOIN pg_class cls      ON cls.oid = c.conrelid
        JOIN pg_namespace nsp  ON nsp.oid = cls.relnamespace
        WHERE nsp.nspname = 'public' AND c.conname ~ sel
    LOOP
        new_name := r.n;
        FOR i IN 1..array_length(pairs, 1) LOOP
            new_name := replace(new_name, pairs[i][1], pairs[i][2]);
        END LOOP;
        IF new_name <> r.n AND NOT EXISTS (
            SELECT 1 FROM pg_constraint c2
            JOIN pg_class t2 ON t2.oid = c2.conrelid
            WHERE t2.relname = r.tbl AND c2.conname = new_name
        ) THEN
            EXECUTE format('ALTER TABLE public.%I RENAME CONSTRAINT %I TO %I',
                           r.tbl, r.n, new_name);
        END IF;
    END LOOP;
END
$academy$;

-- ── 4. Polymorphic entity_type discriminator: learning_track → academy_track ─
UPDATE public.bookmarks            SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.reactions            SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.reviews              SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.review_aggregates    SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.content_embeddings   SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.follows              SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.platform_events      SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.slug_history         SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.trending_snapshots   SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.intel_records        SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.moderation_appeals   SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';
UPDATE public.custom_field_values  SET entity_type = 'academy_track' WHERE entity_type = 'learning_track';

-- ── 5. RBAC permission nodes ─────────────────────────────────────────────────
UPDATE public.permissions SET key = 'academy' || substring(key from 9)
    WHERE key LIKE 'learning.%';
UPDATE public.permissions SET key = 'academy' || substring(key from 11)
    WHERE key LIKE 'university.%';
UPDATE public.permissions
    SET description = replace(replace(description, 'Japa University', 'Japa Academy'),
                              'learning ', 'academy ')
    WHERE key LIKE 'academy.%';

-- ── 6. system_settings keys ──────────────────────────────────────────────────
UPDATE public.system_settings SET key = 'academy' || substring(key from 11)
    WHERE key LIKE 'university.%';
UPDATE public.system_settings SET value = '"academy"'::jsonb
    WHERE key = 'academy.subdomain';
UPDATE public.system_settings
    SET value = replace(value::text, 'japa-university', 'japa-academy')::jsonb
    WHERE value::text LIKE '%japa-university%';
UPDATE public.system_settings
    SET value = replace(value::text, 'Japa University', 'Japa Academy')::jsonb
    WHERE value::text LIKE '%Japa University%';
UPDATE public.system_settings
    SET value = replace(value::text, '"/university"', '"/academy"')::jsonb
    WHERE value::text LIKE '%"/university"%';

-- ── 7. AI tool registry (learn.* → academy.*; university.get is the ──────────
--     real-university tool and stays) ─────────────────────────────────────────
UPDATE public.ai_tools SET name = 'academy' || substring(name from 6)
    WHERE name LIKE 'learn.%';
UPDATE public.ai_tools SET category = 'agent_academy' WHERE category = 'agent_learning';
UPDATE public.ai_tools SET required_permission = 'academy' || substring(required_permission from 9)
    WHERE required_permission LIKE 'learning.%';
UPDATE public.ai_tools SET config = replace(replace(config::text,
        '"learn.', '"academy.'),
        '"handler": "Learn', '"handler": "Academy')::jsonb
    WHERE config::text ~ '"learn\.|"handler": "Learn';

-- ── 8. Menus ─────────────────────────────────────────────────────────────────
UPDATE public.menus SET slug = 'academy' || substring(slug from 6)
    WHERE slug LIKE 'learn\_%' ESCAPE '\';
UPDATE public.menus SET name = replace(name, 'Learn ', 'Academy ')
    WHERE slug LIKE 'academy\_%' ESCAPE '\';

-- ── 9. Ad slot types ─────────────────────────────────────────────────────────
UPDATE public.ad_slot_types SET slot_name = 'academy' || substring(slot_name from 6)
    WHERE slot_name LIKE 'learn\_%' ESCAPE '\';
UPDATE public.ad_slot_types
    SET description = replace(description, 'learning', 'academy')
    WHERE slot_name LIKE 'academy\_%' ESCAPE '\' AND description LIKE '%learning%';

-- ── 10. Notification types ───────────────────────────────────────────────────
-- notification_types.key is the PK and both notifications.notification_type
-- and notification_preferences.notification_type FK to it WITHOUT
-- ON UPDATE CASCADE — a bare key UPDATE violates the FK on any DB with
-- notification/preference rows. Clone the rows under the new keys, repoint
-- dependents, then delete the old keys.
INSERT INTO public.notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
SELECT 'academy_instructor_approved', label, description, enabled, default_in_app, default_email, default_push, category
FROM public.notification_types
WHERE key = 'university_instructor_approved'
ON CONFLICT (key) DO NOTHING;
INSERT INTO public.notification_types (key, label, description, enabled, default_in_app, default_email, default_push, category)
SELECT 'academy_course_approved', label, description, enabled, default_in_app, default_email, default_push, category
FROM public.notification_types
WHERE key = 'university_course_approved'
ON CONFLICT (key) DO NOTHING;
UPDATE public.notifications SET notification_type = 'academy_instructor_approved'
    WHERE notification_type = 'university_instructor_approved';
UPDATE public.notifications SET notification_type = 'academy_course_approved'
    WHERE notification_type = 'university_course_approved';
UPDATE public.notification_preferences SET notification_type = 'academy_instructor_approved'
    WHERE notification_type = 'university_instructor_approved';
UPDATE public.notification_preferences SET notification_type = 'academy_course_approved'
    WHERE notification_type = 'university_course_approved';
DELETE FROM public.notification_types
    WHERE key IN ('university_instructor_approved', 'university_course_approved');
UPDATE public.notification_types
    SET description = replace(description, 'the marketplace', 'the Japa Academy catalog')
    WHERE key = 'academy_course_approved';

-- ── 11. DB-backed UI translations (learn.* + learning.* → academy.*) ─────────
-- university.* translation keys are real-university UI strings — untouched.
-- learn.* and learning.* both collapse to academy.*.
-- learn.* rows are renamed first so their translations win on collision;
-- colliding learning.* or leftover learn.* duplicates are dropped first to
-- satisfy the translations (locale, key) unique index.
DELETE FROM public.translations t
    WHERE t.key LIKE 'learn.%' AND EXISTS (
        SELECT 1 FROM public.translations t2
        WHERE t2.locale = t.locale AND t2.key = 'academy' || substring(t.key from 6));
UPDATE public.translations SET key = 'academy' || substring(key from 6)
    WHERE key LIKE 'learn.%';
DELETE FROM public.translations t
    WHERE t.key LIKE 'learning.%' AND EXISTS (
        SELECT 1 FROM public.translations t2
        WHERE t2.locale = t.locale AND t2.key = 'academy' || substring(t.key from 9));
UPDATE public.translations SET key = 'academy' || substring(key from 9)
    WHERE key LIKE 'learning.%';

-- ── 11b. Feature flag + menu items ───────────────────────────────────────────
UPDATE public.feature_flags SET key = 'feature.academy' WHERE key = 'feature.learning';
UPDATE public.menu_items SET feature_flag = 'feature.academy' WHERE feature_flag = 'feature.learning';
UPDATE public.menu_items SET url = '/academy', label = 'Academy' WHERE url = '/learn';
UPDATE public.menu_items SET url = '/dashboard/academy' WHERE url = '/dashboard/learning';
UPDATE public.menu_items SET label = 'dashboard.nav.academy' WHERE label = 'dashboard.nav.learning';
UPDATE public.menu_items SET url = replace(url, 'learn.japa.to', 'academy.japa.to')
    WHERE url LIKE '%learn.japa.to%';
UPDATE public.menu_items SET url = replace(url, 'university.japa.to', 'academy.japa.to')
    WHERE url LIKE '%university.japa.to%';
UPDATE public.translations SET key = 'dashboard.nav.academy' WHERE key = 'dashboard.nav.learning';

-- ── 12. JSON-LD template branding ────────────────────────────────────────────
UPDATE public.content_type_jsonld_templates
    SET template = replace(replace(template::text,
            'JapaConnect Learn', 'JapaConnect Academy'),
            'learn.japa.to', 'academy.japa.to')::jsonb
    WHERE template::text LIKE '%JapaConnect Learn%' OR template::text LIKE '%learn.japa.to%';

-- ── 13. Onboarding product tour (learn surface → academy) ───────────────────
-- system_settings.onboarding.tour_steps is a jsonb map surface → ordered
-- data-tour target keys. Rename the "learn" surface key, the "learn" step in
-- the main array, and the learn-* step keys (my-learning/certificates/
-- instructor/primary-nav are already neutral).
UPDATE public.system_settings
SET value = replace(replace(value::text, '"learn"', '"academy"'),
                    '"learn-', '"academy-')::jsonb
WHERE key = 'onboarding.tour_steps';

-- Per-user dismissal map: carry the recorded "learn" dismissal to "academy"
-- so returning users do not see the tour again.
UPDATE public.onboarding_state
SET tour_dismissals = replace(tour_dismissals::text, '"learn"', '"academy"')::jsonb
WHERE tour_dismissals ? 'learn';

-- Main-surface tour step copy (tour.learn_* → tour.academy_*).
UPDATE public.translations SET key = 'tour.academy_' || substring(key from 12)
    WHERE key LIKE 'tour.learn\_%' ESCAPE '\';

-- ── 14. Sidebar surface slug (learn → academy) ───────────────────────────────
ALTER TABLE public.sidebars DROP CONSTRAINT sidebars_app_check;
UPDATE public.sidebars
SET app = 'academy',
    slug = replace(slug, 'learn-', 'academy-'),
    name = replace(name, 'Learn', 'Academy')
WHERE app = 'learn';
ALTER TABLE public.sidebars
    ADD CONSTRAINT sidebars_app_check
    CHECK ((app = ANY (ARRAY['main'::text, 'docs'::text, 'academy'::text])));
