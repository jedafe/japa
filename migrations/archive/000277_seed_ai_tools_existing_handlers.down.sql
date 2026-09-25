-- 000277 down — remove the seeded AI tool rows for handlers that were
-- previously unregistered. Only deletes these four canonical names; any
-- admin-created tools are preserved.
DELETE FROM ai_tools
WHERE name IN (
    'content.generate_og',
    'content.summarize',
    'content.improve',
    'chat.japabot'
);
