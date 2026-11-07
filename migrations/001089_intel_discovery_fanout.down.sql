-- 001089 down — remove the intel discovery fan-out seeds.

BEGIN;

DELETE FROM system_settings WHERE key = 'intel.discovery_fanout.max_recipients';
DELETE FROM notification_types WHERE key = 'intel.discovery';

COMMIT;
