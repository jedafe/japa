-- 000938_content_reports_review_fixes.down.sql

ALTER TABLE public.content_reports
    DROP CONSTRAINT content_reports_notes_length;

ALTER TABLE public.content_reports
    DROP CONSTRAINT content_reports_reason_nonempty;

-- Pre-cleanup: any rows carrying admin-configured reasons outside the legacy
-- eight are deleted so the CHECK can be restored. Reports are queue items,
-- not user content, so this mirrors the rollback semantics of the original
-- migration.
DELETE FROM public.content_reports
WHERE reason <> ALL (ARRAY['spam'::text, 'misinformation'::text, 'harassment'::text,
    'hate_speech'::text, 'illegal_content'::text, 'self_harm'::text, 'copyright'::text, 'other'::text]);

ALTER TABLE public.content_reports
    ADD CONSTRAINT content_reports_reason_check
    CHECK ((reason = ANY (ARRAY['spam'::text, 'misinformation'::text, 'harassment'::text,
        'hate_speech'::text, 'illegal_content'::text, 'self_harm'::text, 'copyright'::text, 'other'::text])));

DELETE FROM public.content_reports WHERE entity_type = 'forum_reply';

ALTER TABLE public.content_reports
    DROP CONSTRAINT content_reports_entity_type_check;

ALTER TABLE public.content_reports
    ADD CONSTRAINT content_reports_entity_type_check
    CHECK ((entity_type = ANY (ARRAY['content'::text, 'comment'::text, 'user'::text,
        'space'::text, 'message'::text, 'source'::text])));
