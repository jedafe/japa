# Section 22 — Admin Panel

**Status:** ✅ Locked

## Decided
University CRUD, Space type + custom field management. User management: tiered warning system (admin-configurable accumulation threshold), tiered bans (time-boxed or permanent, with immediate session invalidation), appeals (`appeals` table, gated by `moderation.appeals.review`, every resolution logged to Audit Log). Unified content moderation queue combining user reports and AI moderation flags, filterable, with bulk actions. Community-health analytics dashboard (signups, DAU/MAU, content volume, top spaces, geo distribution, moderation stats, search trends) — distinct from Control Panel's infrastructure-health dashboard. Notification broadcast (`broadcast_notifications`: scoped targeting, optional scheduling, batched fan-out for large audiences to avoid overwhelming the queue/WS layer in one shot).

## Amendment — Per-User AI Context Panel (post-audit)

A per-user panel in the existing user management area shows what Honcho has actually stored/derived for that user — read-only, plus the ability to delete an individual entry, but no editing (correcting a wrong AI inference by hand-editing memory invites trust issues; deleting and letting it re-derive is the safer mechanism). Gated by new permission nodes `ai.context.admin_view` and `ai.context.admin_delete_entry`, distinct from the `ai.context.*` category nodes in Section 03 that gate what feeds the AI in the first place. Intended use: support tickets and trust & safety review. See ADR-027.
