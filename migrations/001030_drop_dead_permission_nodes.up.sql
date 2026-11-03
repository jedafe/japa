-- 001030_drop_dead_permission_nodes.up.sql
-- Removes 60 permission nodes that are seeded but enforced by no route,
-- service, or frontend permission check. They fall into four classes:
--
--   1. Removed features — the §33 professional-profile model was dropped in
--      favour of the unified user-tier model (professional.*,
--      professional_profile.*), and `guide` is a contents row type gated by
--      the generic content.* nodes, not a separate entity (guide.*).
--   2. Superseded duplicates — nodes whose live gate uses a different key:
--      control.* (replaced by platform.*/api.keys.*/audit.logs.*/ai.budget.*/
--      ai.providers.*), media.library.view_all -> media.view_all,
--      media.view_own -> media.library.view_own, follows.create ->
--      follows.toggle, reactions.* -> content.reactions.*, forum.solve ->
--      forum.thread.solve, newsletter.admin -> newsletter.manage,
--      referral.admin -> referral.manage, admin.i18n.manage -> i18n.manage,
--      admin.tickets.manage -> tickets.admin, academy.manage ->
--      academy.admin.manage, moderation.broadcast -> admin.broadcast,
--      affiliate.enroll -> affiliate.program.enroll,
--      wallet.withdraw.manage is spec-kept (JEOS §) and NOT removed here.
--   3. Undocumented "view/own" nodes for public or RequireAuth-only surfaces:
--      *.view nodes with default grant "all" that JEOS never catalogued
--      (analytics.view.*, tax_calculator.view, sop_generator.view,
--      remittance.view is spec'd and kept, reviews.aggregate.view,
--      visa_routes.view, payments.view_own, notifications.view,
--      content.scheduled.view, content.scores.view, jobs.browse is spec'd).
--   4. Internal-only actions with no callable surface: ad.event.track
--      (public beacon), payments.webhook (signature-verified callbacks),
--      wallet.credits.balance/spend, wallet.points.earn, content.render,
--      academy.enroll/view/certificate.issue/progress.update,
--      organizations.manage_types.
--
-- Nodes documented in the JEOS permissions catalog but currently unenforced
-- (studio.review, appointment_alert.manage_any, participation.manage,
-- remittance.view, jobs.browse, wallet.payout_provider.manage,
-- wallet.withdraw.manage, the *.view "all" grants, feature.* audience nodes,
-- ai.context.* opt-in categories, agent.content_type.*) are intentionally
-- retained — the spec is the source of truth.
--
-- Deleting the permission rows cascades to role_permissions (FK CASCADE),
-- removing the stale grants in the same step.

BEGIN;

DELETE FROM permissions
WHERE key IN (
    'academy.certificate.issue',
    'academy.enroll',
    'academy.manage',
    'academy.progress.update',
    'academy.view',
    'ad.event.track',
    'admin.i18n.manage',
    'admin.tickets.manage',
    'affiliate.enroll',
    'analytics.view.all',
    'analytics.view.own',
    'content.render',
    'content.scheduled.view',
    'content.scores.view',
    'control.ai_budget.manage',
    'control.api_keys.manage',
    'control.audit_logs.view',
    'control.cache.manage',
    'control.feature_flags.manage',
    'control.health.view',
    'control.queue.manage',
    'control.security.manage',
    'follows.create',
    'forum.admin',
    'forum.solve',
    'guide.create',
    'guide.delete_any',
    'guide.delete_own',
    'guide.edit_any',
    'guide.edit_own',
    'guide.feature',
    'guide.publish',
    'media.library.view_all',
    'media.view_own',
    'moderation.broadcast',
    'newsletter.admin',
    'notifications.view',
    'organizations.manage_types',
    'payments.view_own',
    'payments.webhook',
    'professional.create',
    'professional.manage',
    'professional_profile.author.switch',
    'professional_profile.create',
    'professional_profile.delete_own',
    'professional_profile.edit_own',
    'professional_profile.invite',
    'professional_profile.member.manage',
    'professional_profile.type.create',
    'professional_profile.type.view',
    'reactions.create',
    'reactions.delete.own',
    'referral.admin',
    'reviews.aggregate.view',
    'sop_generator.view',
    'tax_calculator.view',
    'visa_routes.view',
    'wallet.credits.balance',
    'wallet.credits.spend',
    'wallet.points.earn'
);

COMMIT;
