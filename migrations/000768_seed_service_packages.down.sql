-- 000768_seed_service_packages.down.sql
BEGIN;

DELETE FROM notification_types WHERE key IN (
    'service_booking_new', 'service_booking_confirmed', 'service_booking_declined',
    'service_booking_fulfilled', 'service_booking_cancelled', 'service_booking_reminder'
);

DELETE FROM system_settings WHERE key IN (
    'service_packages.fee_enabled', 'service_packages.fee_type',
    'service_packages.fee_pct', 'service_packages.fee_fixed',
    'service_packages.max_items', 'service_packages.require_verified_org',
    'service_packages.review_before_publish', 'service_packages.org_response_sla_hours',
    'service_packages.refund_window_days'
);

DELETE FROM role_permissions rp
USING permissions p
WHERE rp.permission_id = p.id
  AND p.key IN ('service.package.create', 'service.booking.create',
                'service.booking.manage', 'service.booking.admin');

DELETE FROM permissions WHERE key IN (
    'service.package.create', 'service.booking.create',
    'service.booking.manage', 'service.booking.admin'
);

COMMIT;
