-- 000486_seed_cv_standards_all_countries.up.sql
-- Seeds cv_standards for ALL ~250 country spaces (§95, ADR-103).
-- Uses geo_code to resolve destination_space_id from spaces.
-- Standards are grouped by regional CV convention:
--   * North America (US, CA): 1-2 pages, no photo, ATS-friendly, MM/DD/YYYY
--   * UK & Ireland: 1-2 pages, no photo, DD/MM/YYYY, "CV" terminology
--   * EU (continental): 1-2 pages, photo optional, DD/MM/YYYY, personal details
--   * Middle East & Gulf: 1-2 pages, photo required, DD/MM/YYYY
--   * Africa: 1-3 pages, photo optional, DD/MM/YYYY
--   * Asia-Pacific: 1-2 pages, photo varies, YYYY-MM-DD or DD/MM/YYYY
--   * Latin America: 1-2 pages, photo optional, DD/MM/YYYY
--   * Oceania: 1-2 pages, no photo, DD/MM/YYYY
--   * Default (rest of world): 1-2 pages, photo optional, DD/MM/YYYY
-- Idempotent: ON CONFLICT (destination_space_id) DO NOTHING.
-- Also enables the cv_formatter.enabled feature flag.

BEGIN;

-- ============================================================
-- Helper: insert a standard for a country by geo_code
-- ============================================================

-- North America (US, CA) — ATS-friendly, no photo, MM/DD/YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'prohibited', 'MM/DD/YYYY',
    ARRAY['Contact Information', 'Professional Summary', 'Work Experience', 'Education', 'Skills', 'Certifications', 'Optional: Volunteer Experience'],
    'Concise and achievement-oriented. Use action verbs and quantifiable results. Tailor to the job description. Avoid first-person pronouns.',
    'Including a photo (ATS systems reject it), using subjective statements without evidence, exceeding 2 pages, including personal details (age, marital status, SSN), using fancy formatting that breaks ATS parsing.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('US', 'CA')
ON CONFLICT (destination_space_id) DO NOTHING;

-- UK & Ireland — no photo, DD/MM/YYYY, "CV" terminology
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'not_recommended', 'DD/MM/YYYY',
    ARRAY['Personal Details', 'Personal Profile', 'Work Experience', 'Education', 'Skills', 'Achievements', 'Optional: Hobbies & Interests'],
    'Professional and evidence-based. Use reverse-chronological order. Keep to two pages unless senior-level. Include a strong personal profile statement.',
    'Exceeding 2 pages for non-senior roles, including a photo, using American spelling, listing references on the CV, including age or marital status.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('GB', 'IE')
ON CONFLICT (destination_space_id) DO NOTHING;

-- EU (continental Europe) — photo optional, DD/MM/YYYY, personal details expected
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'optional', 'DD/MM/YYYY',
    ARRAY['Personal Information', 'Professional Summary', 'Work Experience', 'Education', 'Language Skills', 'IT Skills', 'References'],
    'Structured and formal. Include nationality and date of birth (common practice). List language proficiency using CEFR levels. Use reverse-chronological order.',
    'Omitting language skills, not including personal details expected in EU markets, exceeding 2 pages, using informal tone, not tailoring to the specific country.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('DE', 'FR', 'NL', 'BE', 'AT', 'CH', 'LU', 'ES', 'PT', 'IT', 'SE', 'NO', 'DK', 'FI', 'PL', 'CZ', 'SK', 'HU', 'RO', 'BG', 'HR', 'SI', 'EE', 'LV', 'LT', 'GR', 'CY', 'MT', 'IS', 'LI', 'MC', 'SM', 'VA', 'AD', 'MT')
ON CONFLICT (destination_space_id) DO NOTHING;

-- Middle East & Gulf — photo required, DD/MM/YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'required', 'DD/MM/YYYY',
    ARRAY['Personal Details (with Photo)', 'Career Objective', 'Work Experience', 'Education', 'Skills', 'Languages', 'References'],
    'Formal and detailed. Include a professional passport-size photo. List nationality, visa status, and date of birth. Mention notice period and expected salary.',
    'Omitting a professional photo, not including nationality/visa status, using an informal tone, not listing references available, exceeding 2 pages.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('SA', 'AE', 'QA', 'KW', 'BH', 'OM', 'JO', 'LB', 'IQ', 'IR', 'SY', 'YE', 'PS', 'IL', 'TR', 'EG', 'LY', 'SD', 'SS', 'MA', 'DZ', 'TN', 'MR')
ON CONFLICT (destination_space_id) DO NOTHING;

