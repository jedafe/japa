-- Revert 000800: Japa Academy -> Learn + Japa University

-- Onboarding product tour (reverse of section 13).
UPDATE public.translations SET key = 'tour.learn_' || substring(key from 14)
    WHERE key LIKE 'tour.academy\_%' ESCAPE '\';
UPDATE public.onboarding_state
SET tour_dismissals = replace(tour_dismissals::text, '"academy"', '"learn"')::jsonb
WHERE tour_dismissals ? 'academy';
UPDATE public.system_settings
SET value = replace(replace(value::text, '"academy"', '"learn"'),
                    '"academy-', '"learn-')::jsonb
WHERE key = 'onboarding.tour_steps';

UPDATE public.content_type_jsonld_templates
    SET template = replace(replace(template::text,
            'JapaConnect Academy', 'JapaConnect Learn'),
            'academy.japaconnect.com', 'learn.japaconnect.com')::jsonb
    WHERE template::text LIKE '%JapaConnect Academy%' OR template::text LIKE '%academy.japaconnect.com%';

UPDATE public.translations SET key = 'learning' || substring(key from 8)
    WHERE key LIKE 'academy.%' AND substring(key from 9) IN (
        'title', 'description', 'track_id', 'module_id', 'content_markdown', 'cancel');
UPDATE public.translations SET key = 'learn' || substring(key from 8)
    WHERE key LIKE 'academy.%';
UPDATE public.notification_types SET key = 'university_instructor_approved'
    WHERE key = 'academy_instructor_approved';
UPDATE public.notification_types SET key = 'university_course_approved'
    WHERE key = 'academy_course_approved';
UPDATE public.notification_types
    SET description = replace(description, 'the Japa Academy catalog', 'the marketplace')
    WHERE key = 'university_course_approved';
UPDATE public.ad_slot_types SET slot_name = 'learn' || substring(slot_name from 8)
    WHERE slot_name LIKE 'academy\_%' ESCAPE '\';
UPDATE public.menus SET slug = 'learn' || substring(slug from 8)
    WHERE slug LIKE 'academy\_%' ESCAPE '\';
UPDATE public.menus SET name = replace(name, 'Academy ', 'Learn ')
    WHERE slug LIKE 'learn\_%' ESCAPE '\';

UPDATE public.ai_tools SET name = 'learn' || substring(name from 8)
    WHERE name LIKE 'academy.%';
UPDATE public.ai_tools SET category = 'agent_learning' WHERE category = 'agent_academy';
UPDATE public.ai_tools SET required_permission = 'learning' || substring(required_permission from 8)
    WHERE required_permission LIKE 'academy.%';
UPDATE public.ai_tools SET config = replace(replace(config::text,
        '"academy.', '"learn.'),
        '"handler": "Academy', '"handler": "Learn')::jsonb
    WHERE config::text ~ '"academy\.|"handler": "Academy';

UPDATE public.system_settings SET value = replace(value::text, '"/academy"', '"/university"')::jsonb
    WHERE value::text LIKE '%"/academy"%';
UPDATE public.system_settings SET value = replace(value::text, 'Japa Academy', 'Japa University')::jsonb
    WHERE value::text LIKE '%Japa Academy%';
UPDATE public.system_settings SET value = replace(value::text, 'japa-academy', 'japa-university')::jsonb
    WHERE value::text LIKE '%japa-academy%';
UPDATE public.system_settings SET value = '"university"'::jsonb WHERE key = 'academy.subdomain';
UPDATE public.system_settings SET key = 'university' || substring(key from 8)
    WHERE key LIKE 'academy.%';

UPDATE public.permissions SET key = 'university' || substring(key from 8)
    WHERE key LIKE 'academy.%' AND key ~ 'instructor|course|review|payout|revenue|category';
UPDATE public.permissions SET key = 'learning' || substring(key from 8)
    WHERE key LIKE 'academy.%';

UPDATE public.translations SET key = 'dashboard.nav.learning' WHERE key = 'dashboard.nav.academy';
UPDATE public.menu_items SET url = replace(url, 'academy.japaconnect.com', 'learn.japaconnect.com')
    WHERE url LIKE '%academy.japaconnect.com%';
UPDATE public.menu_items SET label = 'dashboard.nav.learning' WHERE label = 'dashboard.nav.academy';
UPDATE public.menu_items SET url = '/dashboard/learning' WHERE url = '/dashboard/academy';
UPDATE public.menu_items SET url = '/learn', label = 'Learn' WHERE url = '/academy';
UPDATE public.menu_items SET feature_flag = 'feature.learning' WHERE feature_flag = 'feature.academy';
UPDATE public.feature_flags SET key = 'feature.learning' WHERE key = 'feature.academy';

