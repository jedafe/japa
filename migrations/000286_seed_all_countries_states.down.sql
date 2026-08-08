-- Reverse the 000286 full geo seed.
-- Removes only the countries and states that were NOT part of the original
-- 000100 seed (i.e. countries beyond the initial 11). The original 11
-- countries and their states are preserved so rolling back 000286 returns
-- the database to the post-000100 state.
--
-- The 11 original countries (from 000100): NG, GB, CA, US, IE, DE, NL, ZA, AE, GH, FR.

-- Remove parent relationships for states added by 000286
DELETE FROM space_parent_spaces
WHERE parent_space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level = 'country'
      AND geo_code NOT IN ('NG','GB','CA','US','IE','DE','NL','ZA','AE','GH','FR')
)
OR space_id IN (
    SELECT id FROM spaces
    WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
      AND level = 'state'
      AND slug NOT LIKE 'ng-%'
      AND slug NOT LIKE 'gb-%'
      AND slug NOT LIKE 'ca-%'
      AND slug NOT LIKE 'us-%'
      AND slug NOT LIKE 'ie-%'
      AND slug NOT LIKE 'de-%'
      AND slug NOT LIKE 'nl-%'
      AND slug NOT LIKE 'za-%'
      AND slug NOT LIKE 'ae-%'
      AND slug NOT LIKE 'gh-%'
      AND slug NOT LIKE 'fr-%'
);

-- Remove state-level spaces added by 000286 (not from the original 11 countries)
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'state'
  AND slug NOT LIKE 'ng-%'
  AND slug NOT LIKE 'gb-%'
  AND slug NOT LIKE 'ca-%'
  AND slug NOT LIKE 'us-%'
  AND slug NOT LIKE 'ie-%'
  AND slug NOT LIKE 'de-%'
  AND slug NOT LIKE 'nl-%'
  AND slug NOT LIKE 'za-%'
  AND slug NOT LIKE 'ae-%'
  AND slug NOT LIKE 'gh-%'
  AND slug NOT LIKE 'fr-%'
  AND creator_user_id IS NULL;

-- Remove country-level spaces added by 000286 (not the original 11)
DELETE FROM spaces
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND level = 'country'
  AND geo_code NOT IN ('NG','GB','CA','US','IE','DE','NL','ZA','AE','GH','FR')
  AND creator_user_id IS NULL;
