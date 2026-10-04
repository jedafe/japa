-- 000784_crs_canonical_factor_rows.up.sql
-- Complete the seeded "IRCC 2025-01 rules" ruleset with the canonical CRS
-- factor grid. The baseline seed shipped a sparse demo table (10 rows);
-- several factor keys the scoring engine implements had no rows at all
-- (language_second, spouse factors, french, canadian_education, job_offer,
-- and all transferability combos except one), so those inputs silently
-- scored zero. Point values follow the published IRCC CRS criteria.
--
-- Also corrects the seeded edu_language row: {"edu_min":"bachelors"} paid
-- the 50-point tier, which canonically requires two_or_more (or master's+);
-- and re-sorts education rows so the derived dropdown order is canonical.
--
-- Ref: .jeos/sections/107-crs-calculator.md
--      internal/crscalculator/service.go (descriptor semantics)

BEGIN;

-- Remove the seeded 30-39 age band: it overlaps the canonical per-year rows
-- (a 30-year-old would score both rows) and its flat 95 points is not the
-- canonical per-year value.
DELETE FROM crs_factor_points
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'age'
  AND input_descriptor = '{"max_age": 39, "min_age": 30}'::jsonb;

-- Canonicalize the seeded edu_language row (50-tier needs two_or_more+).
UPDATE crs_factor_points
SET input_descriptor = '{"combo": "edu_lang", "clb_min": 9, "edu_min": "two_or_more"}'::jsonb,
    sort_order = 4
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'edu_language'
  AND input_descriptor = '{"combo": "edu_lang", "clb_min": 9, "edu_min": "bachelors"}'::jsonb;

-- Re-sort seeded education rows into canonical dropdown order.
UPDATE crs_factor_points SET sort_order = 5
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'education' AND input_descriptor = '{"edu": "bachelors"}'::jsonb;
UPDATE crs_factor_points SET sort_order = 7
WHERE ruleset_id = (SELECT id FROM crs_rulesets WHERE version_label = 'IRCC 2025-01 rules')
  AND factor_key = 'education' AND input_descriptor = '{"edu": "masters"}'::jsonb;

INSERT INTO crs_factor_points
    (ruleset_id, factor_group, factor_key, input_descriptor, label, points_single, points_with_spouse, sort_order)
SELECT rs.id, v.factor_group, v.factor_key, v.input_descriptor::jsonb, v.label, v.points_single, v.points_with_spouse, v.sort_order
FROM crs_rulesets rs
JOIN (VALUES
    -- core — age bands (single / with spouse)
    ('core', 'age', '{"min_age": 18, "max_age": 18}', 'Age 18',         99,  90,  101),
    ('core', 'age', '{"min_age": 19, "max_age": 19}', 'Age 19',        105,  95,  102),
    ('core', 'age', '{"min_age": 30, "max_age": 30}', 'Age 30',        105,  95,  103),
    ('core', 'age', '{"min_age": 31, "max_age": 31}', 'Age 31',         99,  90,  104),
    ('core', 'age', '{"min_age": 32, "max_age": 32}', 'Age 32',         96,  88,  105),
    ('core', 'age', '{"min_age": 33, "max_age": 33}', 'Age 33',         93,  85,  106),
    ('core', 'age', '{"min_age": 34, "max_age": 34}', 'Age 34',         90,  81,  107),
    ('core', 'age', '{"min_age": 35, "max_age": 35}', 'Age 35',         84,  77,  108),
    ('core', 'age', '{"min_age": 36, "max_age": 36}', 'Age 36',         78,  72,  109),
    ('core', 'age', '{"min_age": 37, "max_age": 37}', 'Age 37',         72,  66,  110),
    ('core', 'age', '{"min_age": 38, "max_age": 38}', 'Age 38',         66,  61,  111),
    ('core', 'age', '{"min_age": 39, "max_age": 39}', 'Age 39',         60,  55,  112),
    ('core', 'age', '{"min_age": 40, "max_age": 40}', 'Age 40',         54,  50,  113),
    ('core', 'age', '{"min_age": 41, "max_age": 41}', 'Age 41',         50,  38,  114),
    ('core', 'age', '{"min_age": 42, "max_age": 42}', 'Age 42',         46,  34,  115),
    ('core', 'age', '{"min_age": 43, "max_age": 43}', 'Age 43',         42,  30,  116),
    ('core', 'age', '{"min_age": 44, "max_age": 44}', 'Age 44',         38,  26,  117),
    ('core', 'age', '{"min_age": 45, "max_age": 120}','Age 45+',         0,   0,  118),

    -- core — education (feeds the dropdown via factor rows)
    ('core', 'education', '{"edu": "none"}',           'No post-secondary credential',        0,   0,  1),
    ('core', 'education', '{"edu": "secondary"}',      'Secondary (high school) diploma',    30,  28,  2),
    ('core', 'education', '{"edu": "one_year_post"}',  'One-year post-secondary program',    90,  84,  3),
    ('core', 'education', '{"edu": "two_year_post"}',  'Two-year post-secondary program',    98,  91,  4),
    ('core', 'education', '{"edu": "two_or_more"}',    'Two or more post-secondary credentials', 128, 119, 6),
    ('core', 'education', '{"edu": "professional"}',   'Professional degree',               140, 131,  8),
    ('core', 'education', '{"edu": "phd"}',            'Doctoral degree (PhD)',             150, 140,  9),

    -- core — first official language, per skill
    ('core', 'language_first', '{"clb": 4}', 'CLB 4',  6,  6, 1),
    ('core', 'language_first', '{"clb": 5}', 'CLB 5',  6,  6, 2),
    ('core', 'language_first', '{"clb": 6}', 'CLB 6',  9,  8, 3),
    ('core', 'language_first', '{"clb": 7}', 'CLB 7', 17, 16, 4),
    ('core', 'language_first', '{"clb": 8}', 'CLB 8', 23, 22, 5),

    -- core — second official language, per skill (24/22 combined cap in code)
    ('core', 'language_second', '{"clb": 5}', '2nd lang CLB 5', 1, 1, 1),
    ('core', 'language_second', '{"clb": 6}', '2nd lang CLB 6', 1, 1, 2),
    ('core', 'language_second', '{"clb": 7}', '2nd lang CLB 7', 3, 3, 3),
    ('core', 'language_second', '{"clb": 8}', '2nd lang CLB 8', 3, 3, 4),
    ('core', 'language_second', '{"clb": 9}', '2nd lang CLB 9', 6, 6, 5),
    ('core', 'language_second', '{"clb": 10}','2nd lang CLB 10',6, 6, 6),

    -- core — Canadian work experience bands (1yr is seeded)
    ('core', 'canadian_experience', '{"min_years": 2, "max_years": 2}', '2yr CA exp', 53, 46, 8),
    ('core', 'canadian_experience', '{"min_years": 3, "max_years": 3}', '3yr CA exp', 64, 56, 9),
    ('core', 'canadian_experience', '{"min_years": 4, "max_years": 4}', '4yr CA exp', 72, 63, 10),
    ('core', 'canadian_experience', '{"min_years": 5, "max_years": 60}','5yr+ CA exp',80, 70, 11),

    -- spouse — education
    ('spouse', 'spouse_education', '{"edu": "none"}',          'Spouse: no credential',      0, NULL, 1),
    ('spouse', 'spouse_education', '{"edu": "secondary"}',     'Spouse: secondary diploma',  2, NULL, 2),
    ('spouse', 'spouse_education', '{"edu": "one_year_post"}', 'Spouse: 1yr post-secondary', 6, NULL, 3),
    ('spouse', 'spouse_education', '{"edu": "two_year_post"}', 'Spouse: 2yr post-secondary', 7, NULL, 4),
    ('spouse', 'spouse_education', '{"edu": "bachelors"}',     'Spouse: bachelor''s',        8, NULL, 5),
    ('spouse', 'spouse_education', '{"edu": "two_or_more"}',   'Spouse: 2+ credentials',     9, NULL, 6),
    ('spouse', 'spouse_education', '{"edu": "masters"}',       'Spouse: master''s',         10, NULL, 7),
    ('spouse', 'spouse_education', '{"edu": "professional"}',  'Spouse: professional',      10, NULL, 8),
    ('spouse', 'spouse_education', '{"edu": "phd"}',           'Spouse: PhD',               10, NULL, 9),

    -- spouse — first language, per skill
    ('spouse', 'spouse_language', '{"clb": 5}', 'Spouse CLB 5', 1, NULL, 1),
    ('spouse', 'spouse_language', '{"clb": 6}', 'Spouse CLB 6', 1, NULL, 2),
    ('spouse', 'spouse_language', '{"clb": 7}', 'Spouse CLB 7', 3, NULL, 3),
    ('spouse', 'spouse_language', '{"clb": 8}', 'Spouse CLB 8', 3, NULL, 4),
    ('spouse', 'spouse_language', '{"clb": 9}', 'Spouse CLB 9', 5, NULL, 5),
    ('spouse', 'spouse_language', '{"clb": 10}','Spouse CLB 10',5, NULL, 6),

    -- spouse — Canadian work experience
    ('spouse', 'spouse_canadian_experience', '{"min_years": 1, "max_years": 1}', 'Spouse 1yr CA exp',  5, NULL, 1),
    ('spouse', 'spouse_canadian_experience', '{"min_years": 2, "max_years": 2}', 'Spouse 2yr CA exp',  7, NULL, 2),
    ('spouse', 'spouse_canadian_experience', '{"min_years": 3, "max_years": 3}', 'Spouse 3yr CA exp',  8, NULL, 3),
    ('spouse', 'spouse_canadian_experience', '{"min_years": 4, "max_years": 4}', 'Spouse 4yr CA exp',  9, NULL, 4),
    ('spouse', 'spouse_canadian_experience', '{"min_years": 5, "max_years": 60}','Spouse 5yr+ CA exp',10, NULL, 5),

    -- transferability — education + language (50-tier seeded above)
    ('transferability', 'edu_language', '{"combo": "edu_lang", "edu_min": "one_year_post", "clb_min": 7}', 'Edu + CLB 7',        13, NULL, 1),
    ('transferability', 'edu_language', '{"combo": "edu_lang", "edu_min": "one_year_post", "clb_min": 9}', 'Edu + CLB 9',        25, NULL, 2),
    ('transferability', 'edu_language', '{"combo": "edu_lang", "edu_min": "two_or_more", "clb_min": 7}',   '2+ creds + CLB 7',   25, NULL, 3),

    -- transferability — education + Canadian experience
    ('transferability', 'edu_work', '{"combo": "edu_canadian", "edu_min": "one_year_post", "canadian_min": 1}', 'Edu + 1yr CA',       13, NULL, 5),
    ('transferability', 'edu_work', '{"combo": "edu_canadian", "edu_min": "one_year_post", "canadian_min": 2}', 'Edu + 2yr CA',       25, NULL, 6),
    ('transferability', 'edu_work', '{"combo": "edu_canadian", "edu_min": "two_or_more", "canadian_min": 1}',   '2+ creds + 1yr CA',  25, NULL, 7),
    ('transferability', 'edu_work', '{"combo": "edu_canadian", "edu_min": "two_or_more", "canadian_min": 2}',   '2+ creds + 2yr CA',  50, NULL, 8),

    -- transferability — foreign experience + language
    ('transferability', 'foreign_language', '{"combo": "foreign_lang", "foreign_min": 1, "clb_min": 7}', '1yr foreign + CLB 7', 13, NULL, 9),
    ('transferability', 'foreign_language', '{"combo": "foreign_lang", "foreign_min": 1, "clb_min": 9}', '1yr foreign + CLB 9', 25, NULL, 10),
    ('transferability', 'foreign_language', '{"combo": "foreign_lang", "foreign_min": 3, "clb_min": 7}', '3yr foreign + CLB 7', 25, NULL, 11),
    ('transferability', 'foreign_language', '{"combo": "foreign_lang", "foreign_min": 3, "clb_min": 9}', '3yr foreign + CLB 9', 50, NULL, 12),

    -- transferability — foreign + Canadian experience
    ('transferability', 'foreign_work', '{"combo": "foreign_canadian", "foreign_min": 1, "canadian_min": 1}', '1yr frgn + 1yr CA', 13, NULL, 13),
    ('transferability', 'foreign_work', '{"combo": "foreign_canadian", "foreign_min": 1, "canadian_min": 2}', '1yr frgn + 2yr CA', 25, NULL, 14),
    ('transferability', 'foreign_work', '{"combo": "foreign_canadian", "foreign_min": 3, "canadian_min": 1}', '3yr frgn + 1yr CA', 25, NULL, 15),
    ('transferability', 'foreign_work', '{"combo": "foreign_canadian", "foreign_min": 3, "canadian_min": 2}', '3yr frgn + 2yr CA', 50, NULL, 16),

    -- transferability — certificate of qualification + language
    ('transferability', 'certificate_language', '{"combo": "cert_lang", "cert": true, "clb_min": 5}', 'Cert + CLB 5', 25, NULL, 17),
    ('transferability', 'certificate_language', '{"combo": "cert_lang", "cert": true, "clb_min": 7}', 'Cert + CLB 7', 50, NULL, 18),

    -- additional — French bonus (25 base + 25 top-up when English CLB 5+)
    ('additional', 'french', '{"clb_min": 7}', 'French NCLC 7+', 25, NULL, 11),
    ('additional', 'french', '{"clb_min": 7, "english_min": 5}', 'French NCLC 7+ & English CLB 5+', 25, NULL, 12),

    -- additional — Canadian education
    ('additional', 'canadian_education', '{"edu": "one_or_two_years"}', 'CA study 1–2yr', 15, NULL, 13),
    ('additional', 'canadian_education', '{"edu": "three_plus_years"}', 'CA study 3yr+',  30, NULL, 14),

    -- additional — arranged employment (pre-2025-03 ruleset era)
    ('additional', 'job_offer', '{"flag": true}', 'Arranged employment', 50, NULL, 15)
) AS v(factor_group, factor_key, input_descriptor, label, points_single, points_with_spouse, sort_order)
    ON rs.version_label = 'IRCC 2025-01 rules'
ON CONFLICT (ruleset_id, factor_group, factor_key, input_descriptor) DO NOTHING;

COMMIT;
