-- 000176_remove_dead_duplicate_permission_nodes.up.sql
-- AUDIT-FRESH-DEAD-NODES: removes permission nodes that are seeded but never
-- checked by any middleware, and have a wired equivalent that covers the same
-- operation. These are genuine duplicates — not future features.
--
-- Removed nodes and their wired equivalents:
--   blog.edit_any / blog.edit_own          → content.edit (blog posts are content rows)
--   content.delete.any / content.delete.own → content.delete
--   content.edit.any / content.edit.own     → content.edit
--   media.delete.any / media.delete.own     → media.delete_any / media.delete_own (underscore)
--   forum.thread.edit_any / .delete_any     → forum.thread.edit_own / .delete_own
--   forum.reply.edit_any / .delete_any      → forum.reply.edit_own / .delete_own
--   reviews.edit_any / .delete_any          → reviews.moderate
--   comparison.edit_any / .delete_any       → comparison.item.manage
--   professional_profile.edit_any / .delete_any → professional_profile.member.manage
--   payments.view_any                       → analytics.admin (admin payment visibility)
--   payments.view.own                       → payments.view_own (underscore variant)
--   blog.review                             → blog.review.create / blog.review.manage
--   content.page_builder.create/edit/delete → pagebuilder.blocks.manage / pages.manage

BEGIN;

DELETE FROM role_permissions WHERE permission_id IN (
    SELECT id FROM permissions WHERE key IN (
        'blog.edit_any',
        'blog.edit_own',
        'content.delete.any',
        'content.delete.own',
        'content.edit.any',
        'content.edit.own',
        'media.delete.any',
        'media.delete.own',
        'forum.thread.edit_any',
        'forum.thread.delete_any',
        'forum.reply.edit_any',
        'forum.reply.delete_any',
        'reviews.edit_any',
        'reviews.delete_any',
        'comparison.edit_any',
        'comparison.delete_any',
        'professional_profile.edit_any',
        'professional_profile.delete_any',
        'payments.view_any',
        'payments.view.own',
        'blog.review',
        'content.page_builder.create',
        'content.page_builder.edit',
        'content.page_builder.delete'
    )
);

DELETE FROM permissions WHERE key IN (
    'blog.edit_any',
    'blog.edit_own',
    'content.delete.any',
    'content.delete.own',
    'content.edit.any',
    'content.edit.own',
    'media.delete.any',
    'media.delete.own',
    'forum.thread.edit_any',
    'forum.thread.delete_any',
    'forum.reply.edit_any',
    'forum.reply.delete_any',
    'reviews.edit_any',
    'reviews.delete_any',
    'comparison.edit_any',
    'comparison.delete_any',
    'professional_profile.edit_any',
    'professional_profile.delete_any',
    'payments.view_any',
    'payments.view.own',
    'blog.review',
    'content.page_builder.create',
    'content.page_builder.edit',
    'content.page_builder.delete'
);

COMMIT;
