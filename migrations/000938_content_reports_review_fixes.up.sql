-- 000938_content_reports_review_fixes.up.sql
-- Report-system review fixes:
--   1. Add 'forum_reply' to the entity_type CHECK so forum replies are
--      reportable (POST /forum/replies/{id}/report).
--   2. Drop the reason CHECK — valid report reasons are now admin-configurable
--      via the moderation.report_reasons system setting (Section 51 / Rule 16)
--      and validated in the service layer. A hardcoded CHECK silently broke
--      any admin-added reason with a 500-level constraint violation.
--   3. Bound notes length so reports can't carry unbounded payloads.

ALTER TABLE public.content_reports
    DROP CONSTRAINT content_reports_entity_type_check;

ALTER TABLE public.content_reports
    ADD CONSTRAINT content_reports_entity_type_check
    CHECK ((entity_type = ANY (ARRAY['content'::text, 'comment'::text, 'user'::text,
        'space'::text, 'message'::text, 'source'::text, 'forum_reply'::text])));

ALTER TABLE public.content_reports
    DROP CONSTRAINT content_reports_reason_check;

ALTER TABLE public.content_reports
    ADD CONSTRAINT content_reports_reason_nonempty CHECK (length(btrim(reason)) > 0);

ALTER TABLE public.content_reports
    ADD CONSTRAINT content_reports_notes_length CHECK (notes IS NULL OR char_length(notes) <= 5000);
