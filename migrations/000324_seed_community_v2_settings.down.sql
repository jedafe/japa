-- 000324_seed_community_v2_settings.down.sql
DELETE FROM system_settings
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key IN (
    'scam_alert.enabled','scam_alert.anonymous_reports',
    'planning_group.enabled',
    'visa_timeline.enabled',
    'digest.enabled','digest.email',
    'mutual_aid.enabled',
    'calendar.enabled',
    'japa_stage.enabled',
    'forum.ama.enabled','ama_cooldown_days',
    'achievement.enabled','achievement_reactions',
    'service_directory.enabled','service_directory.subdomain','service_directory.custom_domain'
  );
