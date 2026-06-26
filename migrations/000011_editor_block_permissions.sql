-- Editor Block Permissions
-- Controls which block types each user group can insert into the content editor.
-- Enables granular access control for rich content blocks (embeds, AI tools, etc.)
-- without requiring separate RBAC permission nodes for every block type.

CREATE TABLE editor_block_permissions (
    block_type    text NOT NULL,
    user_group_id uuid NOT NULL REFERENCES user_groups(id) ON DELETE CASCADE,
    enabled       boolean NOT NULL DEFAULT true,
    PRIMARY KEY (block_type, user_group_id)
);

COMMENT ON TABLE editor_block_permissions IS 'Per-group permissions for editor block types. Row presence + enabled flag controls access.';
COMMENT ON COLUMN editor_block_permissions.block_type IS 'Editor block type identifier (e.g. paragraph, video_embed, ai_generate).';
COMMENT ON COLUMN editor_block_permissions.user_group_id IS 'FK to user_groups. Cascade-deletes when group is removed.';
COMMENT ON COLUMN editor_block_permissions.enabled IS 'Whether this group can use this block type. Allows disabling without deleting the row.';

-- Seed permissions for system default groups.
-- super_admin (00000000-0000-0000-0000-000000000010): all block types except ai_generate
INSERT INTO editor_block_permissions (block_type, user_group_id, enabled) VALUES
    ('paragraph',        '00000000-0000-0000-0000-000000000010', true),
    ('heading',          '00000000-0000-0000-0000-000000000010', true),
    ('image',            '00000000-0000-0000-0000-000000000010', true),
    ('divider',          '00000000-0000-0000-0000-000000000010', true),
    ('code_block',       '00000000-0000-0000-0000-000000000010', true),
    ('quote',            '00000000-0000-0000-0000-000000000010', true),
    ('list',             '00000000-0000-0000-0000-000000000010', true),
    ('table',            '00000000-0000-0000-0000-000000000010', true),
    ('video_embed',      '00000000-0000-0000-0000-000000000010', true),
    ('comparison_embed', '00000000-0000-0000-0000-000000000010', true),
    ('embed',            '00000000-0000-0000-0000-000000000010', true),
    ('ai_generate',      '00000000-0000-0000-0000-000000000010', false);

-- members (00000000-0000-0000-0000-000000000020): most blocks except ai_generate and embed
INSERT INTO editor_block_permissions (block_type, user_group_id, enabled) VALUES
    ('paragraph',        '00000000-0000-0000-0000-000000000020', true),
    ('heading',          '00000000-0000-0000-0000-000000000020', true),
    ('image',            '00000000-0000-0000-0000-000000000020', true),
    ('divider',          '00000000-0000-0000-0000-000000000020', true),
    ('code_block',       '00000000-0000-0000-0000-000000000020', true),
    ('quote',            '00000000-0000-0000-0000-000000000020', true),
    ('list',             '00000000-0000-0000-0000-000000000020', true),
    ('table',            '00000000-0000-0000-0000-000000000020', true),
    ('video_embed',      '00000000-0000-0000-0000-000000000020', true),
    ('comparison_embed', '00000000-0000-0000-0000-000000000020', true),
    ('embed',            '00000000-0000-0000-0000-000000000020', false),
    ('ai_generate',      '00000000-0000-0000-0000-000000000020', false);

-- guest (00000000-0000-0000-0000-000000000030): basic blocks only
INSERT INTO editor_block_permissions (block_type, user_group_id, enabled) VALUES
    ('paragraph',        '00000000-0000-0000-0000-000000000030', true),
    ('heading',          '00000000-0000-0000-0000-000000000030', true),
    ('image',            '00000000-0000-0000-0000-000000000030', true),
    ('divider',          '00000000-0000-0000-0000-000000000030', true),
    ('code_block',       '00000000-0000-0000-0000-000000000030', true),
    ('quote',            '00000000-0000-0000-0000-000000000030', false),
    ('list',             '00000000-0000-0000-0000-000000000030', false),
    ('table',            '00000000-0000-0000-0000-000000000030', false),
    ('video_embed',      '00000000-0000-0000-0000-000000000030', false),
    ('comparison_embed', '00000000-0000-0000-0000-000000000030', false),
    ('embed',            '00000000-0000-0000-0000-000000000030', false),
    ('ai_generate',      '00000000-0000-0000-0000-000000000030', false);
