/**
 * Shared domain + transport types for the Tools App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API
 * at internal/tools/handlers.go and the relevant feature handlers).
 *
 * Ref: ADR-086 (multi-surface architecture), §78 (Tools Web Surface),
 *      §70 (Visa Checklist), §71 (Readiness Assessment), §72 (Cost of Living),
 *      §73 (Scholarship Finder), §74 (Remittance Tracker), §69/§20 (JapaBot).
 */

export type ThemeName = 'light' | 'dark';

/** Supported UI locale code (e.g. 'en', 'fr', 'yo'). Dynamic per ADR-013. */
export type Locale = string;

/**
 * ADR-086 / §78: which tool surface the request host maps to.
 * 'hub' renders the full tools listing; a tool slug renders the single-tool shell.
 */
export type ToolSurface =
	| 'hub'
	| 'visa-checklist'
	| 'readiness'
	| 'cost-of-living'
	| 'scholarships'
	| 'remittance'
	| 'japabot'
	// V2 functional tools (§§90–97, §§99–101) — geo-aware tool pattern (ADR-088).
	| 'visa-routes'
	| 'embassy-alerter'
	| 'arrival-checklist'
	| 'processing-tracker'
	| 'credential-checker'
	| 'cv-formatter'
	| 'housing-guide'
	| 'attestation-guide'
	| 'language-test'
	| 'tax-calculator'
	| 'community-locator';

/* ------------------------------------------------------------------ *
 * Transport envelope
 * ------------------------------------------------------------------ */

/** Success envelope: every endpoint wraps its payload in `{ data }`. */
export interface ApiOk<T> {
	data: T;
	meta?: ListMeta;
}

/** Error envelope: `{ error: { code, message } }`. */
export interface ApiErr {
	error: { code: string; message: string; status?: number };
}

export interface ListMeta {
	total?: number;
	page?: number;
	limit?: number;
	next_cursor?: string | null;
}

/* ------------------------------------------------------------------ *
 * Session / user
 * ------------------------------------------------------------------ */

export interface SessionUser {
	id: string;
	email: string;
	username: string;
	displayName: string;
	avatar_url?: string | null;
	groups?: string[];
	primary_space?: SpaceRef | null;
	ui?: { nav_collapsed?: boolean } & Record<string, unknown>;
	is_pro?: boolean;
}

export interface SpaceRef {
	id: string;
	slug: string;
	name: string;
}

/* ------------------------------------------------------------------ *
 * §70 — Visa Document Checklist
 *   GET /checklist/selector, GET /checklist/:guide_id,
 *   GET /checklist/:guide_id/progress,
 *   PATCH /checklist/:guide_id/progress/:item_key,
 *   DELETE /checklist/:guide_id/progress
 * ------------------------------------------------------------------ */

export interface ChecklistSelector {
	/** Origin country Geo Spaces the user can select from. */
	origins: ChecklistCountryOption[];
	/** Destination country Geo Spaces with published visa guides. */
	destinations: ChecklistCountryOption[];
}

export interface ChecklistCountryOption {
	id: string;
	name: string;
	slug: string;
	/** Visa types available for this destination (destinations only). */
	visa_types?: ChecklistVisaTypeOption[];
}

export interface ChecklistVisaTypeOption {
	/** The visa_type custom field value, e.g. "student", "work", "tourist". */
	value: string;
	label: string;
	/** The guide (content) ID that matches this origin→destination→visa_type. */
	guide_id: string;
}

export interface ChecklistGuide {
	guide_id: string;
	title: string;
	/** Document items extracted from the guide's checklist custom field. */
	items: ChecklistItem[];
}

export interface ChecklistItem {
	/** Stable slug derived from the document label. */
	item_key: string;
	label: string;
	description?: string | null;
	/** Required vs recommended. */
	required?: boolean;
	/** Category for grouping (e.g. "Identity", "Financial", "Academic"). */
	category?: string | null;
}

export interface ChecklistProgress {
	guide_id: string;
	items: ChecklistProgressItem[];
	completed_count: number;
	total_count: number;
}

export interface ChecklistProgressItem {
	item_key: string;
	completed: boolean;
	completed_at?: string | null;
	notes?: string | null;
}

/* ------------------------------------------------------------------ *
 * §71 — Japa Readiness Assessment
 *   GET /assessment, POST /assessment/submit,
 *   GET /assessment/result/:id, GET /assessment/result/by-submission/:id,
 *   GET /assessment/history
 *
 * The assessment is built on the §06 Form Builder: a forms row
 * (form_type='assessment') with a flat list of custom_fields. Scoring runs
 * server-side in an Asynq task; submit returns a submission_id and the
 * client polls /result/by-submission/:id until the result is written.
 * ------------------------------------------------------------------ */

