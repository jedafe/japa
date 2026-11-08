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
        -- Same body the in-process minter writes: the entity description
        -- plus a conversation prompt (TipTap doc).
        jsonb_build_object('type', 'doc', 'content', jsonb_build_array(
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text', 'Discussing a page in the ' || s.name || ' wiki.'))),
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text', 'Questions, experiences, and tips about ' || wp.title || ' — join the discussion below.')))
        ))::text,
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
    ON CONFLICT DO NOTHING
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
-- Summary strings mirror comments.toolAnchorMeta — keep them in sync.
WITH tool_slugs (slug, title, summary) AS (
    VALUES
        ('arrival-checklist',  'Arrival Checklist',          'A personalized checklist for your first weeks after landing — documents to carry, registrations to complete, bank, phone, transport, and the tasks people commonly forget. Built per destination so nothing slips through.'),
        ('attestation-guide',  'Attestation Guide',          'Step-by-step guidance for getting your documents attested — which offices to visit, what order to follow, and the fees and timelines at each stage. Covers Nigerian and destination-country requirements.'),
        ('community-locator',  'Community Locator',          'Find diaspora communities near you — associations, religious centres, cultural groups, and informal networks organised by city and country of origin.'),
        ('cost-of-living',     'Cost of Living Calculator',  'Compare real living costs across destination cities — rent, food, transport, childcare, and utilities — side by side with your origin city so you can budget before you move.'),
        ('credential-checker', 'Credential Checker',         'Check how your degrees and professional credentials transfer abroad — recognition status, evaluation bodies, and the extra steps each destination requires.'),
        ('crs-calculator',     'CRS Calculator',             'Estimate your Comprehensive Ranking System score for Canadian Express Entry — age, education, language, and work experience broken down per factor so you can see where to gain points.'),
        ('currency-converter', 'Currency Converter',         'Convert between your home and destination currencies with live rates — useful for budgeting, remittances, and comparing salaries across countries.'),
        ('cv-formatter',       'CV Formatter',               'Format your CV for your destination market — the structure, length, and conventions recruiters in that country actually expect.'),
        ('eca-guide',          'ECA Guide',                  'Educational Credential Assessment guidance — which ECA bodies evaluate your qualifications, how long each takes, and which one your destination accepts.'),
        ('embassy-alerter',    'Embassy Alerter',            'Alerts when embassy and visa application centre appointment slots open — interview calendars fill fast and checking manually is a losing game.'),
        ('housing-guide',      'Housing Guide',              'Housing guides for destination cities — where newcomers typically live, what deposits and paperwork landlords expect, and realistic rent ranges by area.'),
        ('language-test',      'Language Test Hub',          'Prepare for IELTS, TOEFL, PTE and other language tests — target scores per visa route, prep resources, and booking links.'),
        ('pof-calculator',     'Proof of Funds Calculator',  'Compute proof-of-funds requirements for your destination — minimum balances, how many months of statements, and acceptable account types.'),
        ('processing-tracker', 'Processing Time Tracker',    'Track current visa and permit processing times — real submission-to-decision timelines by route, so you can plan flights and resignations sensibly.'),
        ('readiness',          'Readiness Assessment',       'Assess how ready you are to relocate — documents, finances, credentials, and timeline scored so you know exactly what''s left before you can move.'),
        ('remittance',         'Remittance Rate Tracker',    'Track live remittance rates and fees across corridors — compare providers before you send money home.'),
        ('sop-generator',      'SOP Generator',              'Draft a statement of purpose for admissions or visa applications — structured around your background, goals, and the programme you''re targeting.'),
        ('tax-calculator',     'Tax Calculator',             'Estimate income tax and take-home pay in your destination country — so salary offers can be compared on net, not gross.'),
        ('travel',             'Travel Search',              'Search and compare flights and hotels for your route — prices across airlines and dates so you can book the legwork smartly.'),
        ('visa-checklist',     'Visa Checklist',             'Generate your visa document checklist by destination — every required document, form, and supporting evidence for your route, in submission order.'),
        ('visa-routes',        'Visa Routes',                'Compare visa routes for your destination — eligibility, cost, timeline, and pathway-to-residency for each option.')
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
        -- Same body the in-process minter writes (TipTap doc).
        jsonb_build_object('type', 'doc', 'content', jsonb_build_array(
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text', ts.summary))),
            jsonb_build_object('type', 'paragraph', 'content', jsonb_build_array(
                jsonb_build_object('type', 'text', 'text', 'Questions, experiences, and tips about ' || ts.title || ' — join the discussion below.')))
        ))::text,
        '',
        'published',
        'public',
        now()
    FROM tool_slugs ts
    WHERE NOT EXISTS (
        SELECT 1 FROM thread_anchors ta
        WHERE ta.entity_type = 'tool' AND ta.entity_key = ts.slug
    )
    ON CONFLICT DO NOTHING
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
