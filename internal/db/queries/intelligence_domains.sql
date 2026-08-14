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
-- by tenant_id to guarantee data isolation across tenants.
--
-- Ref: docs/superpowers/specs/2026-08-12-japa-intelligence-system-design.md (Section 8)

-- ===========================================================================
-- intel_jobs
-- ===========================================================================

-- name: CreateIntelJob :one
INSERT INTO intel_jobs (
  tenant_id, intel_record_id, guide_id, source_url, source_type, trust_level,
  title, description, country, country_space_id, city, employer,
  job_type, experience_level, salary_min, salary_max, currency, skills,
  application_url, deadline, remote, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20,
  $21, $22, $23, $24, $25, $26
) RETURNING *;

-- name: GetIntelJob :one
SELECT * FROM intel_jobs WHERE tenant_id = $1 AND id = $2;

-- name: GetIntelJobByIntelRecord :one
SELECT * FROM intel_jobs WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: GetIntelJobBySourceURL :one
SELECT * FROM intel_jobs WHERE tenant_id = $1 AND source_url = $2;

-- name: UpdateIntelJob :exec
UPDATE intel_jobs
SET title = $3, description = $4, country = $5, country_space_id = $6, city = $7,
    employer = $8, job_type = $9, experience_level = $10, salary_min = $11,
    salary_max = $12, currency = $13, skills = $14, application_url = $15,
    deadline = $16, remote = $17, confidence_overall = $18,
    verification_status = $19, freshness_score = $20, override_source = $21,
    editorial_overrides = $22, last_verified_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListIntelJobsKeyset :many
SELECT * FROM intel_jobs
WHERE tenant_id = $1
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: ListIntelJobsBySpaces :many
SELECT * FROM intel_jobs
WHERE tenant_id = $1
  AND country_space_id = ANY($2::uuid[])
ORDER BY created_at DESC
LIMIT $3;

-- ===========================================================================
-- visa_routes (intelligence columns added by migration 000444)
-- ===========================================================================

-- name: CreateVisaRouteIntel :one
INSERT INTO visa_routes (
  tenant_id, destination_space_id, visa_category, name, slug,
  eligibility_summary, requirements_json, typical_fee_usd, path_to_pr,
  renewal_terms, restrictions, gov_source_url, status, is_active,
  intel_record_id, confidence_overall, verification_status, freshness_score,
  override_source, editorial_overrides, guide_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21
) RETURNING *;

-- name: GetVisaRouteByIntelRecord :one
SELECT * FROM visa_routes WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateVisaRouteIntel :exec
UPDATE visa_routes
SET name = $3, visa_category = $4, eligibility_summary = $5, requirements_json = $6,
    typical_fee_usd = $7, path_to_pr = $8, renewal_terms = $9, restrictions = $10,
    gov_source_url = $11, source_url_last_checked = $12,
    confidence_overall = $13, verification_status = $14, freshness_score = $15,
    override_source = $16, editorial_overrides = $17,
    last_verified_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListVisaRoutesIntelKeyset :many
SELECT * FROM visa_routes
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- housing_guides (intelligence columns added by migration 000445)
-- ===========================================================================

-- name: CreateHousingGuideIntel :one
INSERT INTO housing_guides (
  tenant_id, destination_space_id, city_space_id, accommodation_type,
  search_platforms, landlord_requirements, rent_ranges, scam_categories,
  tips, status, is_active,
  intel_record_id, confidence_overall, verification_status, freshness_score,
  override_source, editorial_overrides, guide_id
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18
) RETURNING *;

-- name: GetHousingGuideByIntelRecord :one
SELECT * FROM housing_guides WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateHousingGuideIntel :exec
UPDATE housing_guides
SET accommodation_type = $3, search_platforms = $4, landlord_requirements = $5,
    rent_ranges = $6, scam_categories = $7, tips = $8,
    confidence_overall = $9, verification_status = $10, freshness_score = $11,
    override_source = $12, editorial_overrides = $13,
    last_verified_at = now(), updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListHousingGuidesIntelKeyset :many
SELECT * FROM housing_guides
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- remittance_rates (intelligence columns added by migration 000446)
-- ===========================================================================

-- name: CreateRemittanceRateIntel :one
INSERT INTO remittance_rates (
  tenant_id, provider_id, send_currency, receive_currency, rate,
  fee_flat, fee_pct, transfer_time, source_url,
  intel_record_id, confidence_overall, verification_status, freshness_score,
  override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9,
  $10, $11, $12, $13, $14, $15
) RETURNING *;

-- name: GetRemittanceRateByIntelRecord :one
SELECT * FROM remittance_rates WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateRemittanceRateIntel :exec
UPDATE remittance_rates
SET rate = $3, fee_flat = $4, fee_pct = $5, transfer_time = $6,
    confidence_overall = $7, verification_status = $8, freshness_score = $9,
    override_source = $10, editorial_overrides = $11,
    fetched_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListRemittanceRatesIntelKeyset :many
SELECT * FROM remittance_rates
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY fetched_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- comparison_items (intelligence columns added by migration 000447)
-- ===========================================================================

