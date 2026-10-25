BEGIN;
DELETE FROM public.system_settings WHERE key IN ('appearance.logo_width','appearance.logo_height','appearance.logo_dark_width','appearance.logo_dark_height');
COMMIT;
