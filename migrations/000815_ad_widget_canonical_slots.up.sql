-- 000815_ad_widget_canonical_slots.up.sql — §25 amendment.
-- Canonicalize the ad_slot sidebar widget default slot name:
-- the seeded slot is "sidebar"; the widget used to default to "sidebar_top".

UPDATE public.sidebar_widgets
SET props = jsonb_set(props, '{slot}', '"sidebar"'::jsonb)
WHERE widget_type = 'ad_slot' AND props->>'slot' = 'sidebar_top';
