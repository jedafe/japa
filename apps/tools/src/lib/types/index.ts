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
	| 'processing-times'
	| 'credential-checker'
	| 'cv-formatter'
	| 'housing-guide'
	| 'attestation-guide'
	| 'language-test'
	| 'tax-calculator'
	| 'community-locator'
	// SEO growth tools (§§107–110, ADR-118..121).
	| 'crs-calculator'
	| 'pof-calculator'
	| 'eca-guide'
	| 'sop-generator'
	| 'currency-converter';

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
	/** Effective RBAC permission nodes resolved by the backend (§26). Present
	 *  on /me so frontend pages gate UI on the actual node (e.g.
	 *  `appointment_alert.premium`) instead of hardcoded role names (Rule 16). */
	permissions?: string[];
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
	/** Maps option value -> raw score. Stripped from public form responses. */
	option_scores?: Record<string, number>;
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
	geo_space_slug: string;
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

export interface SourceAttribution {
	id: string;
	name: string;
	slug: string;
	logo_url?: string | null;
	source_url: string;
	trust_level: number;
}

export interface Scholarship {
	id: string;
	/** Canonical contents guide id — used for detail/save/status routes. */
	guide_id: string;
	title: string;
	slug?: string;
	summary?: string | null;
	description?: string | null;
	institution?: string | null;
	country?: string | null;
	level_of_study?: string | null;
	award_amount_usd?: number | null;
	is_renewable?: boolean;
	application_deadline?: string | null;
	/** URL to the scholarship provider's application page. */
	application_url?: string | null;
	field_values?: Record<string, string>;
	tags?: string[];
	source?: SourceAttribution;
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
	website_url?: string | null;
	data_source?: string;
	is_active: boolean;
	/** §112 — mid-market benchmark provider. Reference only — never bookable. */
	is_benchmark?: boolean;
	display_order?: number;
	created_at?: string;
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
	/** §112 — mid-market benchmark row (seeded 'mid-market' provider). Not a
	 *  bookable provider quote — display with a badge, exclude from best-rate. */
	is_benchmark?: boolean;
	source?: SourceAttribution;
}

export interface RemittanceCorridor {
	send_currency: string;
	receive_currency: string;
}

export interface RemittanceCalculation {
	send_amount: number;
	send_currency: string;
	receive_currency: string;
	/** Per-provider calculation results. */
	results: RemittanceCalculationRow[];
	/** §112 — mid-market benchmark reference; carries no fees and is never
	 *  ranked against the provider results. */
	benchmark?: RemittanceBenchmark | null;
}

/** §112 — mid-market benchmark reference row in the calculator response. */
export interface RemittanceBenchmark {
	provider_id: string;
	provider_name: string;
	rate: number;
	receive_amount: number;
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
	/** Entity type: 'content', 'forum_thread', 'space', 'academy_track'. */
	entity_type: string;
	entity_id: string;
	/** Similarity score from the retrieval ranking. */
	similarity?: number;
	/** Source tier label — 'editorial' or 'community' (§69, ADR-069 §4). */
	tier?: string;
	/**
	 * §69 Citation Source Contract — resolvable link + label emitted by the
	 * backend. Empty when the entity row no longer resolves; render a
	 * non-linking fallback in that case. Never derive hrefs from entity_id.
	 */
	title?: string;
	slug?: string;
	url?: string;
}

/**
 * Conversation summary returned by GET /ai/conversations. The list endpoint
 * intentionally omits messages (BUG-005) — fetch /ai/conversations/{id} for
 * the full turn history.
 */
export interface ChatConversationSummary {
	conversation_id: string;
	preview: string;
	turn_count: number;
	last_entry_at: string;
}

/** Single Honcho context entry inside GET /ai/conversations/{id}. */
export interface ChatContextEntry {
	id: string;
	user_id: string;
	content: string;
	metadata?: { role?: string; conversation_id?: string; [k: string]: unknown } | null;
	created_at: string;
}

