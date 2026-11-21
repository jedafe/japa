DROP INDEX IF EXISTS doc_pages_audience_slug_lang_key;
CREATE UNIQUE INDEX doc_pages_audience_slug_key ON public.doc_pages USING btree (audience, slug);
ALTER TABLE doc_pages DROP CONSTRAINT IF EXISTS doc_pages_lang_code_check;
ALTER TABLE doc_pages DROP COLUMN IF EXISTS lang_code;
