-- db/seed/tools_eca_credential_examples.sql
-- Ref: japaconnect-engineering/sections/109-eca-guide.md
--      japaconnect-engineering/sections/94-professional-credential-checker.md
--
-- Demo dataset covering both tool surfaces end-to-end:
--   ECA providers (5) → ECA guides (7: published / draft / ai_draft)
--   Professions (5) → credential pathways (5: published / ai_draft)
--   Community items (4): pending + approved update, pending + published note
--   → exercises the public tool, the admin review queue, and both
--     moderation queues in every state the UI renders.
--
-- Idempotent: safe to run repeatedly (upserts on natural keys / fixed IDs).
--   psql "$JC_DATABASE_URL" -f db/seed/tools_eca_credential_examples.sql

BEGIN;

-- ─────────────────────────────────────────────────────────────────────
-- 1. Demo author for community updates / notes.
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO users (id, email, username, is_synthetic, synthetic_source, status)
VALUES ('a1b2c3d4-e5f6-4890-9abc-def012345001',
        'demo.tools@japa.to', 'tools_demo', true, 'tools_seed', 'active')
ON CONFLICT (email) DO NOTHING;

INSERT INTO user_profiles (user_id, display_name, bio)
SELECT u.id, 'Tools Demo', 'Seed author for tool demo content.'
FROM users u WHERE u.username = 'tools_demo'
ON CONFLICT (user_id) DO UPDATE SET display_name = EXCLUDED.display_name;

-- ─────────────────────────────────────────────────────────────────────
-- 2. ECA providers (§109 designated/known assessment bodies).
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO eca_providers (slug, name, website)
SELECT v.slug, v.name, v.website
FROM (VALUES
	('wes',    'World Education Services (WES)',   'https://www.wes.org'),
	('iqas',   'International Qualifications Assessment Service (IQAS)', 'https://www.alberta.ca/iqas'),
	('icas',   'International Credential Assessment Service (ICAS)',     'https://www.icascanada.ca'),
	('ces',    'Comparative Education Service (CES)',                    'https://learn.utoronto.ca/programs-courses/comparative-education-service'),
	('ecctis', 'Ecctis (UK ENIC)',                                       'https://www.enic.org.uk')
) AS v(slug, name, website)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, website = EXCLUDED.website;

-- ─────────────────────────────────────────────────────────────────────
-- 3. Credential professions.
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO credential_professions (slug, name, category)
SELECT v.slug, v.name, v.category
FROM (VALUES
	('registered-nurse',      'Registered Nurse',      'medical'),
	('physician',             'Physician',             'medical'),
	('civil-engineer',        'Civil Engineer',        'engineering'),
	('chartered-accountant',  'Chartered Accountant',  'finance'),
	('secondary-teacher',     'Secondary School Teacher', 'education')
) AS v(slug, name, category)
ON CONFLICT (slug) DO UPDATE SET name = EXCLUDED.name, category = EXCLUDED.category;