UPDATE public.custom_field_values  SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.moderation_appeals   SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.intel_records        SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.trending_snapshots   SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.slug_history         SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.platform_events      SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.follows              SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.content_embeddings   SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.review_aggregates    SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.reviews              SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.reactions            SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';
UPDATE public.bookmarks            SET entity_type = 'learning_track' WHERE entity_type = 'academy_track';

DO $academy$
DECLARE
    r        record;
    new_name text;
    pairs    text[][] := ARRAY[
        ['academy_track_categories',       'university_track_categories'],
        ['academy_certificates',           'learning_certificates'],
        ['academy_tracks',                 'learning_tracks'],
        ['academy_modules',                'learning_modules'],
        ['academy_lessons',                'learning_lessons'],
        ['academy_lesson_progress',        'lesson_progress'],
        ['academy_quizzes',                'lesson_quizzes'],
        ['academy_instructor_profiles',    'instructor_profiles'],
        ['academy_instructor_payouts',     'instructor_payouts'],
        ['academy_course_access_grants',   'course_access_grants'],
        ['academy_subscription_plan_tracks','subscription_plan_tracks'],
        ['academy_enrollments',            'enrollments'],
        ['academy',                        'university']
    ];
    i int;
    sel text := 'academy_';
BEGIN
    FOR r IN
        SELECT indexname AS n FROM pg_indexes
        WHERE schemaname = 'public' AND indexname ~ sel
          AND indexname !~ 'academy_university'
    LOOP
        new_name := r.n;
        FOR i IN 1..array_length(pairs, 1) LOOP
            new_name := replace(new_name, pairs[i][1], pairs[i][2]);
        END LOOP;
        IF new_name <> r.n AND new_name !~ 'academy' AND NOT EXISTS (
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
          AND cls.relname LIKE 'academy\_%' ESCAPE '\'
    LOOP
        new_name := r.n;
        FOR i IN 1..array_length(pairs, 1) LOOP
            new_name := replace(new_name, pairs[i][1], pairs[i][2]);
        END LOOP;
        IF new_name <> r.n AND new_name !~ 'academy' AND NOT EXISTS (
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

ALTER TABLE public.academy_tracks RENAME COLUMN academy_featured          TO university_featured;
ALTER TABLE public.academy_tracks RENAME COLUMN academy_rejection_reason  TO university_rejection_reason;
ALTER TABLE public.academy_tracks RENAME COLUMN academy_reviewed_at       TO university_reviewed_at;
ALTER TABLE public.academy_tracks RENAME COLUMN academy_submitted_at      TO university_submitted_at;
ALTER TABLE public.academy_tracks RENAME COLUMN academy_submission_status TO university_submission_status;
ALTER TABLE public.academy_tracks RENAME COLUMN is_published_to_academy   TO is_published_to_university;

ALTER TABLE public.academy_track_categories       RENAME TO university_track_categories;
ALTER TABLE public.academy_subscription_plan_tracks RENAME TO subscription_plan_tracks;
ALTER TABLE public.academy_course_access_grants   RENAME TO course_access_grants;
ALTER TABLE public.academy_instructor_payouts     RENAME TO instructor_payouts;
ALTER TABLE public.academy_instructor_profiles    RENAME TO instructor_profiles;
ALTER TABLE public.academy_enrollments            RENAME TO enrollments;
ALTER TABLE public.academy_lesson_progress        RENAME TO lesson_progress;
ALTER TABLE public.academy_quizzes                RENAME TO lesson_quizzes;
ALTER TABLE public.academy_certificates           RENAME TO learning_certificates;
ALTER TABLE public.academy_lessons                RENAME TO learning_lessons;
ALTER TABLE public.academy_modules                RENAME TO learning_modules;
ALTER TABLE public.academy_tracks                 RENAME TO learning_tracks;

ALTER TABLE public.sidebars DROP CONSTRAINT sidebars_app_check;
UPDATE public.sidebars
SET app = 'learn',
    slug = replace(slug, 'academy-', 'learn-'),
    name = replace(name, 'Academy', 'Learn')
WHERE app = 'academy';
ALTER TABLE public.sidebars
    ADD CONSTRAINT sidebars_app_check
    CHECK ((app = ANY (ARRAY['main'::text, 'docs'::text, 'learn'::text])));