export interface AssessmentForm {
	id: string;
	name: string;
	slug: string;
	description?: string | null;
	is_multi_step: boolean;
	submit_label: string;
	status: string;
	max_score: number;
	thresholds: AssessmentThreshold[];
	fields: AssessmentField[];
	scoring_config: {
		max_score: number;
		thresholds: AssessmentThreshold[];
	};
}

export interface AssessmentThreshold {
	min: number;
	max: number;
	label: string;
	/** Optional CSS color for the gauge / badge. */
	color?: string | null;
}

/** A single scored question, backed by a custom_field row. */
export interface AssessmentField {
	id: string;
	label: string;
	/** Field key — the field_values map is keyed by this. */
	key: string;
	/** Custom-field type: 'select' | 'multiselect' | 'number' | 'rating' | 'text' ... */
	field_type: string;
	is_required: boolean;
	position: number;
	placeholder: string;
	/** Options for select/multiselect fields: [{value, label}]. */
	options: AssessmentFieldOption[];
	/** Per-field scoring metadata (weight, dimension, option_scores). */
	config: AssessmentFieldConfig;
}

export interface AssessmentFieldOption {
	value: string;
	label: string;
}

export interface AssessmentFieldConfig {
	weight: number;
	dimension: string;
	/** Maps option value -> raw score (keys match options[].value). */
	option_scores: Record<string, number>;
}

/** POST /assessment/submit response — the result is produced async. */
export interface AssessmentSubmitResponse {
	submission_id: string;
	status: string;
}

/** GET /assessment/result/by-submission/:id while scoring is in flight. */
export interface AssessmentResultPending {
	status: 'pending';
}

export interface AssessmentResult {
	id: string;
	total_score: number;
	threshold_label: string;
	dimension_scores: Record<string, number>;
	roadmap: AssessmentRoadmapStep[];
	user_id: string;
	submission_id: string;
	created_at: string;
}

export interface AssessmentRoadmapStep {
	title: string;
	description?: string | null;
	/** Platform content slug the step links to (guide/tool/track). */
	content_slug?: string;
	dimension?: string | null;
	priority?: number;
}

/** History list items are summaries — fetch the full result by id to render. */
export interface AssessmentHistoryItem {
	id: string;
	total_score: number;
	threshold_label: string;
	submission_id: string;
	created_at: string;
}

/* ------------------------------------------------------------------ *
 * §72 — Cost of Living Calculator
 *   GET /calculator/cost-of-living/cities,
 *   GET /calculator/cost-of-living/compare?city_ids=
 * ------------------------------------------------------------------ */

export interface ColCity {
	id: string;
	title: string;
	description: string;
	geo_space_name: string;
	geo_space_slug: string;
	is_baseline: boolean;
	last_updated: string;
	position: number;
}

export interface ColCitiesListResponse {
	cities: ColCity[];
	total: number;
	page: number;
	limit: number;
}

export interface ColCityData {
	id: string;
	title: string;
	geo_space_name: string;
	is_baseline: boolean;
	last_updated: string;
	values: Record<string, string>;
}

export interface ColCategoryData {
	key: string;
	label: string;
	values: Record<string, number>;
	ratios: Record<string, number>;
}

export interface ColComparisonMatrix {
	cities: ColCityData[];
	categories: ColCategoryData[];
	baseline_city_id: string;
	overall_index: Record<string, number>;
}

/* ------------------------------------------------------------------ *
 * §73 — Scholarship Finder
 *   GET /scholarships, GET /scholarships/:guide_id,
 *   GET /scholarships/saved, POST/PATCH/DELETE /scholarships/:guide_id/save
 * ------------------------------------------------------------------ */

export type ScholarshipStatus =
	| 'saved'
	| 'researching'
	| 'applied'
	| 'accepted'
	| 'rejected'
	| 'withdrawn';

export interface Scholarship {
	id: string;
	title: string;
	institution?: string | null;
	description?: string | null;
	country?: string | null;
	level_of_study?: string | null;
	award_amount_usd?: number | null;
	is_renewable?: boolean;
	application_deadline?: string | null;
	/** URL to the scholarship provider's application page. */
	application_url?: string | null;
	tags?: string[];
}

export interface ScholarshipDetail extends Scholarship {
	eligibility?: string | null;
	how_to_apply?: string | null;
	contact_email?: string | null;
}

