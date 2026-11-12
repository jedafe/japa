BEGIN;

DELETE FROM custom_fields WHERE group_id IN (
    SELECT id FROM custom_field_groups WHERE slug IN (
        'wiki_ai_agent_overview', 'wiki_ai_model_overview', 'wiki_cloud_overview',
        'wiki_hosting_overview', 'wiki_mobile_app_overview', 'wiki_saas_overview',
        'wiki_web_app_overview', 'wiki_website_overview'
    )
);

COMMIT;
