-- 001039_region_wiki_stub_pages.down.sql — drop the stub rows seeded by
-- 001039 (only slugs from the 001038 categories, only on region spaces).
BEGIN;

DELETE FROM wiki_revisions wr
USING wiki_pages wp
JOIN spaces s ON s.id = wp.space_id
WHERE wr.wiki_page_id = wp.id
  AND s.space_type_id = '00000000-0000-0000-0000-00000000007f'
  AND wp.slug IN ('cost-of-living','universities','attractions','embassies','banking','scholarships','other');

DELETE FROM wiki_pages wp
USING spaces s
WHERE wp.space_id = s.id
  AND s.space_type_id = '00000000-0000-0000-0000-00000000007f'
  AND wp.slug IN ('cost-of-living','universities','attractions','embassies','banking','scholarships','other');

COMMIT;
