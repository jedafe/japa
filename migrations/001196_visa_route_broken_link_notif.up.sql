-- 001196_visa_route_broken_link_notif
-- §90 Data Maintenance: the weekly visa_routes:check_links sweep must
-- produce an admin notification when a gov_source_url returns 404 (a
-- moved government page usually signals a rule change). The
-- notifications.notification_type FK requires the type to be seeded.

BEGIN;

INSERT INTO public.notification_types
    (key, label, description, enabled, default_in_app, default_email, default_push, category)
SELECT 'visa_route.broken_link', 'Visa Route Source Link Broken',
       'A visa route government source URL returned 404 during the weekly link check — possible rule change',
       true, true, false, false, 'system'
WHERE NOT EXISTS (
    SELECT 1 FROM public.notification_types WHERE key = 'visa_route.broken_link'
);

COMMIT;
