-- Restore the 4 removed fields with original ids/positions.

BEGIN;

INSERT INTO custom_fields (id, group_id, label, key, field_type, position, is_system, show_in_frontend, config)
VALUES
    ('fc30870d-9d17-4dcd-a329-5f8ae136c3a5', 'b97a06c1-22f0-4b3a-9250-69982cc3e599', 'PRESTO Card', 'ttc_presto_card', 'price_parts', 7, false, true, '{}'),
    ('d5055d46-236f-4102-9eb6-0a7d071535be', 'b97a06c1-22f0-4b3a-9250-69982cc3e599', 'Bike Share Toronto (Single Ride)', 'bike_single', 'price_parts', 30, false, true, '{}'),
    ('0c969efc-0bde-4271-ab8e-0e9b5f640fed', 'b97a06c1-22f0-4b3a-9250-69982cc3e599', 'Bike Share Toronto (24-Hour Pass)', 'bike_day_pass', 'price_parts', 31, false, true, '{}'),
    ('79901935-387a-46d3-a885-b5986b261ef2', 'b97a06c1-22f0-4b3a-9250-69982cc3e599', 'Electric Scooter (Bird/Lime)', 'escooter', 'price_parts', 32, false, true, '{}')
ON CONFLICT DO NOTHING;

COMMIT;
