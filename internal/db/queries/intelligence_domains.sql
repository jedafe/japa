-- internal/db/queries/intelligence_domains.sql
-- sqlc queries for the domain projection tables added by the intelligence
-- system expansion: intel_jobs, visa_routes (ALTERed), housing_guides
-- (ALTERed), remittance_rates (ALTERed), comparison_items (ALTERed).
--
-- The 7 pure guide content types (travel_routes, grants, admission,
-- banking, work_career, health_insurance, culture_lifestyle) do NOT need
-- new queries — they use the existing CreateContent query + the
-- custom_field_values infrastructure.
--
-- All queries are tenant-scoped (ADR-112 Amendment 2). Every query filters
-- by the id to guarantee data isolation across tenants.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

-- ===========================================================================
-- intel_jobs
-- ===========================================================================

-- name: CreateIntelJob :one
INSERT INTO intel_jobs (intel_record_id, guide_id, source_url, source_type, trust_level, title, description, country, country_space_id, city, employer, job_type, experience_level, salary_min, salary_max, currency, skills, application_url, deadline, remote, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25) RETURNING *;

-- name: GetIntelJob :one
SELECT * FROM intel_jobs WHERE id = $1;

-- name: GetIntelJobByIntelRecord :one
SELECT * FROM intel_jobs WHERE intel_record_id = $1;

-- name: GetIntelJobBySourceURL :one
SELECT * FROM intel_jobs WHERE source_url = $1;

-- name: UpdateIntelJob :exec
UPDATE intel_jobs
SET title = $2, description = $3, country = $4, country_space_id = $5, city = $6,
 employer = $7, job_type = $8, experience_level = $9, salary_min = $10,
 salary_max = $11, currency = $12, skills = $13, application_url = $14,
 deadline = $15, remote = $16, confidence_overall = $17,
 verification_status = $18, freshness_score = $19, override_source = $20,
 editorial_overrides = $21, last_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: ListIntelJobsKeyset :many
SELECT * FROM intel_jobs
WHERE ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: ListIntelJobsBySpaces :many
SELECT * FROM intel_jobs
WHERE country_space_id = ANY($1::uuid[])
ORDER BY created_at DESC
LIMIT $2;

-- ===========================================================================
-- visa_routes (intelligence columns added by migration 000444)
-- ===========================================================================

-- name: CreateVisaRouteIntel :one
INSERT INTO visa_routes (destination_space_id, visa_category, name, slug, eligibility_summary, requirements_json, typical_fee_usd, path_to_pr, renewal_terms, restrictions, gov_source_url, status, is_active, intel_record_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides, guide_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20) RETURNING *;

-- name: GetVisaRouteByIntelRecord :one
SELECT * FROM visa_routes WHERE intel_record_id = $1;

-- name: UpdateVisaRouteIntel :exec
UPDATE visa_routes
SET name = $2, visa_category = $3, eligibility_summary = $4, requirements_json = $5,
 typical_fee_usd = $6, path_to_pr = $7, renewal_terms = $8, restrictions = $9,
 gov_source_url = $10, source_url_last_checked = $11,
 confidence_overall = $12, verification_status = $13, freshness_score = $14,
 override_source = $15, editorial_overrides = $16,
 last_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: ListVisaRoutesIntelKeyset :many
SELECT * FROM visa_routes
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- housing_guides (intelligence columns added by migration 000445)
-- ===========================================================================

-- name: CreateHousingGuideIntel :one
INSERT INTO housing_guides (destination_space_id, city_space_id, accommodation_type, search_platforms, landlord_requirements, rent_ranges, scam_categories, tips, status, is_active, intel_record_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides, guide_id) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17) RETURNING *;

-- name: GetHousingGuideByIntelRecord :one
SELECT * FROM housing_guides WHERE intel_record_id = $1;

-- name: UpdateHousingGuideIntel :exec
UPDATE housing_guides
SET accommodation_type = $2, search_platforms = $3, landlord_requirements = $4,
 rent_ranges = $5, scam_categories = $6, tips = $7,
 confidence_overall = $8, verification_status = $9, freshness_score = $10,
 override_source = $11, editorial_overrides = $12,
 last_verified_at = now(), updated_at = now()
