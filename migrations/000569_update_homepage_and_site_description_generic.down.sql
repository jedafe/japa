-- Revert homepage and site description to the previous hardcoded defaults.
-- This restores the pre-generic seed content for the default tenant.

UPDATE system_settings
SET value = '"The community hub for the Nigerian diaspora and Nigerians planning to relocate abroad."'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND key = 'general.site_description';

UPDATE pages
SET
    title = 'Homepage',
    description = NULL,
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
                                    '"The Nigerian diaspora community"'
                                ),
                                '{0,props,stats}',
                                '[{"label": "Members", "value": "50K+"}, {"label": "Destinations", "value": "12+"}, {"label": "Stories shared", "value": "10K+"}]'
                            ),
                            '{6,props,text}',
                            '"Trending discussions"'
                        ),
                        '{6,props,cta,href}',
                        '"/forum"'
                    ),
                    '{7,props,entity_type}',
                    '"thread"'
                ),
                '{7,props,query,sort}',
                'null'
            ),
            '{15,props,text}',
            '"Where Nigerians are thriving"'
        ),
        '{17,props,items,0,content_blocks,0,props,html}',
        '"<p>JapaConnect is the community hub for the Nigerian diaspora. Share your journey, ask questions, compare destinations, and connect with fellow Nigerians abroad.</p>"'
    )
WHERE slug = 'homepage' AND tenant_id = '00000000-0000-0000-0000-000000000001';

UPDATE pages
SET blocks = jsonb_set(
    blocks,
    '{19,props,subhead}',
    '"Create a free account and start sharing your journey, asking questions, and connecting with fellow Nigerians abroad."'
)
WHERE slug = 'homepage' AND tenant_id = '00000000-0000-0000-0000-000000000001';
