-- Reverse: revert all fields back to 'text'
UPDATE custom_fields SET field_type = 'text' WHERE key IN (
    'processing_time', 'validity_period',
    'travel_duration', 'transit_hubs', 'recommended_airlines',
    'countries_eligible',
    'required_exams', 'language_requirements', 'acceptance_rate',
    'lease_duration',
    'international_transfer_fee',
    'in_demand_sectors', 'tax_rate',
    'language_spoken'
);
