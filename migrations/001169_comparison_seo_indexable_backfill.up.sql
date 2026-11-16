-- Backfill seo_indexable on live JCE comparison rows to the §30/§43
-- derived value: detailed depth + published status + public visibility +
-- body word count >= seo.sitemap.comparison_min_words (falling back to the
-- 250 compiled default when the setting is missing or non-positive,
-- matching bots.ComparisonSeoIndexable).
--
-- Rows written before the derivation shipped can carry a stale flag both
-- ways: rows published through the generic status path kept
-- seo_indexable=false despite qualifying, and quick rows can hold a leaked
-- true from the column default or pre-derivation writes.
--
-- Scope: JCE rows only (INNER JOIN comparison_content_metadata). Legacy
-- wizard comparisons carry no metadata row — they are not depth-gated and
-- keep their editorial flag untouched.
--
-- updated_at is intentionally left untouched — a flag repair must not
-- masquerade as a content edit (sitemap lastmod, updated_at-ordered
-- feeds). Rows already correct are skipped via IS DISTINCT FROM so the
-- backfill only writes what actually changes.

DO $$
DECLARE
  v_floor   int;
  v_updated int;
BEGIN
  SELECT (value #>> '{}')::int
    INTO v_floor
    FROM public.system_settings
   WHERE key = 'seo.sitemap.comparison_min_words';

  -- Same fallback as the runtime reader: missing or non-positive -> 250.
  IF v_floor IS NULL OR v_floor <= 0 THEN
    v_floor := 250;
  END IF;

  UPDATE public.contents c
     SET seo_indexable = d.indexable
    FROM (
      SELECT c2.id,
             m.depth = 'detailed'
               AND c2.status = 'published'
               AND c2.visibility = 'public'
               AND COALESCE(array_length(string_to_array(trim(c2.body), ' '), 1), 0) >= v_floor
             AS indexable
        FROM public.contents c2
        JOIN public.comparison_content_metadata m ON m.content_id = c2.id
       WHERE c2.content_type = 'comparison'
         AND c2.deleted_at IS NULL
    ) d
   WHERE c.id = d.id
     AND c.seo_indexable IS DISTINCT FROM d.indexable;

  GET DIAGNOSTICS v_updated = ROW_COUNT;
  RAISE NOTICE 'comparison seo_indexable backfill: % row(s) corrected (word floor %)', v_updated, v_floor;
END $$;
