-- 001214_doc_pages_lang_code
-- ROADMAP-DOCS Phase 08.03: doc_pages carries one row per language. The
-- (audience, slug) unique claim becomes (audience, slug, lang_code) so a
-- French row can coexist with its English source. The index stays
-- non-partial: trashed rows keep their claim until force-delete (001209).
-- GET /docs/{audience}/{slug}?lang=fr returns the French row when published,
-- else the English row with content_lang='en' for the fallback notice.
ALTER TABLE doc_pages ADD COLUMN IF NOT EXISTS lang_code text NOT NULL DEFAULT 'en';

ALTER TABLE doc_pages
    ADD CONSTRAINT doc_pages_lang_code_check CHECK (lang_code ~ '^[a-z]{2}$');

DROP INDEX IF EXISTS doc_pages_audience_slug_key;
CREATE UNIQUE INDEX doc_pages_audience_slug_lang_key
    ON public.doc_pages USING btree (audience, slug, lang_code);