-- ─────────────────────────────────────────────────────────────────────
-- 4. ECA guides — 5 published (Canada has 3 providers × immigration so
--    the compare UI's ≥2-provider gate is exercisable), 1 ai_draft
--    (inactive — approve/reject queue state), 1 draft.
--    Rows whose destination space slug doesn't exist are skipped.
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO eca_guides (
	destination_space_id, provider_id, purpose, pathway_steps,
	fee_estimate, currency, timeline_estimate, documents_required,
	accepted_for, official_url, source_url_last_checked,
	status, is_active, ai_generated, ai_model, ai_prompt_hash
)
SELECT s.id, p.id, v.purpose, v.steps::jsonb,
	v.fee, v.currency, v.timeline, v.docs::jsonb,
	v.accepted::jsonb, v.url, CURRENT_DATE,
	v.status, v.active, v.ai, v.model, v.hash
FROM (VALUES
	('canada', 'wes', 'immigration',
	 '[
	   {"step":1,"title":"Create a WES account","detail":"Choose the ECA Application for IRCC package and get your WES reference number.","duration":"~15 minutes"},
	   {"step":2,"title":"Send your documents","detail":"Your university sends transcripts directly to WES; you upload your degree certificate translation if needed.","duration":"2–4 weeks (institution dependent)"},
	   {"step":3,"title":"WES evaluates","detail":"WES verifies and assesses equivalency against Canadian standards.","duration":"~35 business days"},
	   {"step":4,"title":"Report issued","detail":"Electronic report goes to your account and can be linked to your Express Entry profile.","duration":"instant once complete"}
	 ]',
	 'CAD 258 + delivery', 'CAD', '~35 business days after documents received',
	 '["Degree certificate","Official transcripts (sent by institution)","Passport bio page","Document translations (if not in English/French)"]',
	 '["Express Entry","Provincial Nominee Programs","Study permits (some institutions)"]',
	 'https://www.wes.org/ca/eca/',
	 'published', true, false, NULL, NULL),
	('canada', 'iqas', 'immigration',
	 '[
	   {"step":1,"title":"Apply on the IQAS portal","detail":"Create an account, select ECA for immigration, pay the assessment fee.","duration":"~30 minutes"},
	   {"step":2,"title":"Mail/arrange transcripts","detail":"Institution sends sealed transcripts; IQAS also accepts verified digital channels.","duration":"2–6 weeks"},
	   {"step":3,"title":"Assessment","detail":"IQAS compares your credential to Canadian education standards.","duration":"~15 weeks average"},
	   {"step":4,"title":"Receive certificate","detail":"PDF ECA certificate issued for IRCC.","duration":"instant once complete"}
	 ]',
	 'CAD 220 + courier', 'CAD', '~15 weeks average',
	 '["Degree certificate","Official transcripts","Valid ID","Translations where required"]',
	 '["Express Entry","Alberta Advantage Immigration Program"]',
	 'https://www.alberta.ca/iqas-immigration',
	 'published', true, false, NULL, NULL),
	('canada', 'icas', 'immigration',
	 '[
	   {"step":1,"title":"Start an ICAS application","detail":"Online application; choose the immigration (IRCC) report type.","duration":"~20 minutes"},
	   {"step":2,"title":"Submit documents","detail":"Upload scans now; institution-posted originals may be requested for verification.","duration":"2–4 weeks"},
	   {"step":3,"title":"Verification & assessment","detail":"ICAS contacts issuing institutions directly where needed.","duration":"~8–12 weeks"},
	   {"step":4,"title":"ECA report","detail":"Report downloadable from your ICAS account.","duration":"instant once complete"}
	 ]',
	 'CAD 248', 'CAD', '~8–12 weeks',
	 '["Degree certificate","Transcripts","Government-issued photo ID"]',
	 '["Express Entry","Ontario Immigrant Nominee Program"]',
	 'https://www.icascanada.ca',
	 'published', true, false, NULL, NULL),
	('canada', 'wes', 'study',
	 '[
	   {"step":1,"title":"Create a WES account","detail":"Choose the course-by-course ICAP evaluation most universities request.","duration":"~15 minutes"},
	   {"step":2,"title":"Send documents","detail":"Transcripts sent by your institution; include grading scale.","duration":"2–4 weeks"},
	   {"step":3,"title":"Evaluation","detail":"Course-by-course equivalency with GPA conversion.","duration":"~35 business days"}
	 ]',
	 'CAD 230', 'CAD', '~35 business days',
	 '["Degree certificate","Official transcripts","Syllabus excerpts (some universities)"]',
	 '["University admissions","College admissions","Professional programs"]',
	 'https://www.wes.org/ca/',
	 'published', true, false, NULL, NULL),
	('united-kingdom', 'ecctis', 'employment',
	 '[
	   {"step":1,"title":"Apply to Ecctis (UK ENIC)","detail":"Select the Statement of Comparability for employment/visa purposes.","duration":"~20 minutes"},
	   {"step":2,"title":"Upload documents","detail":"Degree certificate, transcripts, and passport — certified translations if needed.","duration":"same day"},
	   {"step":3,"title":"Assessment","detail":"Ecctis compares your qualification to UK framework levels (RQF).","duration":"~15 working days"},
	   {"step":4,"title":"Statement issued","detail":"Digital statement usable for employers and visa applications.","duration":"instant once complete"}
	 ]',
	 'GBP 196 + VAT', 'GBP', '~15 working days',
	 '["Degree certificate","Transcripts","Passport","Certified translations"]',
	 '["Skilled Worker visa","Graduate employment","Regulated profession entry (partial)"]',
	 'https://www.enic.org.uk',
	 'published', true, false, NULL, NULL),
	('canada', 'ces', 'professional_licensure',
	 '[
	   {"step":1,"title":"Open a CES application","detail":"Select the assessment used by regulated professions.","duration":"~20 minutes"},
	   {"step":2,"title":"Document submission","detail":"Sealed transcripts mailed by the institution.","duration":"2–4 weeks"},
	   {"step":3,"title":"Assessment","detail":"Equivalency report for licensing bodies.","duration":"~6–10 weeks"}
	 ]',
	 'CAD 265', 'CAD', '~6–10 weeks',
	 '["Degree certificate","Sealed transcripts","Professional registration (if held)"]',
	 '["Regulated professions","Licensing bodies (engineering, teaching)"]',
	 'https://learn.utoronto.ca/programs-courses/comparative-education-service',
	 'ai_draft', false, true, 'content_agent_v1', 'demo-seed-prompt-hash-ces'),
	('united-kingdom', 'ecctis', 'immigration',
	 '[
	   {"step":1,"title":"Apply to Ecctis","detail":"Statement of Comparability for immigration routes.","duration":"~20 minutes"},
	   {"step":2,"title":"Upload documents","detail":"Certificate + transcripts + ID.","duration":"same day"},
	   {"step":3,"title":"Assessment & statement","detail":"RQF comparability statement for Home Office use.","duration":"~15 working days"}
	 ]',
	 'GBP 196 + VAT', 'GBP', '~15 working days',
	 '["Degree certificate","Transcripts","Passport"]',
	 '["Skilled Worker visa","Global Talent visa"]',
	 'https://www.enic.org.uk',
	 'draft', true, false, NULL, NULL)
) AS v(dest, provider, purpose, steps, fee, currency, timeline, docs, accepted, url, status, active, ai, model, hash)
JOIN spaces s ON s.slug = v.dest
JOIN eca_providers p ON p.slug = v.provider
ON CONFLICT (destination_space_id, provider_id, purpose) DO UPDATE SET
	pathway_steps = EXCLUDED.pathway_steps,
	fee_estimate = EXCLUDED.fee_estimate,
	currency = EXCLUDED.currency,
	timeline_estimate = EXCLUDED.timeline_estimate,
	documents_required = EXCLUDED.documents_required,
	accepted_for = EXCLUDED.accepted_for,
	official_url = EXCLUDED.official_url,
	source_url_last_checked = EXCLUDED.source_url_last_checked,
	status = EXCLUDED.status,
	is_active = EXCLUDED.is_active,
	ai_generated = EXCLUDED.ai_generated,
	ai_model = EXCLUDED.ai_model,
	ai_prompt_hash = EXCLUDED.ai_prompt_hash;

