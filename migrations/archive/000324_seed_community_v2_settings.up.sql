-- 000324_seed_community_v2_settings.up.sql
-- Seeds system_settings for all V2 Community Features (§81–§89, §98).
-- §79 (story) and §80 (wiki) settings are seeded by 000300 and 000303.
-- Ref: japaconnect-engineering/sections/81-89,98

INSERT INTO system_settings (tenant_id, key, value) VALUES
    -- §81 Scam Alert Board
    ('00000000-0000-0000-0000-000000000001', 'scam_alert.enabled',           'false'),
    ('00000000-0000-0000-0000-000000000001', 'scam_alert.anonymous_reports', 'true'),
    -- §82 Japa Planning Groups
    ('00000000-0000-0000-0000-000000000001', 'planning_group.enabled',       'false'),
    -- §83 Visa Timeline Tracker
    ('00000000-0000-0000-0000-000000000001', 'visa_timeline.enabled',        'false'),
    -- §84 Community Digest Widget
    ('00000000-0000-0000-0000-000000000001', 'digest.enabled',               'false'),
    ('00000000-0000-0000-0000-000000000001', 'digest.email',                 'true'),
    -- §85 Mutual Aid Board
    ('00000000-0000-0000-0000-000000000001', 'mutual_aid.enabled',           'false'),
    -- §86 Community Calendar
    ('00000000-0000-0000-0000-000000000001', 'calendar.enabled',             'false'),
    -- §87 Japa Stage Progress Profiles
    ('00000000-0000-0000-0000-000000000001', 'japa_stage.enabled',           'false'),
    -- §88 Ask an Expat AMA
    ('00000000-0000-0000-0000-000000000001', 'forum.ama.enabled',            'false'),
    ('00000000-0000-0000-0000-000000000001', 'ama_cooldown_days',            '30'),
    -- §89 Community Achievement Feed
    ('00000000-0000-0000-0000-000000000001', 'achievement.enabled',          'false'),
    ('00000000-0000-0000-0000-000000000001', 'achievement_reactions',        '[]'),
    -- §98 Service Directory
    ('00000000-0000-0000-0000-000000000001', 'service_directory.enabled',    'false'),
    ('00000000-0000-0000-0000-000000000001', 'service_directory.subdomain',  '""'),
    ('00000000-0000-0000-0000-000000000001', 'service_directory.custom_domain', '""')
ON CONFLICT (tenant_id, key) DO NOTHING;
