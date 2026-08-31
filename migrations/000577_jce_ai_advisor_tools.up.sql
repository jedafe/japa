-- 000577_jce_ai_advisor_tools.up.sql
-- JCE Sub-spec 4: seed AI advisor tools for comparisons.
-- Ref: docs/superpowers/specs/2026-08-30-jce-subspec-4-design.md

BEGIN;

INSERT INTO ai_tools (tenant_id, name, description, category, config, is_enabled, required_permission) VALUES
(
    '00000000-0000-0000-0000-000000000001', 'comparison.analyze',
    'Analyze a comparison against the user profile and return a ranked, explained answer with next actions.',
    'agent_comparison',
    '{
        "function_name": "comparison.analyze",
        "description": "Analyze a comparison and profile, return scores/explanations and next actions.",
        "parameters": {
            "type": "object",
            "properties": {
                "content_id": { "type": "string", "description": "Comparison content UUID" },
                "profile_id": { "type": "string", "description": "Optional comparison profile UUID" },
                "question":   { "type": "string", "description": "User question, e.g. Which is best for me?" }
            },
            "required": ["content_id"]
        },
        "handler": "JCEAnalyze",
        "reads_user_data": true,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true, 'jce.ai_advisor.use'
),
(
    '00000000-0000-0000-0000-000000000001', 'comparison.rank',
    'Rank comparison items with reasoning and confidence.',
    'agent_comparison',
    '{
        "function_name": "comparison.rank",
        "description": "Rank items in a comparison with reasoning.",
        "parameters": {
            "type": "object",
            "properties": {
                "content_id": { "type": "string" },
                "profile_id": { "type": "string" }
            },
            "required": ["content_id"]
        },
        "handler": "JCERank",
        "reads_user_data": true,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true, 'jce.ai_advisor.use'
),
(
    '00000000-0000-0000-0000-000000000001', 'comparison.explain',
    'Explain a single JapaScore factor or recommendation for an item.',
    'agent_comparison',
    '{
        "function_name": "comparison.explain",
        "description": "Explain a single score or recommendation.",
        "parameters": {
            "type": "object",
            "properties": {
                "content_id": { "type": "string" },
                "item_id":    { "type": "string" },
                "factor_key": { "type": "string" }
            },
            "required": ["content_id", "item_id"]
        },
        "handler": "JCEExplain",
        "reads_user_data": true,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true, 'jce.ai_advisor.use'
),
(
    '00000000-0000-0000-0000-000000000001', 'comparison.recommend_next_step',
    'Get the top next actions and recommendations for a comparison and profile.',
    'agent_comparison',
    '{
        "function_name": "comparison.recommend_next_step",
        "description": "Get the top next actions and recommendations for a comparison and profile.",
        "parameters": {
            "type": "object",
            "properties": {
                "content_id": { "type": "string" },
                "profile_id": { "type": "string" }
            },
            "required": ["content_id"]
        },
        "handler": "JCERecommend",
        "reads_user_data": true,
        "writes_user_data": false,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true, 'jce.ai_advisor.use'
),
(
    '00000000-0000-0000-0000-000000000001', 'comparison.refresh',
    'Trigger a refresh of stale comparison values from data sources.',
    'agent_comparison',
    '{
        "function_name": "comparison.refresh",
        "description": "Refresh stale comparison values from registered data sources.",
        "parameters": {
            "type": "object",
            "properties": {
                "content_id": { "type": "string" },
                "source_kind": { "type": "string", "description": "Optional filter: official|third_party|ai_estimate" }
            },
            "required": ["content_id"]
        },
        "handler": "JCERefresh",
        "reads_user_data": true,
        "writes_user_data": true,
        "requires_hitl": false,
        "is_blocked": false
    }'::jsonb,
    true, 'jce.refresh.trigger'
)
ON CONFLICT (tenant_id, name) DO UPDATE SET
    description = EXCLUDED.description,
    category = EXCLUDED.category,
    config = EXCLUDED.config,
    is_enabled = EXCLUDED.is_enabled,
    required_permission = EXCLUDED.required_permission,
    updated_at = now();

COMMIT;