-- ─────────────────────────────────────────────────────────────────────
-- 5. Credential pathways — 4 published, 1 ai_draft (inactive → exercises
--    the approve/reject queue). issuing space = nigeria for all; rows are
--    skipped when the space slug or profession slug doesn't exist.
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO credential_pathways (
	profession_id, issuing_space_id, destination_space_id,
	regulatory_body, regulatory_body_url, pathway_type,
	timeline_estimate, cost_estimate_usd, pathway_description,
	pathway_steps,
	official_application_url, source_url_last_checked,
	status, is_active, ai_generated, ai_model, ai_prompt_hash
)
SELECT pr.id, si.id, sd.id,
	v.body, v.body_url, v.ptype,
	v.timeline, v.cost, v.descr,
	v.steps::jsonb,
	v.app_url, CURRENT_DATE,
	v.status, v.active, v.ai, v.model, v.hash
FROM (VALUES
	('registered-nurse', 'nigeria', 'canada',
	 'National Nursing Assessment Service (NNAS) → provincial college (e.g., College of Nurses of Ontario)',
	 'https://www.nnas.ca', 'bridging_exam',
	 '12–24 months', '~$2,500–4,000 (NNAS + exams + bridging)',
	 'Nurses trained in Nigeria complete an NNAS advisory report, then apply to a provincial regulator. Most are asked to pass the NCLEX-RN and may need bridging courses or a supervised-practice period before licensure.',
	 '[
	   {"step":1,"title":"NNAS advisory report","detail":"Create an NNAS account, submit credentials and documents for the advisory report.","duration":"~3–6 months"},
	   {"step":2,"title":"Apply to the provincial college","detail":"Submit your NNAS report to the provincial regulator (e.g., College of Nurses of Ontario).","duration":"~1–2 months"},
	   {"step":3,"title":"NCLEX-RN exam","detail":"Sit the NCLEX-RN licensing exam at an approved test centre.","duration":"exam day + ~6 weeks results"},
	   {"step":4,"title":"Bridging/supervised practice (if required)","detail":"Complete assigned competency-bridging courses or a supervised-practice period.","duration":"3–12 months (if assigned)"}
	 ]',
	 'https://www.nnas.ca/apply/',
	 'published', true, false, NULL, NULL),
	('physician', 'nigeria', 'united-kingdom',
	 'General Medical Council (GMC)',
	 'https://www.gmc-uk.org', 'bridging_exam',
	 '12–18 months', '~$3,000–5,000 (PLAB 1+2, GMC registration, visa)',
	 'Internationally trained doctors register with the GMC via the PLAB route: PLAB 1 (written), PLAB 2 (OSCE in Manchester), then GMC registration. A recognised primary medical qualification and internship completion are required; supervised practice applies on first registration.',
	 '[
	   {"step":1,"title":"Verify your primary medical qualification","detail":"Confirm your MBBCh/MD is on the GMC-recognised list and complete EPIC primary-source verification.","duration":"4–8 weeks"},
	   {"step":2,"title":"PLAB 1 (written)","detail":"Book and sit PLAB 1 — UK centres fill months ahead.","duration":"exam day + ~4 weeks results"},
	   {"step":3,"title":"PLAB 2 (OSCE, Manchester)","detail":"Clinical assessment held only in Manchester.","duration":"exam day + ~4 weeks results"},
	   {"step":4,"title":"GMC registration","detail":"Apply for registration with licence to practise; first registration is supervised.","duration":"~4–6 weeks"}
	 ]',
	 'https://www.gmc-uk.org/registration-and-licensing/join-the-register/plab',
	 'published', true, false, NULL, NULL),
	('civil-engineer', 'nigeria', 'canada',
	 'Engineers Canada → provincial association (e.g., PEO)',
	 'https://engineerscanada.ca', 'supervised_practice',
	 '18–36 months', '~$1,500–3,000 (assessment + exams + EIT period)',
	 'Nigerian engineering degrees are assessed by the provincial regulator (academic review). Most applicants are assigned confirmatory/technical exams, then complete a supervised Engineering-in-Training (EIT) period plus the Professional Practice Examination before P.Eng licensure.',
	 '[
	   {"step":1,"title":"Academic assessment","detail":"Submit degree + transcripts to the provincial association for equivalency review.","duration":"~3–6 months"},
	   {"step":2,"title":"Confirmatory/technical exams","detail":"Sit any assigned exams from the academic review.","duration":"3–12 months (if assigned)"},
	   {"step":3,"title":"EIT supervised experience","detail":"Accumulate 48 months of engineering experience (up to 12 months of international experience may count).","duration":"up to 48 months total"},
	   {"step":4,"title":"Professional Practice Examination","detail":"Ethics + law exam, then P.Eng licensure.","duration":"~3 months"}
	 ]',
	 'https://engineerscanada.ca/become-an-engineer',
	 'published', true, false, NULL, NULL),
	('chartered-accountant', 'nigeria', 'united-kingdom',
	 'ICAEW / ACCA (mutual recognition with ICAN)',
	 'https://www.icaew.com', 'direct',
	 '3–9 months', '~$500–1,500 (membership + assessment fees)',
	 'ICAN members in good standing can seek UK chartered membership through reciprocal agreements (ICAEW/ICAN MOU) or ACCA pathways. Where full reciprocity is unavailable, exemptions cover most papers; the remainder plus professional-level assessments complete the route.',
	 '[
	   {"step":1,"title":"Confirm ICAN standing","detail":"Obtain a letter of good standing from ICAN covering membership and CPD.","duration":"2–6 weeks"},
	   {"step":2,"title":"Map exemptions","detail":"Apply to ICAEW/ACCA for the reciprocal route and confirm which papers are exempted.","duration":"~1–2 months"},
	   {"step":3,"title":"Complete remaining assessments","detail":"Sit any non-exempt professional-level papers.","duration":"3–6 months"},
	   {"step":4,"title":"Membership & practising certificate","detail":"Apply for UK membership; separate practising certificate if offering public practice.","duration":"~1 month"}
	 ]',
	 'https://www.icaew.com/membership/become-a-member',
	 'published', true, false, NULL, NULL),
	('secondary-teacher', 'nigeria', 'canada',
	 'Provincial teacher regulatory body (e.g., Ontario College of Teachers)',
	 'https://www.oct.ca', 'supervised_practice',
	 '12–24 months', '~$1,000–2,500 (certification + bridging)',
	 'Internationally educated teachers apply to the provincial college of teachers for evaluation. Outcomes typically require additional teacher-education coursework and a supervised practicum before full certification.',
	 '[
	   {"step":1,"title":"Credential evaluation","detail":"Submit transcripts and teaching qualifications to the provincial college for assessment.","duration":"~2–4 months"},
	   {"step":2,"title":"Bridging coursework","detail":"Complete assigned teacher-education courses at an accredited provider.","duration":"6–12 months (if assigned)"},
	   {"step":3,"title":"Supervised practicum","detail":"Classroom practicum under a certified mentor teacher.","duration":"~1 term"},
	   {"step":4,"title":"Certification","detail":"College issues the teaching certificate on successful review.","duration":"~1–2 months"}
	 ]',
	 'https://www.oct.ca/becoming-a-teacher/internationally-educated-teachers',
	 'ai_draft', false, true, 'content_agent_v1', 'demo-seed-prompt-hash-teacher')
) AS v(prof, issuing, dest, body, body_url, ptype, timeline, cost, descr, steps, app_url, status, active, ai, model, hash)
JOIN credential_professions pr ON pr.slug = v.prof
JOIN spaces si ON si.slug = v.issuing
JOIN spaces sd ON sd.slug = v.dest
ON CONFLICT (profession_id, issuing_space_id, destination_space_id) DO UPDATE SET
	regulatory_body = EXCLUDED.regulatory_body,
	regulatory_body_url = EXCLUDED.regulatory_body_url,
	pathway_type = EXCLUDED.pathway_type,
	timeline_estimate = EXCLUDED.timeline_estimate,
	cost_estimate_usd = EXCLUDED.cost_estimate_usd,
	pathway_description = EXCLUDED.pathway_description,
	pathway_steps = EXCLUDED.pathway_steps,
	official_application_url = EXCLUDED.official_application_url,
	source_url_last_checked = EXCLUDED.source_url_last_checked,
	status = EXCLUDED.status,
	is_active = EXCLUDED.is_active,
	ai_generated = EXCLUDED.ai_generated,
	ai_model = EXCLUDED.ai_model,
	ai_prompt_hash = EXCLUDED.ai_prompt_hash;