export interface ChatConversationDetail {
	conversation_id: string;
	entries: ChatContextEntry[];
	turn_count: number;
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

/**
 * GET /me/arrival-checklists/{guide_id} — same item fields plus the user's
 * progress merged in (§92 progress persistence).
 */
export interface ArrivalChecklistUserItem extends ArrivalChecklistItem {
	completed: boolean;
	completed_at?: string | null;
	notes?: string | null;
}

/**
 * GET /me/arrival-checklists — a guide the user has progress on, with
 * completion counts (§92 progress persistence).
 */
export interface UserArrivalChecklist {
	guide_id: string;
	destination_space_name?: string | null;
	destination_space_code?: string | null;
	visa_category?: string | null;
	applicant_type: string;
	total_items: number;
	completed_items: number;
	first_progress_at?: string | null;
	last_progress_at?: string | null;
}

/* --- §93 Processing Time Tracker (reuses §83 Visa Timeline backend) --- */
// GET /visa-timelines/stats, GET /visa-timelines/stats/trend

export interface ProcessingStat {
	id?: string;
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

/* --- §83 Visa Timeline Tracker — personal tracker (/me/visa-timelines) --- */

export interface VisaTimelineMilestone {
	id: string;
	timeline_id: string;
	milestone_type: string;
	occurred_at: string;
	notes?: string | null;
	created_at: string;
}

export interface VisaTimeline {
	id: string;
	user_id: string;
	origin_space_id?: string | null;
	destination_space_id: string;
	visa_category: string;
	consulate?: string | null;
	application_method?: string | null;
	outcome?: string | null;
	is_shared: boolean;
	anonymised_at?: string | null;
	created_at: string;
	updated_at: string;
	milestones?: VisaTimelineMilestone[] | null;
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
	/** Slug of the linked space — public space URLs are /spaces/{slug}. */
	japaconnect_space_slug?: string | null;
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

/** Generic value/label option returned by backend catalogs (Rule 16). */
export interface CatalogOption {
	value: string;
	label: string;
	default?: boolean;
}

/** Org-type allow-list entry from GET /community-locator/org-types (Rule 16). */
export interface OrgTypeOption extends CatalogOption {}

/* --- Geo Space reference (for country/city selectors) --- */
// Used by all V2 tools for country selectors (fetched from /geo/countries)
// and city selectors (fetched from /spaces?level=city).

/** GET /api/v1/geo/countries — a country-level geo space (ADR-005). */
export interface GeoCountry {
	id: string;
	name: string;
	slug: string;
	geo_code: string;
	level: 'country';
	member_count: number;
	post_count: number;
	follower_count: number;
}

export interface GeoSpaceOption {
	id: string;
	name: string;
	slug: string;
	code?: string | null;
}

/* --- SEO Growth Tools (§§107–110, ADR-118..121) --- */

/** GET /crs/factors — a versioned CRS ruleset + factor point tables. */
export interface CrsRuleset {
	id: string;
	version_label: string;
	effective_date: string;
	status: string;
	source_url?: string;
}

export interface CrsFactor {
	group: string;
	key: string;
	input_descriptor: Record<string, unknown>;
	label: string;
	points_single: number;
	points_with_spouse?: number;
	sort_order: number;
}

export interface CrsFactorsResponse {
	ruleset: CrsRuleset;
	factors: CrsFactor[];
}

/** POST /crs/calculate request. */
export interface CrsCalculateInput {
	marital_status: 'single' | 'married';
	age: number;
	education: string;
	clb_first: { listening: number; reading: number; writing: number; speaking: number };
	clb_second?: { listening: number; reading: number; writing: number; speaking: number };
	canadian_experience_years: number;
	foreign_experience_years: number;
	certificate_of_qualification: boolean;
	spouse?: {
		education: string;
		clb_first: { listening: number; reading: number; writing: number; speaking: number };
		canadian_experience_years: number;
	};
	provincial_nomination: boolean;
	job_offer: boolean;
	sibling_in_canada: boolean;
	french_clb?: { listening: number; reading: number; writing: number; speaking: number };
	canadian_education: string;
	ruleset_version?: string;
}

/** POST /crs/calculate response. */
export interface CrsBreakdownItem {
	group: string;
	key: string;
	label: string;
	points: number;
}

export interface CrsCalculateResult {
	ruleset_version: string;
	score: number;
	breakdown: CrsBreakdownItem[];
	saved: boolean;
}

/** GET /crs/draws + /crs/draws/latest — Express Entry draw history. */
export interface CrsDraw {
	id: string;
	draw_number?: number;
	draw_date: string;
	category: string;
	crs_cutoff: number;
	invitations_issued: number;
	tie_break_rule?: string;
	source_url?: string;
}

/** GET /me/crs-scores — saved score history (auth). */
export interface CrsScoreHistoryItem {
	id: string;
	score: number;
	inputs: Record<string, unknown>;
	breakdown: CrsBreakdownItem[];
	created_at: string;
}

/** GET /pof/requirements — a proof-of-funds requirement row. */
export interface PofRequirement {
	id: string;
	destination_space_id: string;
	destination_space_name?: string;
	destination_space_code?: string;
	visa_type: string;
	program_label: string;
	amount_schedule: PofScheduleBand[] | PofScheduleFormula;
	currency: string;
	funds_must_be_liquid: boolean;
	accepted_proof?: string[];
	notes?: string;
	effective_date: string;
	source_url?: string;
	status: string;
	is_active: boolean;
	ai_generated: boolean;
	is_japabot_indexed: boolean;
}

export interface PofScheduleBand {
	members: number;
	amount: number;
	per_additional?: number;
}

export interface PofScheduleFormula {
	base: number;
	base_members: number;
	per_additional: number;
}

/** GET /pof/calculate response. */
export interface PofCalculateResult {
	requirement_id: string;
	program_label: string;
	household: number;
	amount: number;
	currency: string;
	ngn_equivalent?: number;
	usd_equivalent?: number;
	funds_must_be_liquid: boolean;
	accepted_proof?: string[];
	notes?: string;
	effective_date: string;
	source_url?: string;
}

/** GET /eca/providers — an ECA provider directory entry. */
export interface EcaProvider {
	id: string;
	slug: string;
	name: string;
	website?: string;
	is_active?: boolean;
}

/** GET /eca/guides + /eca/guides/:id — an ECA guide. */
export interface EcaGuide {
	id: string;
	destination_space_id: string;
	provider_id: string;
	provider_slug?: string;
	provider_name?: string;
	purpose: string;
	pathway_steps: EcaPathwayStep[];
	fee_estimate?: string;
	currency?: string;
	timeline_estimate?: string;
	documents_required?: string[];
	accepted_for?: string[];
	official_url?: string;
	source_url_last_checked?: string;
	status: string;
	is_active: boolean;
	ai_generated: boolean;
	is_japabot_indexed: boolean;
}

export interface EcaPathwayStep {
	step: number;
	title: string;
	detail: string;
	duration?: string;
}

/** GET /eca/guides/:id — guide detail + approved community updates. */
export interface EcaGuideDetail {
	guide: EcaGuide;
	updates: EcaCommunityUpdate[];
}

export interface EcaCommunityUpdate {
	id: string;
	guide_id: string;
	user_name?: string;
	step_index?: number;
	fee_estimate?: string;
	turnaround_days?: number;
	note?: string;
	status: string;
	created_at: string;
}

/** GET /eca/compare — side-by-side provider comparison. */
export interface EcaCompareResponse {
	providers: EcaProvider[];
	guides: EcaGuide[];
}

/** GET /sop/templates — a public SOP template. */
export interface SopTemplate {
	id: string;
	destination_space_id?: string;
	purpose: string;
	title: string;
	structure: SopTemplateSection[];
}

export interface SopTemplateSection {
	section: string;
	guidance: string;
	min_words?: number;
}

/** GET /sop/samples + /sop/samples/:id — a published SOP sample. */
export interface SopSample {
	id: string;
	destination_space_name?: string;
	program_level: string;
	field_of_study?: string;
	title: string;
	body: string;
	word_count?: number;
	ai_generated: boolean;
}

/** GET /sop/drafts + /sop/drafts/:id — a user-owned draft. */
export interface SopDraft {
	id: string;
	destination_space_id?: string;
	purpose: string;
	program?: string;
	institution?: string;
	title?: string;
	inputs: Record<string, unknown>;
	generated_text?: string;
	status: 'queued' | 'generated' | 'failed';
	ai_model?: string;
	version: number;
	created_at: string;
	updated_at: string;
}

/** GET /sop/quota — remaining monthly generations (auth). */
export interface SopQuota {
	limit: number;
	used: number;
	remaining: number;
}

/* --- §112 FX Currency Converter (ADR-125) --- */
// GET /fx/currencies, GET /fx/convert?amount=&from=&to=

/** GET /fx/currencies — picker currency row (matrix-covered codes). */
export interface FxCurrency {
	code: string;
	name: string;
	symbol: string;
}

/** GET /fx/convert?amount=&from=&to= — mid-market conversion result. */
export interface FxConvertResult {
	amount: number;
	from: string;
	to: string;
	/** Quote units per 1 unit of `from`. */
	rate: number;
	converted: number;
	/** When the underlying matrix row was fetched. */
	fetched_at: string;
	/** True when the serving matrix row is older than the freshness window —
	 *  render a staleness warning (§112). */
	stale: boolean;
}
