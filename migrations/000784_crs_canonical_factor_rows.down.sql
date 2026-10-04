-- 000784_crs_canonical_factor_rows.down.sql
-- Revert 000784: remove the canonical factor rows added to the
-- "IRCC 2025-01 rules" ruleset and restore the seeded rows it touched.

BEGIN;

DELETE FROM crs_factor_points
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND (factor_key, input_descriptor) IN (
    VALUES
    ('age', '{"min_age": 18, "max_age": 18}'::jsonb),
    ('age', '{"min_age": 19, "max_age": 19}'::jsonb),
    ('age', '{"min_age": 30, "max_age": 30}'::jsonb),
    ('age', '{"min_age": 31, "max_age": 31}'::jsonb),
    ('age', '{"min_age": 32, "max_age": 32}'::jsonb),
    ('age', '{"min_age": 33, "max_age": 33}'::jsonb),
    ('age', '{"min_age": 34, "max_age": 34}'::jsonb),
    ('age', '{"min_age": 35, "max_age": 35}'::jsonb),
    ('age', '{"min_age": 36, "max_age": 36}'::jsonb),
    ('age', '{"min_age": 37, "max_age": 37}'::jsonb),
    ('age', '{"min_age": 38, "max_age": 38}'::jsonb),
    ('age', '{"min_age": 39, "max_age": 39}'::jsonb),
    ('age', '{"min_age": 40, "max_age": 40}'::jsonb),
    ('age', '{"min_age": 41, "max_age": 41}'::jsonb),
    ('age', '{"min_age": 42, "max_age": 42}'::jsonb),
    ('age', '{"min_age": 43, "max_age": 43}'::jsonb),
    ('age', '{"min_age": 44, "max_age": 44}'::jsonb),
    ('age', '{"min_age": 45, "max_age": 120}'::jsonb),
    ('education', '{"edu": "none"}'::jsonb),
    ('education', '{"edu": "secondary"}'::jsonb),
    ('education', '{"edu": "one_year_post"}'::jsonb),
    ('education', '{"edu": "two_year_post"}'::jsonb),
    ('education', '{"edu": "two_or_more"}'::jsonb),
    ('education', '{"edu": "professional"}'::jsonb),
    ('education', '{"edu": "phd"}'::jsonb),
    ('language_first', '{"clb": 4}'::jsonb),
    ('language_first', '{"clb": 5}'::jsonb),
    ('language_first', '{"clb": 6}'::jsonb),
    ('language_first', '{"clb": 7}'::jsonb),
    ('language_first', '{"clb": 8}'::jsonb),
    ('language_second', '{"clb": 5}'::jsonb),
    ('language_second', '{"clb": 6}'::jsonb),
    ('language_second', '{"clb": 7}'::jsonb),
    ('language_second', '{"clb": 8}'::jsonb),
    ('language_second', '{"clb": 9}'::jsonb),
    ('language_second', '{"clb": 10}'::jsonb),
    ('canadian_experience', '{"min_years": 2, "max_years": 2}'::jsonb),
    ('canadian_experience', '{"min_years": 3, "max_years": 3}'::jsonb),
    ('canadian_experience', '{"min_years": 4, "max_years": 4}'::jsonb),
    ('canadian_experience', '{"min_years": 5, "max_years": 60}'::jsonb),
    ('spouse_education', '{"edu": "none"}'::jsonb),
    ('spouse_education', '{"edu": "secondary"}'::jsonb),
    ('spouse_education', '{"edu": "one_year_post"}'::jsonb),
    ('spouse_education', '{"edu": "two_year_post"}'::jsonb),
    ('spouse_education', '{"edu": "bachelors"}'::jsonb),
    ('spouse_education', '{"edu": "two_or_more"}'::jsonb),
    ('spouse_education', '{"edu": "masters"}'::jsonb),
    ('spouse_education', '{"edu": "professional"}'::jsonb),
    ('spouse_education', '{"edu": "phd"}'::jsonb),
    ('spouse_language', '{"clb": 5}'::jsonb),
    ('spouse_language', '{"clb": 6}'::jsonb),
    ('spouse_language', '{"clb": 7}'::jsonb),
    ('spouse_language', '{"clb": 8}'::jsonb),
    ('spouse_language', '{"clb": 9}'::jsonb),
    ('spouse_language', '{"clb": 10}'::jsonb),
    ('spouse_canadian_experience', '{"min_years": 1, "max_years": 1}'::jsonb),
    ('spouse_canadian_experience', '{"min_years": 2, "max_years": 2}'::jsonb),
    ('spouse_canadian_experience', '{"min_years": 3, "max_years": 3}'::jsonb),
    ('spouse_canadian_experience', '{"min_years": 4, "max_years": 4}'::jsonb),
    ('spouse_canadian_experience', '{"min_years": 5, "max_years": 60}'::jsonb),
    ('edu_language', '{"combo": "edu_lang", "edu_min": "one_year_post", "clb_min": 7}'::jsonb),
    ('edu_language', '{"combo": "edu_lang", "edu_min": "one_year_post", "clb_min": 9}'::jsonb),
    ('edu_language', '{"combo": "edu_lang", "edu_min": "two_or_more", "clb_min": 7}'::jsonb),
    ('edu_work', '{"combo": "edu_canadian", "edu_min": "one_year_post", "canadian_min": 1}'::jsonb),
    ('edu_work', '{"combo": "edu_canadian", "edu_min": "one_year_post", "canadian_min": 2}'::jsonb),
    ('edu_work', '{"combo": "edu_canadian", "edu_min": "two_or_more", "canadian_min": 1}'::jsonb),
    ('edu_work', '{"combo": "edu_canadian", "edu_min": "two_or_more", "canadian_min": 2}'::jsonb),
    ('foreign_language', '{"combo": "foreign_lang", "foreign_min": 1, "clb_min": 7}'::jsonb),
    ('foreign_language', '{"combo": "foreign_lang", "foreign_min": 1, "clb_min": 9}'::jsonb),
    ('foreign_language', '{"combo": "foreign_lang", "foreign_min": 3, "clb_min": 7}'::jsonb),
    ('foreign_language', '{"combo": "foreign_lang", "foreign_min": 3, "clb_min": 9}'::jsonb),
    ('foreign_work', '{"combo": "foreign_canadian", "foreign_min": 1, "canadian_min": 1}'::jsonb),
    ('foreign_work', '{"combo": "foreign_canadian", "foreign_min": 1, "canadian_min": 2}'::jsonb),
    ('foreign_work', '{"combo": "foreign_canadian", "foreign_min": 3, "canadian_min": 1}'::jsonb),
    ('foreign_work', '{"combo": "foreign_canadian", "foreign_min": 3, "canadian_min": 2}'::jsonb),
    ('certificate_language', '{"combo": "cert_lang", "cert": true, "clb_min": 5}'::jsonb),
    ('certificate_language', '{"combo": "cert_lang", "cert": true, "clb_min": 7}'::jsonb),
    ('french', '{"clb_min": 7}'::jsonb),
    ('french', '{"clb_min": 7, "english_min": 5}'::jsonb),
    ('canadian_education', '{"edu": "one_or_two_years"}'::jsonb),
    ('canadian_education', '{"edu": "three_plus_years"}'::jsonb),
    ('job_offer', '{"flag": true}'::jsonb)
  );

-- Restore the seeded 30-39 age band, edu_language descriptor, and
-- education sort orders.
INSERT INTO crs_factor_points
    (ruleset_id, factor_group, factor_key, input_descriptor, label, points_single, points_with_spouse, sort_order)
SELECT id, 'core', 'age', '{"max_age": 39, "min_age": 30}'::jsonb, 'Age 30-39', 95, 90, 2
FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules'
ON CONFLICT (ruleset_id, factor_group, factor_key, input_descriptor) DO NOTHING;


UPDATE crs_factor_points
SET input_descriptor = '{"combo": "edu_lang", "clb_min": 9, "edu_min": "bachelors"}'::jsonb
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'edu_language'
  AND input_descriptor = '{"combo": "edu_lang", "clb_min": 9, "edu_min": "two_or_more"}'::jsonb;

UPDATE crs_factor_points SET sort_order = 3
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'education' AND input_descriptor = '{"edu": "bachelors"}'::jsonb;
UPDATE crs_factor_points SET sort_order = 4
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'education' AND input_descriptor = '{"edu": "masters"}'::jsonb;

COMMIT;
