-- 000685_seed_intel_channel_settings.down.sql
-- Remove the intel-channels settings rows seeded by 000685. Code falls back
-- to the embedded/in-code defaults when a key is absent, so the down
-- migration does not change runtime behaviour.

BEGIN;

DELETE FROM system_settings
WHERE key IN (
    'intel.probe.max_requests',
    'intel.probe.verify_max_requests',
    'intel.probe.max_urls',
    'intel.probe.rate_limit_per_hr',
    'intel.category_method_map',
    'intel.curate.auto_publish_trust_max',
    'intel.relevance.on_topic_threshold',
    'intel.relevance.off_topic_threshold',
    'intel.relevance.borderline_rate_cap',
    'intel.relevance.classifier_enabled',
    'intel.relevance.classifier_model',
    'intel.consecutive_failure_disable_threshold',
    'intel.render.service_url',
    'intel.render.timeout_ms',
    'intel.search.provider',
    'intel.search.api_key',
    'intel.email.inbound_domain',
    'intel.email.allowed_senders',
    'intel.social.enabled_providers'
);

COMMIT;
