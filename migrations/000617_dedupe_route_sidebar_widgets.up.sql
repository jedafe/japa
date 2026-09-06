-- 000617_dedupe_route_sidebar_widgets.up.sql
-- Remove duplicate widget rows that accumulated on the route-specific Main
-- right sidebars. Migrations 000221–000224 each seeded widget sets with a
-- NOT EXISTS guard keyed on (widget_type, sort_order), so re-seeding after
-- the 000223 sort_order shift produced literal duplicates — e.g.
-- main-tag-right (/tags/*) carries ad_slot x3 and trending_tags x2, which
-- renders three ad slots and two identical "Trending Tags" widgets on every
-- tag hub page.
--
-- A row is a duplicate when another row in the same sidebar has the same
-- (widget_type, label, props); the lowest sort_order row is kept.
-- 'Ad Top' and 'Ad Bottom' ad_slot rows are intentionally distinct (different
-- label + props) and are both preserved.
--
-- Ref: Section 59 (Sidebar Management System), ADR-066.

BEGIN;

DELETE FROM sidebar_widgets a
USING sidebar_widgets b
WHERE a.sidebar_id = b.sidebar_id
  AND a.widget_type = b.widget_type
  AND a.label = b.label
  AND a.props = b.props
  AND a.tenant_id = b.tenant_id
  AND (b.sort_order, b.created_at, b.id) < (a.sort_order, a.created_at, a.id);

COMMIT;