-- name: CreateComparisonItemIntel :one
INSERT INTO comparison_items (
  tenant_id, title, geo_space_id, type_id, position,
  intel_record_id, intel_cost_data, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12
) RETURNING *;

-- name: GetComparisonItemByIntelRecord :one
SELECT * FROM comparison_items WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateComparisonItemIntel :exec
UPDATE comparison_items
SET intel_cost_data = $3,
    confidence_overall = $4, verification_status = $5, freshness_score = $6,
    override_source = $7, editorial_overrides = $8,
    last_updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListComparisonItemsIntelKeyset :many
SELECT * FROM comparison_items
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- wiki_pages (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateWikiPageIntel :one
INSERT INTO wiki_pages (
  tenant_id, space_id, slug, title, category, is_active, is_featured,
  is_locked, created_by,
  intel_record_id, guide_id, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9,
  $10, $11, $12, $13, $14, $15, $16
) RETURNING *;

-- name: GetWikiPageByIntelRecord :one
SELECT * FROM wiki_pages WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateWikiPageIntel :exec
UPDATE wiki_pages
SET title = $3, category = $4, is_active = $5, is_featured = $6, is_locked = $7,
    confidence_overall = $8, verification_status = $9, freshness_score = $10,
    override_source = $11, editorial_overrides = $12,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListWikiPagesIntelKeyset :many
SELECT * FROM wiki_pages
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- language_test_requirements (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateLanguageTestRequirementIntel :one
INSERT INTO language_test_requirements (
  tenant_id, destination_space_id, visa_category, test_id, minimum_score,
  score_breakdown, notes, source_url, status, is_active,
  intel_record_id, guide_id, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17
) RETURNING *;

-- name: GetLanguageTestRequirementByIntelRecord :one
SELECT * FROM language_test_requirements WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateLanguageTestRequirementIntel :exec
UPDATE language_test_requirements
SET visa_category = $3, minimum_score = $4, score_breakdown = $5, notes = $6,
    source_url = $7, status = $8, is_active = $9,
    confidence_overall = $10, verification_status = $11, freshness_score = $12,
    override_source = $13, editorial_overrides = $14,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListLanguageTestRequirementsIntelKeyset :many
SELECT * FROM language_test_requirements
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- name: GetLanguageTestByName :one
-- language_tests is a global reference table (no tenant_id), so the
-- tenantID parameter is accepted for interface consistency but not used
-- in the filter. The promoter resolves the language_tests(id) FK from a
-- test_name extracted as free text.
SELECT * FROM language_tests WHERE name = $1 LIMIT 1;

-- ===========================================================================
-- tax_obligation_guides (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateTaxObligationGuideIntel :one
INSERT INTO tax_obligation_guides (
  tenant_id, origin_space_id, destination_space_id, employment_status,
  residency_trigger, worldwide_income_taxed, first_year_obligations,
  action_checklist, status, is_active,
  intel_record_id, guide_id, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17
) RETURNING *;

-- name: GetTaxObligationGuideByIntelRecord :one
SELECT * FROM tax_obligation_guides WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateTaxObligationGuideIntel :exec
UPDATE tax_obligation_guides
SET employment_status = $3, residency_trigger = $4, worldwide_income_taxed = $5,
    first_year_obligations = $6, action_checklist = $7, status = $8, is_active = $9,
    confidence_overall = $10, verification_status = $11, freshness_score = $12,
    override_source = $13, editorial_overrides = $14,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListTaxObligationGuidesIntelKeyset :many
SELECT * FROM tax_obligation_guides
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;

-- ===========================================================================
-- community_organisations (intelligence columns added by migration 000469)
-- ===========================================================================

-- name: CreateCommunityOrganisationIntel :one
INSERT INTO community_organisations (
  tenant_id, name, slug, org_type, origin_community, destination_space_id,
  city_space_id, address, latitude, longitude, website_url,
  meeting_frequency, contact_info, status, is_active,
  intel_record_id, guide_id, confidence_overall, verification_status,
  freshness_score, override_source, editorial_overrides
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9, $10,
  $11, $12, $13, $14, $15, $16, $17, $18, $19, $20, $21, $22
) RETURNING *;

-- name: GetCommunityOrganisationByIntelRecord :one
SELECT * FROM community_organisations WHERE tenant_id = $1 AND intel_record_id = $2;

-- name: UpdateCommunityOrganisationIntel :exec
UPDATE community_organisations
SET name = $3, org_type = $4, origin_community = $5, address = $6,
    latitude = $7, longitude = $8, website_url = $9, meeting_frequency = $10,
    contact_info = $11, status = $12, is_active = $13,
    confidence_overall = $14, verification_status = $15, freshness_score = $16,
    override_source = $17, editorial_overrides = $18,
    updated_at = now()
WHERE tenant_id = $1 AND id = $2;

-- name: ListCommunityOrganisationsIntelKeyset :many
SELECT * FROM community_organisations
WHERE tenant_id = $1
  AND intel_record_id IS NOT NULL
  AND ($2::timestamptz IS NULL
       OR created_at < $2
       OR (created_at = $2 AND id < $3))
ORDER BY created_at DESC, id DESC
LIMIT $4;
