-- Seed comparable fields into the empty product-type overview groups
-- (ai-agent, ai-model, cloud, hosting, mobile-app, saas, web-app, website).
-- The provisioning chain creates the group shells only — fields were
-- expected to be curated later, leaving the Compare-fields rail empty.

BEGIN;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('pricing_model',   'Pricing model',      'text',     1),
    ('free_tier',       'Free tier',          'boolean',  2),
    ('channels',        'Channels',           'text',     3),
    ('integrations',    'Integrations',       'text',     4),
    ('human_handoff',   'Human handoff',      'boolean',  5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_ai_agent_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('context_window_tokens', 'Context window (tokens)', 'number',      1),
    ('input_price_per_mtok',  'Input price / 1M tokens', 'price_range', 2),
    ('open_weights',          'Open weights',            'boolean',     3),
    ('providers',             'Providers',               'text',        4),
    ('max_output_tokens',     'Max output tokens',       'number',      5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_ai_model_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('free_tier',     'Free tier',          'boolean',    1),
    ('regions_count', 'Regions',            'number',     2),
    ('pricing_model', 'Pricing model',      'text',       3),
    ('uptime_sla',    'Uptime SLA (%)',     'percentage', 4),
    ('support_tiers', 'Support tiers',      'text',       5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_cloud_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('free_tier',        'Free tier',        'boolean',    1),
    ('uptime_sla',       'Uptime SLA (%)',   'percentage', 2),
    ('storage_gb',       'Storage (GB)',     'number',     3),
    ('ssl_included',     'SSL included',     'boolean',    4),
    ('support_channels', 'Support channels', 'text',       5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_hosting_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('platforms',          'Platforms',          'text',        1),
    ('price',              'Price',              'price_range', 2),
    ('in_app_purchases',   'In-app purchases',   'boolean',     3),
    ('offline_mode',       'Offline mode',       'boolean',     4),
    ('store_rating',       'Store rating',       'number',      5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_mobile_app_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('pricing_model',  'Pricing model',  'text',        1),
    ('free_trial',     'Free trial',     'boolean',     2),
    ('starting_price', 'Starting price', 'price_range', 3),
    ('integrations',   'Integrations',   'text',        4),
    ('sso_support',    'SSO support',    'boolean',     5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_saas_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('platforms',        'Platforms',        'text',    1),
    ('free_tier',        'Free tier',        'boolean', 2),
    ('pricing_model',    'Pricing model',    'text',    3),
    ('integrations',     'Integrations',     'text',    4),
    ('offline_support',  'Offline support',  'boolean', 5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_web_app_overview'
ON CONFLICT DO NOTHING;

INSERT INTO custom_fields (group_id, key, label, field_type, is_system, position)
SELECT g.id, v.key, v.label, v.ft, true, v.pos
FROM custom_field_groups g
JOIN (VALUES
    ('responsive_design', 'Responsive design', 'boolean', 1),
    ('custom_domain',     'Custom domain',     'boolean', 2),
    ('pricing_model',     'Pricing model',     'text',    3),
    ('free_plan',         'Free plan',         'boolean', 4),
    ('integrations',      'Integrations',      'text',    5)
) AS v(key, label, ft, pos) ON true
WHERE g.slug = 'wiki_website_overview'
ON CONFLICT DO NOTHING;

COMMIT;
