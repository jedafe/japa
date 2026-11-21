-- 001220_eca_provider_global_coverage.down.sql
-- Removes only the provider rows 001220 seeded AND that are not
-- referenced by any eca_guides row — referenced providers stay so
-- existing guides remain resolvable.

BEGIN;

DELETE FROM eca_providers p
WHERE p.slug IN (
        'enic-albania','enic-andorra','armenic','enic-austria','enic-azerbaijan',
        'enic-belarus','naric-vlaanderen','enic-fwb','enic-ostbelgien','cip',
        'enic-bulgaria','cicic','enic-croatia','kysats','enic-czech',
        'enic-denmark','enic-estonia','oph-finland','nceqe','enic-greece',
        'enic-holy-see','enic-hungary','enic-iceland','enic-israel',
        'enic-kazakhstan','enic-kyrgyzstan','aic-latvia','aiba-liechtenstein',
        'skvc','enic-luxembourg','mqric','enic-moldova','enic-monaco',
        'enic-montenegro','enic-north-macedonia','nawa','dges','cnred',
        'enic-russia','enic-san-marino','enic-serbia','enic-slovakia',
        'enic-slovenia','enic-spain','enic-switzerland','enic-tajikistan',
        'enic-turkey','enic-ukraine','enic-uzbekistan','nic-japan','cscse',
        'saqa','knqa','aiu'
    )
  AND NOT EXISTS (SELECT 1 FROM eca_guides e WHERE e.provider_id = p.id);

COMMIT;
