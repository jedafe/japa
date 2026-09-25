-- 000278 down — remove the moderation.detect_spam tool from the registry.
DELETE FROM ai_tools WHERE name = 'moderation.detect_spam';