export interface SavedScholarship {
	id: string;
	scholarship: Scholarship;
	status: ScholarshipStatus;
	notes?: string | null;
	remind_before_days: number;
	last_reminded_at?: string | null;
	created_at: string;
	updated_at: string;
}

/* ------------------------------------------------------------------ *
 * §74 — Remittance Rate Tracker
 *   GET /remittance/rates, /calculate, /history, /providers, /corridors,
 *   GET/POST/PATCH/DELETE /remittance/alerts
 * ------------------------------------------------------------------ */

export interface RemittanceProvider {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
	website_url?: string | null;
	is_active: boolean;
}

export interface RemittanceRate {
	id: string;
	provider_id: string;
	provider_name: string;
	send_currency: string;
	receive_currency: string;
	rate: number;
	fee_flat?: number | null;
	fee_pct?: number | null;
	transfer_time?: string | null;
	fetched_at: string;
	is_manual: boolean;
}

export interface RemittanceCorridor {
	send_currency: string;
	receive_currency: string;
	/** Label for display, e.g. "GBP → NGN". */
	label: string;
}

export interface RemittanceCalculation {
	send_amount: number;
	send_currency: string;
	receive_currency: string;
	/** Per-provider calculation results. */
	results: RemittanceCalculationRow[];
}

export interface RemittanceCalculationRow {
	provider_id: string;
	provider_name: string;
	rate: number;
	fee_flat: number;
	fee_pct: number;
	/** Amount received in the destination currency. */
	receive_amount: number;
	transfer_time?: string | null;
}

export interface RemittanceHistoryPoint {
	/** ISO date string (YYYY-MM-DD) for the day bucket. */
	day: string;
	max_rate: number;
	min_rate: number;
	avg_rate: number;
}

export interface RemittanceAlert {
	id: string;
	send_currency: string;
	receive_currency: string;
	target_rate: number;
	direction: 'above' | 'below';
	is_active: boolean;
	last_triggered_at?: string | null;
	created_at: string;
}

/* ------------------------------------------------------------------ *
 * §20 / §69 — JapaBot Chat
 *   POST /ai/chat, GET /ai/conversations, GET /ai/conversations/:id
 * ------------------------------------------------------------------ */

export interface ChatMessage {
	id: string;
	role: 'user' | 'assistant';
	content: string;
	created_at: string;
	/** RAG source citations (assistant messages only — §69). */
	sources?: ChatSource[];
}

export interface ChatSource {
	/** Entity type: 'content', 'forum_thread', 'space', 'learning_track'. */
	entity_type: string;
	entity_id: string;
	title: string;
	url: string;
	/** Tier 1 (editorial) or Tier 2 (community) — §69. */
	tier: 1 | 2;
}

export interface ChatConversation {
	id: string;
	title?: string | null;
	created_at: string;
	updated_at: string;
	messages: ChatMessage[];
}

export interface ChatResponse {
	reply: string;
	conversation_id: string;
	sources?: ChatSource[];
}

/* ================================================================== *
 * V2 Functional Tools (§§90–97, §§99–101) — Geo-Aware Tool Pattern
 * Ref: ADR-088 (universal AI content generation), .jeos/skills/geo-aware-tool-pattern.md
 * All endpoints accept ?dest={space_id} as the primary data axis.
 * ================================================================== */

/** Common pagination envelope returned by V2 list endpoints. */
export interface V2Pagination {
	page: number;
	limit: number;
	total: number;
}

/** Common AI generation metadata (ADR-088). Present on all admin-managed content. */
export interface AiGenerationMeta {
	ai_generated: boolean;
	ai_model?: string | null;
	ai_prompt_hash?: string | null;
	hitl_reviewed_by?: string | null;
	hitl_reviewed_at?: string | null;
	hitl_review_notes?: string | null;
	is_japabot_indexed: boolean;
}

/* --- §90 Visa Route Comparison --- */
// GET /visa-routes, GET /visa-routes/compare, GET /visa-routes/{id}