-- ─────────────────────────────────────────────────────────────────────
-- 6. Community items — one pending + one approved ECA update, one
--    pending + one published credential note (moderation queue states).
-- ─────────────────────────────────────────────────────────────────────

INSERT INTO eca_community_updates (id, guide_id, user_id, step_index, fee_estimate, turnaround_days, note, status, reviewed_by)
SELECT 'b2c3d4e5-f6a7-4890-9abc-def012345001', g.id, u.id, 3,
	'CAD 285', 49, 'Fee went up this year — paid CAD 285 including courier delivery of the report.', 'pending', NULL
FROM eca_guides g
JOIN spaces s ON s.id = g.destination_space_id AND s.slug = 'canada'
JOIN eca_providers p ON p.id = g.provider_id AND p.slug = 'wes',
users u
WHERE g.purpose = 'immigration' AND u.username = 'tools_demo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO eca_community_updates (id, guide_id, user_id, step_index, fee_estimate, turnaround_days, note, status, reviewed_by)
SELECT 'b2c3d4e5-f6a7-4890-9abc-def012345002', g.id, u.id, NULL,
	NULL, 100, 'My IQAS report took about 14 weeks end to end — budget more time than the official estimate.', 'approved', u.id
FROM eca_guides g
JOIN spaces s ON s.id = g.destination_space_id AND s.slug = 'canada'
JOIN eca_providers p ON p.id = g.provider_id AND p.slug = 'iqas',
users u
WHERE g.purpose = 'immigration' AND u.username = 'tools_demo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO credential_community_notes (id, pathway_id, user_id, note, status)
SELECT 'c3d4e5f6-a7b8-4890-9abc-def012345001', pw.id, u.id,
	'The NNAS portal was slow — allow an extra two weeks for document upload verification before your file is complete.', 'pending'
FROM credential_pathways pw
JOIN credential_professions pr ON pr.id = pw.profession_id AND pr.slug = 'registered-nurse'
JOIN spaces sd ON sd.id = pw.destination_space_id AND sd.slug = 'canada',
users u
WHERE u.username = 'tools_demo'
ON CONFLICT (id) DO NOTHING;

INSERT INTO credential_community_notes (id, pathway_id, user_id, note, status)
SELECT 'c3d4e5f6-a7b8-4890-9abc-def012345002', pw.id, u.id,
	'Book PLAB 1 early — UK test centres fill up months ahead, and the result takes ~4 weeks.', 'published'
FROM credential_pathways pw
JOIN credential_professions pr ON pr.id = pw.profession_id AND pr.slug = 'physician'
JOIN spaces sd ON sd.id = pw.destination_space_id AND sd.slug = 'united-kingdom',
users u
WHERE u.username = 'tools_demo'
ON CONFLICT (id) DO NOTHING;

COMMIT;
