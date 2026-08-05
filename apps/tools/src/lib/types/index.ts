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

/** Supported UI locales. Mirrors the i18n store's Locale type. */
export type Locale = 'en' | 'fr';

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
	| 'japabot';

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
	name: string;
	country: string;
	/** True if this is the baseline city (Lagos by default). */
	is_baseline: boolean;
	currency_code: string;
	/** ISO timestamp of last data update. */
	last_updated_at?: string | null;
}

export interface ColComparison {
	/** The baseline city the ratios are computed against. */
	baseline: ColComparisonCity;
	cities: ColComparisonCity[];
	/** Attribute definitions (labels, units). */
	attributes: ColAttributeDef[];
}

export interface ColComparisonCity extends ColCity {
	/** Attribute values keyed by attribute slug. Values in USD. */
	attributes: Record<string, number | null>;
	/** Ratios vs baseline, keyed by attribute slug (1.0 = same as baseline). */
	ratios?: Record<string, number | null>;
}

export interface ColAttributeDef {
	slug: string;
	label: string;
	unit: string;
	/** Category for grouping. */
	category?: string | null;
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
	timestamp: string;
	provider_id: string;
	provider_name: string;
	rate: number;
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
