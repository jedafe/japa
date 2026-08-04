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
 *   GET /assessment/result/:id, GET /assessment/history
 * ------------------------------------------------------------------ */

export interface AssessmentForm {
	id: string;
	title: string;
	description?: string | null;
	/** Scoring metadata exposed to the client for display. */
	scoring_config: AssessmentScoringConfig;
	sections: AssessmentSection[];
}

export interface AssessmentScoringConfig {
	max_score: number;
	thresholds: AssessmentThreshold[];
}

export interface AssessmentThreshold {
	label: string;
	min_score: number;
	/** Optional CSS color for the gauge / badge. */
	color?: string | null;
}

export interface AssessmentSection {
	id: string;
	title: string;
	description?: string | null;
	questions: AssessmentQuestion[];
}

export interface AssessmentQuestion {
	id: string;
	prompt: string;
	/** Question type — single-choice, multiple-choice, or scale. */
	type: 'single' | 'multiple' | 'scale' | 'text';
	required?: boolean;
	options?: AssessmentOption[];
	/** For scale questions: min and max values. */
	scale_min?: number;
	scale_max?: number;
	/** Dimension this question contributes to (e.g. "financial", "logistical"). */
	dimension?: string | null;
}

export interface AssessmentOption {
	id: string;
	label: string;
	/** Score value for this option. */
	value: number;
}

export interface AssessmentSubmission {
	question_id: string;
	/** Option IDs (single/multiple) or numeric/text value. */
	answer: string[] | string | number;
}

export interface AssessmentResult {
	id: string;
	total_score: number;
	threshold_label: string;
	dimension_scores: Record<string, number>;
	roadmap: AssessmentRoadmapStep[];
	created_at: string;
}

export interface AssessmentRoadmapStep {
	title: string;
	description?: string | null;
	priority?: number;
	dimension?: string | null;
	/** Optional links to platform resources (guides, tools, tracks). */
	links?: Array<{ label: string; url: string }>;
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
