-- Backfill geo_code for university spaces seeded by 000101.
-- The original seed inserted NULL for geo_code on all 40 universities even
-- though they were grouped by country. This sets the correct ISO 3166-1
-- alpha-2 code per university so the /universities country filter (Section 39)
-- and geo-aware default (ADR-064) work.
--
-- Ref: japaconnect-engineering/sections/39-universities.md
--      migrations/000101_seed_universities.up.sql (fixed in place for fresh installs)

UPDATE spaces SET geo_code = 'GB'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('university-of-oxford', 'university-of-cambridge', 'imperial-college-london',
               'ucl', 'university-of-edinburgh', 'university-of-manchester',
               'kings-college-london', 'london-school-of-economics',
               'university-of-bristol', 'university-of-warwick');

UPDATE spaces SET geo_code = 'CA'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('university-of-toronto', 'mcgill-university', 'university-of-british-columbia',
               'university-of-alberta', 'mcmaster-university', 'university-of-waterloo');

UPDATE spaces SET geo_code = 'US'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('massachusetts-institute-of-technology', 'stanford-university', 'harvard-university',
               'california-institute-of-technology', 'university-of-california-berkeley',
               'columbia-university', 'university-of-chicago', 'yale-university',
               'princeton-university');

UPDATE spaces SET geo_code = 'IE'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('trinity-college-dublin', 'university-college-dublin');

UPDATE spaces SET geo_code = 'DE'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('ludwig-maximilian-university-of-munich', 'technical-university-of-munich',
               'heidelberg-university', 'humboldt-university-of-berlin');

UPDATE spaces SET geo_code = 'NL'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('university-of-amsterdam', 'delft-university-of-technology', 'leiden-university');

UPDATE spaces SET geo_code = 'ZA'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('university-of-cape-town');

UPDATE spaces SET geo_code = 'NG'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND space_type_id = (SELECT id FROM space_types WHERE tenant_id = '00000000-0000-0000-0000-000000000001' AND slug = 'university')
  AND slug IN ('university-of-lagos', 'university-of-ibadan', 'obafemi-awolowo-university',
               'ahmadu-bello-university', 'university-of-nigeria-nsukka');
