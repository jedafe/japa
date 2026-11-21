-- 001219_eca_provider_expansion.down.sql
-- Removes only the provider rows 001219 seeded AND that are not
-- referenced by any eca_guides row — referenced providers stay so
-- existing guides remain resolvable.

BEGIN;

DELETE FROM eca_providers p
WHERE p.slug IN (
        'mcc','pebc','educational-perspectives','facs','fis',
        'transcript-research','ucredo','aes','tra','engineers-australia',
        'acs','anmac','cpaa','aitsl','cimea','nokut','uhr','fei','dataflow'
    )
  AND NOT EXISTS (SELECT 1 FROM eca_guides e WHERE e.provider_id = p.id);

COMMIT;
