-- Restore the 7 removed fields with their original ids/positions so
-- any external references keep working.

BEGIN;

INSERT INTO custom_fields (id, group_id, label, key, field_type, position, is_system, show_in_frontend, config)
VALUES
    ('7e62b998-aaf1-43a5-8fe8-0dd2e637c297', '37b7fd6e-a3be-4a23-9fab-fec3c1694880', 'Emergency Number', 'emergency_number', 'text', 2, false, true, '{}'),
    ('f0ddd701-f74f-4498-b66a-3c7960fc1620', '41ee7fef-6f00-4235-b7b4-6cc31fd63cf5', 'Minimum Wage (monthly)', 'minimum_wage', 'price_range', 2, false, true, '{}'),
    ('a598267f-72ca-41f6-b2e9-d35acdddd0b1', '48763376-4d90-4d5d-b99c-c521fd247527', 'Mobile Operators', 'mobile_operators', 'repeater', 5, false, true, '{}'),
    ('4b5ba1dd-7eae-4f6d-bd39-a9cdc76fbbdc', 'de0d26b4-3f05-47e0-961f-f8755a769b79', 'Official Language', 'official_language', 'text', 4, false, true, '{}'),
    ('51a37cd9-6e34-42a2-9927-7bd90f182c29', '9b4cbca5-24d7-4854-a795-8d49209db8b8', 'Currency', 'qf_currency', 'text', 20, false, true, '{}'),
    ('ed6968ff-3a10-4b91-aa9a-22822af7439a', 'd2aa980f-17ee-4b36-9841-24aff00a285b', 'Currency', 'qf_currency', 'text', 20, false, true, '{}'),
    ('2089bce0-dd71-4ebd-98b2-083df51ac256', 'b97a06c1-22f0-4b3a-9250-69982cc3e599', 'Currency', 'qf_currency', 'text', 40, false, true, '{}')
ON CONFLICT DO NOTHING;

COMMIT;
