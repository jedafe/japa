-- 000815_ad_widget_canonical_slots.down.sql

UPDATE public.sidebar_widgets
SET props = jsonb_set(props, '{slot}', '"sidebar_top"'::jsonb)
WHERE widget_type = 'ad_slot' AND props->>'slot' = 'sidebar';
