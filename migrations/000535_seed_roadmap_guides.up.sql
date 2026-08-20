-- Seed the default roadmap guide contents referenced by the fallback
-- curatedRoadmapLibrary() in internal/assessment/service.go (ADR-071 §3).
-- The library used slugs with '/' which do not resolve to contents rows and
-- are not routable through the single-segment /content/{slug} route.
-- This migration inserts the missing contents with single-segment slugs and
-- normalizes any existing assessment_results.roadmap_content entries.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

DO $$
DECLARE
    v_tenant_id  uuid := '00000000-0000-0000-0000-000000000001';
    v_author_id  uuid;
BEGIN
    SELECT id INTO v_author_id
    FROM users
    WHERE tenant_id = v_tenant_id
    ORDER BY created_at
    LIMIT 1;

    IF v_author_id IS NULL THEN
        RAISE EXCEPTION 'No user found for tenant %', v_tenant_id;
    END IF;

    -- Re-activate any soft-deleted roadmap guide contents so the migration
    -- is idempotent across up/down cycles.
    UPDATE contents
    SET deleted_at = NULL,
        status = 'published',
        updated_at = now()
    WHERE tenant_id = v_tenant_id
      AND slug IN (
          'guide-banking-finance-relocation',
          'tool-cost-of-living-calculator',
          'guide-visa-pathways-overview',
          'guide-visa-document-checklist',
          'guide-destination-research-guide',
          'guide-credential-evaluation',
          'guide-professional-certifications',
          'guide-connecting-with-diaspora',
          'guide-emotional-readiness-relocation',
          'track-language-proficiency'
      );

    -- Insert the missing roadmap guide contents.
    INSERT INTO contents (
        tenant_id,
        author_id,
        content_type,
        title,
        slug,
        body,
        body_html,
        field_values,
        content_hash,
        status,
        visibility,
        is_locked,
        is_pinned,
        read_time_minutes,
        view_count,
        comment_count,
        share_count,
        reshare_count,
        published_at,
        created_at,
        updated_at,
        seo_indexable,
        guide_category
    )
    SELECT
        v_tenant_id,
        v_author_id,
        'guide',
        g.title,
        g.slug,
        g.body,
        '',
        '{}'::jsonb,
        encode(digest(lower(regexp_replace(g.body, '\s+', ' ', 'g'))::text, 'sha256'), 'hex'),
        'published',
        'public',
        false,
        false,
        0,
        0,
        0,
        0,
        0,
        now(),
        now(),
        now(),
        true,
        'roadmap'
    FROM (VALUES
        ('Build your relocation savings fund', 'guide-banking-finance-relocation', 'Calculate your target savings based on your destination''s cost of living and create a monthly savings plan.'),
        ('Use the Cost of Living Calculator', 'tool-cost-of-living-calculator', 'Compare living costs between your current city and target destination to set a realistic budget.'),
        ('Determine your visa pathway', 'guide-visa-pathways-overview', 'Research visa categories you qualify for based on your skills, experience, and destination.'),
        ('Prepare your visa application documents', 'guide-visa-document-checklist', 'Gather and organise the documents required for your target visa category.'),
        ('Research housing at your destination', 'guide-destination-research-guide', 'Compare neighbourhoods, rental costs, and short-term accommodation options before you arrive.'),
        ('Get your credentials evaluated', 'guide-credential-evaluation', 'Research credential recognition requirements for your profession in your target country.'),
        ('Upgrade your professional certifications', 'guide-professional-certifications', 'Identify and pursue certifications that are in demand in your destination''s job market.'),
        ('Connect with the diaspora community', 'guide-connecting-with-diaspora', 'Join spaces and forums for Nigerians in your destination country for on-the-ground insights and support.'),
        ('Plan your emotional and family readiness', 'guide-emotional-readiness-relocation', 'Reflect on cultural adjustment, family logistics, and support networks to ease the transition.'),
        ('Start a language learning track', 'track-language-proficiency', 'Begin structured language lessons for your destination country''s primary language.')
    ) AS g(title, slug, body)
    WHERE NOT EXISTS (
        SELECT 1 FROM contents c
        WHERE c.tenant_id = v_tenant_id AND c.slug = g.slug
    );

    -- Normalize legacy assessment roadmap_content slugs that used '/'
    -- so existing results resolve to the newly seeded contents.
    UPDATE assessment_results
    SET roadmap_content = (
        SELECT COALESCE(
            jsonb_agg(
                jsonb_set(
                    step,
                    '{content_slug}',
                    to_jsonb(replace(step->>'content_slug', '/', '-')),
                    false
                )
            ),
            '[]'::jsonb
        )
        FROM jsonb_array_elements(roadmap_content) AS step
    )
    WHERE roadmap_content IS NOT NULL
      AND jsonb_array_length(roadmap_content) > 0;
END $$;