-- Sub-Saharan Africa — photo optional, DD/MM/YYYY, 1-3 pages
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-3 pages', 'optional', 'DD/MM/YYYY',
    ARRAY['Personal Details', 'Career Objective', 'Work Experience', 'Education', 'Skills', 'Referees'],
    'Formal and detailed. Include referees directly on the CV (common practice). Mention nationality and language proficiency. Use reverse-chronological order.',
    'Not listing referees, omitting career objective, using informal language, not including relevant certifications, exceeding 3 pages for non-academic roles.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('NG', 'GH', 'KE', 'UG', 'TZ', 'RW', 'ET', 'ZM', 'ZW', 'BW', 'NA', 'ZA', 'LS', 'SZ', 'MZ', 'MW', 'MG', 'MU', 'SC', 'KM', 'CV', 'ST', 'GW', 'GN', 'SL', 'LR', 'CI', 'GH', 'TG', 'BJ', 'BF', 'ML', 'SN', 'GM', 'CM', 'GA', 'CG', 'CD', 'TD', 'CF', 'AO', 'BI', 'DJ', 'ER', 'KM', 'SC', 'SH', 'TD', 'NE', 'NG')
ON CONFLICT (destination_space_id) DO NOTHING;

-- Asia-Pacific (developed markets) — no photo, YYYY-MM-DD or DD/MM/YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'not_recommended', 'YYYY-MM-DD',
    ARRAY['Contact Information', 'Summary', 'Work Experience', 'Education', 'Skills', 'Certifications'],
    'Concise and results-focused. Use reverse-chronological order. Tailor to the specific market — Singapore/HK/India/Australia have different expectations. Avoid photos unless specifically requested.',
    'Including a photo when not requested, exceeding 2 pages, using generic templates, not quantifying achievements, including irrelevant personal information.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('SG', 'HK', 'JP', 'KR', 'TW', 'AU', 'NZ', 'MY', 'TH', 'VN', 'ID', 'PH', 'IN', 'PK', 'BD', 'LK', 'NP', 'MM', 'KH', 'LA', 'BN', 'MO')
ON CONFLICT (destination_space_id) DO NOTHING;

-- China — photo expected, YYYY-MM-DD
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'optional', 'YYYY-MM-DD',
    ARRAY['Personal Information (with Photo)', 'Self-Assessment', 'Work Experience', 'Education', 'Skills', 'Certifications'],
    'Formal and structured. Include a professional photo (common practice). List political affiliation and marital status for state-owned enterprises. Use Chinese language for domestic applications.',
    'Omitting self-assessment section, not including photo for domestic applications, using English-only for Chinese-language roles, exceeding 2 pages.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code = 'CN'
ON CONFLICT (destination_space_id) DO NOTHING;

-- Latin America — photo optional, DD/MM/YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'optional', 'DD/MM/YYYY',
    ARRAY['Datos Personales', 'Objetivo Profesional', 'Experiencia Laboral', 'Educación', 'Habilidades', 'Referencias'],
    'Formal and detailed. Write in the local language (Spanish or Portuguese). Include personal details (nationality, date of birth, marital status). List references directly.',
    'Writing in English for local-language roles, omitting personal details, not including references, using an informal tone, exceeding 2 pages.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('MX', 'BR', 'AR', 'CL', 'CO', 'PE', 'VE', 'EC', 'BO', 'PY', 'UY', 'CR', 'PA', 'GT', 'HN', 'SV', 'NI', 'DO', 'CU', 'PR', 'JM', 'HT', 'TT', 'BB', 'GD', 'LC', 'VC', 'KN', 'DM', 'AG', 'BS', 'BZ', 'GY', 'SR', 'GF', 'MQ', 'GP', 'RE', 'CW', 'AW', 'SX', 'BQ', 'BL', 'MF', 'PM', 'MS', 'KY', 'BM', 'VG', 'VI', 'GU', 'MP', 'AS', 'CK', 'NU', 'NF', 'PN', 'TK', 'TO', 'WS', 'FJ', 'PG', 'SB', 'VU', 'NC', 'WF', 'PF', 'TF', 'HM', 'GS', 'IO', 'CC', 'CX', 'SH', 'AC', 'TA', 'AQ', 'UM', 'BV', 'SJ', 'FO', 'GL', 'AX', 'IM', 'JE', 'GG', 'EH', 'XK')
ON CONFLICT (destination_space_id) DO NOTHING;

-- Russia & CIS — photo expected, DD.MM.YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'optional', 'DD.MM.YYYY',
    ARRAY['Личные данные (Personal Details)', 'Цель (Objective)', 'Опыт работы (Work Experience)', 'Образование (Education)', 'Навыки (Skills)', 'Языки (Languages)'],
    'Formal and structured. Include a professional photo. List date of birth, marital status, and citizenship (common practice). Use Russian for domestic applications.',
    'Omitting personal details expected in CIS markets, using English-only for Russian-language roles, not including a photo, exceeding 2 pages.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code IN ('RU', 'UA', 'BY', 'KZ', 'UZ', 'TM', 'KG', 'TJ', 'AZ', 'AM', 'GE', 'MD', 'ME', 'RS', 'BA', 'AL', 'MK', 'XK')