WHERE id = $1;

-- name: ListHousingGuidesIntelKeyset :many
SELECT * FROM housing_guides
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- remittance_rates (intelligence columns added by migration 000446)
-- ===========================================================================

-- name: CreateRemittanceRateIntel :one
INSERT INTO remittance_rates (provider_id, send_currency, receive_currency, rate, fee_flat, fee_pct, transfer_time, source_url, intel_record_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14) RETURNING *;

-- name: GetRemittanceRateByIntelRecord :one
SELECT * FROM remittance_rates WHERE intel_record_id = $1;

-- name: UpdateRemittanceRateIntel :exec
UPDATE remittance_rates
SET rate = $2, fee_flat = $3, fee_pct = $4, transfer_time = $5,
 confidence_overall = $6, verification_status = $7, freshness_score = $8,
 override_source = $9, editorial_overrides = $10,
 fetched_at = now()
WHERE id = $1;

-- name: ListRemittanceRatesIntelKeyset :many
SELECT * FROM remittance_rates
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY fetched_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- comparison_items (intelligence columns added by migration 000447)
-- ===========================================================================

-- name: CreateComparisonItemIntel :one
INSERT INTO comparison_items (title, geo_space_id, type_id, position, intel_record_id, intel_cost_data, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11) RETURNING *;

-- name: GetComparisonItemByIntelRecord :one
SELECT * FROM comparison_items WHERE intel_record_id = $1;

-- name: UpdateComparisonItemIntel :exec
UPDATE comparison_items
SET intel_cost_data = $2,
 confidence_overall = $3, verification_status = $4, freshness_score = $5,
 override_source = $6, editorial_overrides = $7,
 last_updated_at = now()
WHERE id = $1;

