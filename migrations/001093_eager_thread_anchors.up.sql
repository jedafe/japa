-- 001093 — eager thread anchors (unified-discussion spec): every wiki page
-- and every /tools/{tool} page gets a backing discussion thread at
-- creation time. Live creation is minted in-process (wikiService
-- ThreadAnchorMinter → comments.Service.CreateAnchor); this migration
-- backfills anchors for everything that already exists.
--
-- Anchor rows are authored by a dedicated bot user (contents.author_id is
-- NOT NULL — generated threads need a system author, same convention as
-- the intel-* bot users). All anchors live in the seeded
-- 'community-threads' space (001091).

-- 1. System author for generated thread rows.
INSERT INTO public.users (id, email, username, profile_type, status, is_bot)
VALUES (
    '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02',
    'community-threads@bot.internal',
    'community-threads',
    'regular',
    'active',
    true
)
ON CONFLICT (id) DO NOTHING;

-- 2. Wiki pages → thread anchors. Only pages that are live (public,
--    active space + at least one approved revision) — pending/inactive
--    pages mint their anchor on first approval in-process instead.
--    ON CONFLICT guards re-runs and rows already anchored by lazy minting.
WITH new_threads AS (
    INSERT INTO public.contents (
        author_id, space_id, content_type, title, slug, body, body_html,
        status, visibility, published_at
    )
    SELECT
        '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02',
        '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01',
        'discussion',
        'Discussing: ' || wp.title,
        'thread-wiki-' || substr(replace(wp.id::text, '-', ''), 1, 12),
        '',
        '',
        'published',
        'public',
        now()
    FROM wiki_pages wp
    JOIN spaces s ON s.id = wp.space_id AND s.deleted_at IS NULL
        AND s.visibility = 'public' AND s.status = 'active'
    WHERE wp.deleted_at IS NULL
      AND wp.is_active
      -- Only live pages get a public thread: pages whose revisions are
      -- all still pending mint their anchor on first approval instead.
      AND EXISTS (
          SELECT 1 FROM wiki_revisions r
          WHERE r.wiki_page_id = wp.id
            AND r.status = 'approved' AND r.deleted_at IS NULL
      )
      AND NOT EXISTS (
          SELECT 1 FROM thread_anchors ta
          WHERE ta.entity_type = 'wiki_page' AND ta.entity_key = wp.id::text
      )
    ON CONFLICT (slug) DO NOTHING
    RETURNING id, slug
)
INSERT INTO thread_anchors (content_id, entity_type, entity_key, entity_url, entity_title, entity_summary)
SELECT
    nt.id,
    'wiki_page',
    wp.id::text,
    '/wiki/' || s.slug || '/' || wp.slug,
    wp.title,
    'Discussing a page in the ' || s.name || ' wiki.'
FROM wiki_pages wp
JOIN spaces s ON s.id = wp.space_id AND s.deleted_at IS NULL
    AND s.visibility = 'public' AND s.status = 'active'
JOIN new_threads nt ON nt.slug = 'thread-wiki-' || substr(replace(wp.id::text, '-', ''), 1, 12)
WHERE wp.deleted_at IS NULL
ON CONFLICT (entity_type, entity_key) DO NOTHING;

-- 3. Registered /tools/{tool} pages → thread anchors.
WITH tool_slugs (slug, title, summary) AS (
    VALUES
        ('arrival-checklist',  'Arrival Checklist',          'A personalized arrival checklist for your destination.'),
        ('attestation-guide',  'Attestation Guide',          'Step-by-step document attestation guidance.'),
        ('community-locator',  'Community Locator',          'Find diaspora communities near you.'),
        ('cost-of-living',     'Cost of Living Calculator',  'Compare living costs across destination cities.'),
        ('credential-checker', 'Credential Checker',         'Check how your credentials transfer abroad.'),
        ('crs-calculator',     'CRS Calculator',             'Estimate your Comprehensive Ranking System score.'),
        ('currency-converter', 'Currency Converter',         'Convert between origin and destination currencies.'),
        ('cv-formatter',       'CV Formatter',               'Format your CV for your destination market.'),
        ('eca-guide',          'ECA Guide',                  'Educational Credential Assessment guidance.'),
        ('embassy-alerter',    'Embassy Alerter',            'Alerts for embassy appointment availability.'),
        ('housing-guide',      'Housing Guide',              'Housing guides for your destination city.'),
        ('language-test',      'Language Test Hub',          'Prepare for IELTS, TOEFL and other language tests.'),
        ('pof-calculator',     'Proof of Funds Calculator',  'Compute proof-of-funds requirements.'),
        ('processing-tracker', 'Processing Time Tracker',    'Track visa processing times.'),
        ('readiness',          'Readiness Assessment',       'Assess your readiness to relocate abroad.'),
        ('remittance',         'Remittance Rate Tracker',    'Track live remittance rates across corridors.'),
        ('sop-generator',      'SOP Generator',              'Generate a statement of purpose draft.'),
        ('tax-calculator',     'Tax Calculator',             'Estimate taxes in your destination country.'),
        ('travel',             'Travel Search',              'Search and compare flights and hotels.'),
        ('visa-checklist',     'Visa Checklist',             'Generate your visa document checklist by destination.'),
        ('visa-routes',        'Visa Routes',                'Compare visa routes for your destination.')
), new_threads AS (
    INSERT INTO public.contents (
        author_id, space_id, content_type, title, slug, body, body_html,
        status, visibility, published_at
    )
    SELECT
        '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02',
        '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01',
        'discussion',
        'Discussing: ' || ts.title,
        'thread-tool-' || ts.slug,
        '',
        '',
        'published',
        'public',
        now()
    FROM tool_slugs ts
    WHERE NOT EXISTS (
        SELECT 1 FROM thread_anchors ta
        WHERE ta.entity_type = 'tool' AND ta.entity_key = ts.slug
    )
    ON CONFLICT (slug) DO NOTHING
    RETURNING id, slug
)
INSERT INTO thread_anchors (content_id, entity_type, entity_key, entity_url, entity_title, entity_summary)
SELECT
    nt.id,
    'tool',
    ts.slug,
    '/tools/' || ts.slug,
    ts.title,
    ts.summary
FROM tool_slugs ts
JOIN new_threads nt ON nt.slug = 'thread-tool-' || ts.slug
ON CONFLICT (entity_type, entity_key) DO NOTHING;

-- 4. Primary space junction for every anchor row (CreateContent writes one
--    per content; backfilled rows need it for space-scoped listing/perms).
INSERT INTO public.content_spaces (content_id, space_id, is_primary)
SELECT c.id, '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01', true
FROM contents c
WHERE c.author_id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f02'
  AND c.space_id = '9c7c2a1d-2c47-4e3b-9f9d-7a1b9e6a2f01'
  AND NOT EXISTS (
      SELECT 1 FROM content_spaces cs WHERE cs.content_id = c.id AND cs.is_primary
  );
