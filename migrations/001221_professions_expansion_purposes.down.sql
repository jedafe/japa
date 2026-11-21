-- 001221_professions_expansion_purposes.down.sql
-- Removes only the profession rows this migration seeded AND that are
-- not referenced by any credential_pathways row, and restores the
-- original 4-value purpose select options.

BEGIN;

DELETE FROM credential_professions pr
WHERE pr.slug IN (
        'licensed-practical-nurse','nurse-practitioner','dental-hygienist',
        'dental-technician','pharmacy-technician','respiratory-therapist',
        'audiologist','podiatrist','chiropractor','osteopath',
        'dispensing-optician','physician-associate','mental-health-counsellor',
        'prosthetist-orthotist','veterinary-technician',
        'software-engineer','computer-engineer','mining-engineer',
        'environmental-engineer','industrial-engineer','aerospace-engineer',
        'biomedical-engineer','marine-engineer','metallurgical-engineer',
        'agricultural-engineer','engineering-technologist',
        'patent-attorney','conveyancer','immigration-consultant',
        'legal-executive',
        'statutory-auditor','financial-adviser','insurance-agent-broker',
        'mortgage-broker','tax-adviser',
        'vocational-teacher','school-counsellor','school-psychologist',
        'carpenter','hvac-technician','elevator-mechanic','millwright',
        'sheet-metal-worker','ironworker','boilermaker','automotive-technician',
        'aircraft-maintenance-engineer','marine-officer','train-driver',
        'truck-driver','heavy-equipment-operator','crane-operator',
        'hairdresser-barber','cosmetologist','chef','baker','butcher',
        'real-estate-agent','customs-broker','sworn-translator',
        'security-guard','building-inspector'
    )
  AND NOT EXISTS (SELECT 1 FROM credential_pathways p WHERE p.profession_id = pr.id);

UPDATE custom_fields f
SET options = '[
  {"value":"immigration","label":"Immigration"},
  {"value":"study","label":"Study"},
  {"value":"employment","label":"Employment"},
  {"value":"professional_licensure","label":"Professional licensure"}
]'::jsonb
FROM custom_field_groups g
WHERE f.group_id = g.id
  AND g.slug = 'guide_eca_fields'
  AND f.key = 'purpose';

COMMIT;