export interface VisaRoute {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category: string;
	name: string;
	slug: string;
	eligibility_summary?: string | null;
	requirements?: Record<string, unknown> | null;
	typical_fee_usd?: number | null;
	path_to_pr?: string | null;
	renewal_terms?: string | null;
	restrictions?: string | null;
	gov_source_url: string;
	source_url_last_checked?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export interface VisaRouteWithAi extends VisaRoute, AiGenerationMeta {}

/* --- §91 Embassy Appointment Alerter --- */
// GET /embassies, GET/POST/PATCH/DELETE /me/appointment-alerts

export interface Embassy {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	slug: string;
	name: string;
	booking_url: string;
	poll_interval_seconds: number;
	is_active: boolean;
	last_polled_at?: string | null;
	created_at: string;
	updated_at: string;
}

export interface AppointmentAlert {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	embassy_slug: string;
	visa_category?: string | null;
	date_range_start: string;
	date_range_end: string;
	status: string;
	last_checked_at?: string | null;
	fired_at?: string | null;
	expires_at?: string | null;
	notification_channels: string[];
	created_at: string;
	updated_at: string;
}

/* --- §92 Arrival Checklist --- */
// GET /arrival-checklist/selector, GET /arrival-checklist/{guide_id}

export interface ArrivalChecklistSelectorEntry {
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	guide_count: number;
	/** The first published guide ID for this dest+visa combo — used to fetch
	 * items via GET /arrival-checklist/{guide_id} without a separate list call. */
	guide_id: string;
}

export interface ArrivalChecklistItem {
	id: string;
	guide_id: string;
	phase: string;
	title: string;
	description?: string | null;
	official_link?: string | null;
	origin_country_modifier?: Record<string, unknown> | null;
	sort_order: number;
	is_flagged: boolean;
	created_at: string;
}

/* --- §93 Processing Time Tracker (reuses §83 Visa Timeline backend) --- */
// GET /visa-timelines/stats, GET /visa-timelines/stats/trend

export interface ProcessingStat {
	origin_space_id?: string | null;
	destination_space_id: string;
	visa_category: string;
	consulate?: string | null;
	period_start: string;
	sample_count: number;
	avg_days?: number | null;
	median_days?: number | null;
	p10_days?: number | null;
	p90_days?: number | null;
	refreshed_at: string;
}

export interface ProcessingTrendPoint {
	period_start: string;
	sample_count: number;
	avg_days?: number | null;
	median_days?: number | null;
}

/* --- §94 Professional Credential Checker --- */
// GET /credentials/professions, GET /credentials, GET /credentials/{id},
// POST /credentials/{id}/notes

export interface CredentialProfession {
	id: string;
	slug: string;
	name: string;
	category?: string | null;
}

export interface CredentialPathway {
	id: string;
	profession_id: string;
	profession_slug: string;
	profession_name: string;
	profession_category?: string | null;
	issuing_space_id: string;
	issuing_space_name?: string | null;
	issuing_space_code?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	regulatory_body: string;
	regulatory_body_url?: string | null;
	pathway_type: string;
	timeline_estimate?: string | null;
	cost_estimate_usd?: string | null;
	pathway_description: string;
	official_application_url?: string | null;
	source_url_last_checked?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export interface CredentialPathwayWithAi extends CredentialPathway, AiGenerationMeta {}

export interface CredentialCommunityNote {
	id: string;
	pathway_id: string;
	user_id: string;
	user_name?: string | null;
	note: string;
	upvotes: number;
	status: string;
	created_at: string;
}

export interface CredentialPathwayDetail {
	pathway: CredentialPathwayWithAi;
	notes: CredentialCommunityNote[];
}

/* --- §95 CV Formatter --- */
// GET /cv-formatter/standards, GET /cv-formatter/standards/{dest},
// POST /cv-formatter/format, GET /cv-formatter/sessions/{id}, GET /cv-formatter/sessions/{id}/download

export interface CvStandard {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	recommended_length_pages?: string | null;
	photo_policy?: string | null;
	date_format?: string | null;
	recommended_sections?: string[] | null;
	tone_guidance?: string | null;
	common_pitfalls?: string | null;
	created_at: string;
	updated_at: string;
}

export interface CvFormatSession {
	id: string;
	user_id: string;
	origin_space_id?: string | null;
	destination_space_id: string;
	input_text: string;
	output_text?: string | null;
	output_format: string;
	status: string;
	ai_model?: string | null;
	tokens_used?: number | null;
	created_at: string;
	expires_at: string;
}

/* --- §96 Housing Guide --- */
// GET /housing-guide, GET /housing-guide/{id}

export interface HousingGuide {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	city_space_id?: string | null;
	city_space_name?: string | null;
	city_space_code?: string | null;
	accommodation_type?: string | null;
	search_platforms?: string[] | Record<string, unknown> | null;
	landlord_requirements?: string | null;
	rent_ranges?: Record<string, unknown> | null;
	scam_categories?: string[] | null;
	tips?: string | null;
	status: string;
	is_active: boolean;
	last_verified_at?: string | null;
	reviewer_name?: string | null;
	created_at: string;
	updated_at: string;
}

export interface HousingGuideWithAi extends HousingGuide, AiGenerationMeta {}

/* --- §97 Attestation Guide --- */
// GET /attestation, GET /attestation/{id}, GET /attestation/{id}/export/pdf,
// POST /attestation/{id}/updates

export interface AttestationPathway {
	id: string;
	document_type: string;
	issuing_space_id: string;
	issuing_space_name?: string | null;
	issuing_space_code?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	apostille_required: boolean;
	pathway_steps: unknown[] | Record<string, unknown>;
	community_last_verified?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export interface AttestationPathwayWithAi extends AttestationPathway, AiGenerationMeta {}

export interface AttestationCommunityUpdate {
	id: string;
	pathway_id: string;
	user_id: string;
	step_index?: number | null;
	fee_estimate?: string | null;
	turnaround_days?: number | null;
	note?: string | null;
	status: string;
	reviewed_by?: string | null;
	created_at: string;
}

/* --- §99 Language Test Hub --- */
// GET /language-tests, GET /language-tests/requirements,
// GET /language-tests/{test_slug}, GET /language-tests/{test_slug}/resources,
// GET /language-tests/{test_slug}/stories, POST /language-tests/{test_slug}/stories

export interface LanguageTest {
	id: string;
	slug: string;
	name: string;
	official_url?: string | null;
	validity_months?: number | null;
}

export interface LanguageTestRequirement {
	id: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	test_id: string;
	test_slug: string;
	test_name: string;
	minimum_score: string;
	score_breakdown?: Record<string, unknown> | null;
	notes?: string | null;
	source_url?: string | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export interface LanguageTestRequirementWithAi extends LanguageTestRequirement, AiGenerationMeta {}

export interface LanguageTestResource {
	id: string;
	test_id: string;
	title: string;
	resource_url: string;
	resource_type?: string | null;
	community_rating?: string | null;
	rating_count: number;
	created_at: string;
}

export interface LanguageTestScoreStory {
	id: string;
	user_id: string;
	test_id: string;
	starting_level?: string | null;
	final_score: string;
	destination_space_id?: string | null;
	destination_space_name?: string | null;
	prep_duration_weeks?: number | null;
	resources_used?: string[] | null;
	advice?: string | null;
	status: string;
	created_at: string;
	author_name?: string | null;
}

export interface LanguageTestHubPage {
	test: LanguageTest;
	resources_summary: LanguageTestResource[];
	stories_summary: LanguageTestScoreStory[];
}

/* --- §100 Tax Calculator --- */
// GET /tax-calculator, GET /tax-calculator/treaty

export interface TaxGuide {
	id: string;
	origin_space_id: string;
	origin_space_name?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	employment_status: string;
	residency_trigger?: string | null;
	worldwide_income_taxed?: boolean | null;
	first_year_obligations?: string | null;
	action_checklist?: unknown[] | Record<string, unknown> | null;
	status: string;
	is_active: boolean;
	created_at: string;
	updated_at: string;
}

export interface TaxGuideWithAi extends TaxGuide, AiGenerationMeta {}

export interface TaxTreaty {
	id: string;
	origin_space_id: string;
	origin_space_name?: string | null;
	destination_space_id: string;
	destination_space_name?: string | null;
	treaty_exists: boolean;
	treaty_summary?: string | null;
	treaty_source_url?: string | null;
	created_at: string;
	updated_at: string;
}

/* --- §101 Diaspora Community Locator --- */
// GET /community-locator, GET /community-locator/{slug},
// POST /community-locator, POST /community-locator/{id}/claim

export interface CommunityOrg {
	id: string;
	name: string;
	slug: string;
	org_type: string;
	origin_community: string;
	destination_space_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	city_space_id?: string | null;
	city_space_name?: string | null;
	city_space_code?: string | null;
	address?: string | null;
	latitude?: number | null;
	longitude?: number | null;
	website_url?: string | null;
	meeting_frequency?: string | null;
	contact_info?: string | null;
	japaconnect_space_id?: string | null;
	japaconnect_org_id?: string | null;
	claim_status: string;
	is_verified: boolean;
	is_active: boolean;
	is_community_submitted: boolean;
	status: string;
	submitted_by?: string | null;
	submitter_name?: string | null;
	reviewed_by?: string | null;
	reviewed_at?: string | null;
	created_at: string;
	updated_at: string;
}

export interface CommunityOrgWithAi extends CommunityOrg, AiGenerationMeta {}

/* --- Geo Space reference (for destination selectors) --- */
// Used by all V2 tools for the ?dest= destination selector.
// Fetched from /spaces?level=country or the tool-specific selector endpoint.

export interface GeoSpaceOption {
	id: string;
	name: string;
	slug: string;
	code?: string | null;
}
