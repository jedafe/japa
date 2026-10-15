-- ADR-130 rollout complete: the unified engine serves POST /ai/chat
-- unconditionally, so the feature.japa_ai_unified reachability flag is
-- retired. feature.japabot remains the surface on/off flag and
-- feature.ai_agent remains the tool-executor kill-switch.
DELETE FROM feature_flags WHERE key = 'feature.japa_ai_unified';
