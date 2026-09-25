-- Replace Nigerian-specific hardcoded copy on the default landing page and
-- site description with geo-tokenised, generic diaspora copy.
-- Ref: Rule 16 (no hardcoded frontend data), ADR-064 (geo-aware copy).

-- 1. Generic site-wide SEO description.
-- Default tenant UUID: '00000000-0000-0000-0000-000000000001'
INSERT INTO system_settings (tenant_id, key, value, is_public) VALUES
    ('00000000-0000-0000-0000-000000000001', 'general.site_description', '"A community hub for the diaspora and people planning life abroad."', true)
ON CONFLICT (tenant_id, key) DO UPDATE SET value = '"A community hub for the diaspora and people planning life abroad."';

-- 2. Generic, tokenised Page Builder homepage copy and metadata.
-- Only touches the default tenant homepage so per-tenant admin edits are not
-- clobbered on re-run.
UPDATE pages
SET
    title = 'JapaConnect — The {{diaspora_phrase}}',
    description = 'Stories, discussions, comparisons, and guides for {{diaspora_phrase}} navigating life abroad.',
    blocks = jsonb_set(
        jsonb_set(
            jsonb_set(
                jsonb_set(
                    jsonb_set(
                        jsonb_set(
                            jsonb_set(
                                jsonb_set(
                                    blocks,
                                    '{0,props,eyebrow}',
                                    '"The {{diaspora_phrase}}"'
                                ),
                                '{0,props,stats}',
                                '[]'
                            ),
                            '{6,props,text}',
                            '"Trending community discussions"'
                        ),
                        '{6,props,cta,href}',
                        '"/explore?sort=rising"'
                    ),
                    '{7,props,entity_type}',
                    '"post"'
                ),
                '{7,props,query,sort}',
                '"rising"'
            ),
            '{15,props,text}',
            '"Where our community is thriving"'
        ),
        '{17,props,items,0,content_blocks,0,props,html}',
        '"<p>JapaConnect is the community hub for the {{diaspora_phrase}}. Share your journey, ask questions, compare destinations, and connect with fellow diaspora members abroad.</p>"'
    )
WHERE slug = 'homepage' AND tenant_id = '00000000-0000-0000-0000-000000000001';

-- 3. Generic CTA copy on the default homepage.
UPDATE pages
SET blocks = jsonb_set(
    blocks,
    '{19,props,subhead}',
    '"Create a free account and start sharing your journey, asking questions, and connecting with fellow diaspora members abroad."'
)
WHERE slug = 'homepage' AND tenant_id = '00000000-0000-0000-0000-000000000001';
