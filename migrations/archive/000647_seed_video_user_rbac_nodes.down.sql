BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'video.create',
        'video.import.single',
        'video.import.bulk',
        'video.channel.claim'
    )
);

DELETE FROM permissions
WHERE key IN (
    'video.create',
    'video.import.single',
    'video.import.bulk',
    'video.channel.claim'
);

COMMIT;