ON CONFLICT (destination_space_id) DO NOTHING;

-- Default: all remaining countries — photo optional, DD/MM/YYYY
INSERT INTO cv_standards (destination_space_id, recommended_length_pages, photo_policy, date_format, recommended_sections, tone_guidance, common_pitfalls)
SELECT s.id, '1-2 pages', 'optional', 'DD/MM/YYYY',
    ARRAY['Personal Details', 'Professional Summary', 'Work Experience', 'Education', 'Skills', 'Languages', 'References'],
    'Professional and clear. Use reverse-chronological order. Tailor to the specific job description. Include language proficiency and relevant certifications.',
    'Exceeding 2 pages, using an informal tone, not tailoring to the job, including irrelevant personal information, not quantifying achievements.'
FROM spaces s
WHERE s.tenant_id = '00000000-0000-0000-0000-000000000001'
  AND s.level = 'country'
  AND s.geo_code NOT IN (
    -- Already seeded above
    'US', 'CA', 'GB', 'IE',
    'DE', 'FR', 'NL', 'BE', 'AT', 'CH', 'LU', 'ES', 'PT', 'IT', 'SE', 'NO', 'DK', 'FI', 'PL', 'CZ', 'SK', 'HU', 'RO', 'BG', 'HR', 'SI', 'EE', 'LV', 'LT', 'GR', 'CY', 'MT', 'IS', 'LI', 'MC', 'SM', 'VA', 'AD',
    'SA', 'AE', 'QA', 'KW', 'BH', 'OM', 'JO', 'LB', 'IQ', 'IR', 'SY', 'YE', 'PS', 'IL', 'TR', 'EG', 'LY', 'SD', 'SS', 'MA', 'DZ', 'TN', 'MR',
    'NG', 'GH', 'KE', 'UG', 'TZ', 'RW', 'ET', 'ZM', 'ZW', 'BW', 'NA', 'ZA', 'LS', 'SZ', 'MZ', 'MW', 'MG', 'MU', 'SC', 'KM', 'CV', 'ST', 'GW', 'GN', 'SL', 'LR', 'CI', 'TG', 'BJ', 'BF', 'ML', 'SN', 'GM', 'CM', 'GA', 'CG', 'CD', 'TD', 'CF', 'AO', 'BI', 'DJ', 'ER', 'NE',
    'SG', 'HK', 'JP', 'KR', 'TW', 'AU', 'NZ', 'MY', 'TH', 'VN', 'ID', 'PH', 'IN', 'PK', 'BD', 'LK', 'NP', 'MM', 'KH', 'LA', 'BN', 'MO', 'CN',
    'MX', 'BR', 'AR', 'CL', 'CO', 'PE', 'VE', 'EC', 'BO', 'PY', 'UY', 'CR', 'PA', 'GT', 'HN', 'SV', 'NI', 'DO', 'CU', 'PR', 'JM', 'HT', 'TT', 'BB', 'GD', 'LC', 'VC', 'KN', 'DM', 'AG', 'BS', 'BZ', 'GY', 'SR', 'GF', 'MQ', 'GP', 'RE', 'CW', 'AW', 'SX', 'BQ', 'BL', 'MF', 'PM', 'MS', 'KY', 'BM', 'VG', 'VI', 'GU', 'MP', 'AS', 'CK', 'NU', 'NF', 'PN', 'TK', 'TO', 'WS', 'FJ', 'PG', 'SB', 'VU', 'NC', 'WF', 'PF', 'TF', 'HM', 'GS', 'IO', 'CC', 'CX', 'SH', 'AC', 'TA', 'AQ', 'UM', 'BV', 'SJ', 'FO', 'GL', 'AX', 'IM', 'JE', 'GG', 'EH', 'XK',
    'RU', 'UA', 'BY', 'KZ', 'UZ', 'TM', 'KG', 'TJ', 'AZ', 'AM', 'GE', 'MD', 'ME', 'RS', 'BA', 'AL', 'MK'
  )
  AND NOT EXISTS (
    SELECT 1 FROM cv_standards cs WHERE cs.destination_space_id = s.id
  )
ON CONFLICT (destination_space_id) DO NOTHING;

-- ============================================================
-- Enable the CV formatter feature flag (§95)
-- ============================================================
UPDATE system_settings
SET value = 'true'
WHERE tenant_id = '00000000-0000-0000-0000-000000000001'
  AND key = 'cv_formatter.enabled';

COMMIT;
