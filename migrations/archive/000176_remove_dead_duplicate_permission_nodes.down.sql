-- 000176_remove_dead_duplicate_permission_nodes.down.sql
-- Reverts 000176: re-seeds the removed duplicate permission nodes.
-- Note: role_permissions grants are not restored; re-assign them manually if needed.

BEGIN;

INSERT INTO permissions (key, description) VALUES
    ('blog.edit_any', 'Edit any blog post (duplicate of content.edit)'),
    ('blog.edit_own', 'Edit own blog posts (duplicate of content.edit)'),
    ('content.delete.any', 'Delete any content (duplicate of content.delete)'),
    ('content.delete.own', 'Delete own content (duplicate of content.delete)'),
    ('content.edit.any', 'Edit any content (duplicate of content.edit)'),
    ('content.edit.own', 'Edit own content (duplicate of content.edit)'),
    ('media.delete.any', 'Delete any media (duplicate of media.delete_any)'),
    ('media.delete.own', 'Delete own media (duplicate of media.delete_own)'),
    ('forum.thread.edit_any', 'Edit any forum thread (duplicate of forum.thread.edit_own)'),
    ('forum.thread.delete_any', 'Delete any forum thread (duplicate of forum.thread.delete_own)'),
    ('forum.reply.edit_any', 'Edit any forum reply (duplicate of forum.reply.edit_own)'),
    ('forum.reply.delete_any', 'Delete any forum reply (duplicate of forum.reply.delete_own)'),
    ('reviews.edit_any', 'Edit any review (duplicate of reviews.moderate)'),
    ('reviews.delete_any', 'Delete any review (duplicate of reviews.moderate)'),
    ('comparison.edit_any', 'Edit any comparison (duplicate of comparison.item.manage)'),
    ('comparison.delete_any', 'Delete any comparison (duplicate of comparison.item.manage)'),
    ('professional_profile.edit_any', 'Edit any professional profile (duplicate of professional_profile.member.manage)'),
    ('professional_profile.delete_any', 'Delete any professional profile (duplicate of professional_profile.member.manage)'),
    ('payments.view_any', 'View any payment (duplicate of analytics.admin)'),
    ('payments.view.own', 'View own payment history (duplicate of payments.view_own)'),
    ('blog.review', 'Blog review (duplicate of blog.review.create/manage)'),
    ('content.page_builder.create', 'Create page builder content (duplicate of pagebuilder.blocks.manage)'),
    ('content.page_builder.edit', 'Edit page builder content (duplicate of pagebuilder.blocks.manage)'),
    ('content.page_builder.delete', 'Delete page builder content (duplicate of pagebuilder.blocks.manage)')
ON CONFLICT (key) DO NOTHING;

COMMIT;