-- name: ListComparisonItemsIntelKeyset :many
SELECT * FROM comparison_items
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- wiki_pages (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateWikiPageIntel :one
INSERT INTO wiki_pages (space_id, slug, title, category, is_active, is_featured, is_locked, created_by, intel_record_id, guide_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15) RETURNING *;

-- name: GetWikiPageByIntelRecord :one
SELECT * FROM wiki_pages WHERE intel_record_id = $1;

-- name: GetWikiPageByIntelRecordWithSpace :one
SELECT wp.*, s.slug AS space_slug
FROM wiki_pages wp
JOIN spaces s ON s.id = wp.space_id
WHERE wp.intel_record_id = $1;

-- name: UpdateWikiPageIntel :exec
UPDATE wiki_pages
SET title = $2, category = $3, is_active = $4, is_featured = $5, is_locked = $6,
 confidence_overall = $7, verification_status = $8, freshness_score = $9,
 override_source = $10, editorial_overrides = $11,
 updated_at = now()
WHERE id = $1;

-- name: ListWikiPagesIntelKeyset :many
SELECT * FROM wiki_pages
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- language_test_requirements (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateLanguageTestRequirementIntel :one
INSERT INTO language_test_requirements (destination_space_id, visa_category, test_id, minimum_score, score_breakdown, notes, source_url, status, is_active, intel_record_id, guide_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: GetLanguageTestRequirementByIntelRecord :one
SELECT * FROM language_test_requirements WHERE intel_record_id = $1;

-- name: UpdateLanguageTestRequirementIntel :exec
UPDATE language_test_requirements
SET visa_category = $2, minimum_score = $3, score_breakdown = $4, notes = $5,
 source_url = $6, status = $7, is_active = $8,
 confidence_overall = $9, verification_status = $10, freshness_score = $11,
 override_source = $12, editorial_overrides = $13,
 updated_at = now()
WHERE id = $1;

-- name: ListLanguageTestRequirementsIntelKeyset :many
SELECT * FROM language_test_requirements
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: GetLanguageTestByName :one
-- language_tests is a global reference table (no the id). The parameter
-- is accepted for interface consistency but not used in the filter.
-- The promoter resolves the language_tests(id) FK from a
-- test_name extracted as free text.
SELECT * FROM language_tests WHERE name = $1 LIMIT 1;

-- ===========================================================================
-- tax_obligation_guides (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateTaxObligationGuideIntel :one
INSERT INTO tax_obligation_guides (origin_space_id, destination_space_id, employment_status, residency_trigger, worldwide_income_taxed, first_year_obligations, action_checklist, status, is_active, intel_record_id, guide_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16) RETURNING *;

-- name: GetTaxObligationGuideByIntelRecord :one
SELECT * FROM tax_obligation_guides WHERE intel_record_id = $1;

-- name: UpdateTaxObligationGuideIntel :exec
UPDATE tax_obligation_guides
SET employment_status = $2, residency_trigger = $3, worldwide_income_taxed = $4,
 first_year_obligations = $5, action_checklist = $6, status = $7, is_active = $8,
 confidence_overall = $9, verification_status = $10, freshness_score = $11,
 override_source = $12, editorial_overrides = $13,
 updated_at = now()
WHERE id = $1;

-- name: ListTaxObligationGuidesIntelKeyset :many
SELECT * FROM tax_obligation_guides
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- community_organisations (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateCommunityOrganisationIntel :one
INSERT INTO community_organisations (name, slug, org_type, origin_community, destination_space_id, city_space_id, address, latitude, longitude, website_url, meeting_frequency, contact_info, status, is_active, intel_record_id, guide_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21) RETURNING *;

-- name: GetCommunityOrganisationByIntelRecord :one
SELECT * FROM community_organisations WHERE intel_record_id = $1;

-- name: UpdateCommunityOrganisationIntel :exec
UPDATE community_organisations
SET name = $2, org_type = $3, origin_community = $4, address = $5,
 latitude = $6, longitude = $7, website_url = $8, meeting_frequency = $9,
 contact_info = $10, status = $11, is_active = $12,
 confidence_overall = $13, verification_status = $14, freshness_score = $15,
 override_source = $16, editorial_overrides = $17,
 updated_at = now()
WHERE id = $1;

-- name: ListCommunityOrganisationsIntelKeyset :many
SELECT * FROM community_organisations
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- ===========================================================================
-- credential_pathways (intelligence columns added by migration 000521)
-- ===========================================================================

-- name: CreateCredentialPathwayIntel :one
INSERT INTO credential_pathways (profession_id, issuing_space_id, destination_space_id, regulatory_body, regulatory_body_url, pathway_type, timeline_estimate, cost_estimate_usd, pathway_description, official_application_url, source_url_last_checked, status, is_active, ai_generated, ai_model, ai_prompt_hash, hitl_reviewed_by, hitl_reviewed_at, hitl_review_notes, intel_record_id, guide_id, confidence_overall, verification_status, freshness_score, override_source, editorial_overrides) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22, $23, $24, $25, $26) RETURNING *;

-- name: GetCredentialPathwayByIntelRecord :one
SELECT * FROM credential_pathways WHERE intel_record_id = $1;

-- name: UpdateCredentialPathwayIntel :exec
UPDATE credential_pathways
SET profession_id = $2, issuing_space_id = $3, destination_space_id = $4,
 regulatory_body = $5, regulatory_body_url = $6, pathway_type = $7,
 timeline_estimate = $8, cost_estimate_usd = $9, pathway_description = $10,
 official_application_url = $11, source_url_last_checked = $12,
 status = $13, is_active = $14, ai_generated = $15, ai_model = $16, ai_prompt_hash = $17,
 hitl_reviewed_by = $18, hitl_reviewed_at = $19, hitl_review_notes = $20,
 confidence_overall = $21, verification_status = $22, freshness_score = $23,
 override_source = $24, editorial_overrides = $25,
 updated_at = now()
WHERE id = $1;

-- name: ListCredentialPathwaysIntelKeyset :many
SELECT * FROM credential_pathways
WHERE intel_record_id IS NOT NULL
 AND ($1::timestamptz IS NULL
 OR created_at < $1
 OR (created_at = $1 AND id < $2))
ORDER BY created_at DESC, id DESC
LIMIT $3;

-- name: GetCredentialProfessionBySlug :one
SELECT * FROM credential_professions WHERE slug = $1 LIMIT 1;

-- name: GetCredentialProfessionByName :one
SELECT * FROM credential_professions WHERE name = $1 LIMIT 1;
