/**
 * Shared domain + transport types for the Academy App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API
 * at internal/academy/handlers.go and internal/academy/creator/handlers.go).
 *
 * Ref: ADR-060 (Academy shell), ADR-063 (Japa Academy creator economy),
 *      sections/36 (Academy Platform).
 */

export type ThemeName = 'light' | 'dark';

/** Supported UI locale code (e.g. 'en', 'fr', 'yo'). Dynamic per ADR-013. */
export type Locale = string;

/** GET /api/v1/geo/countries — a country-level geo space (ADR-005). */
export interface GeoCountry {
	id: string;
	name: string;
	slug: string;
	geo_code: string;
	level: 'country';
	currency_code?: string | null;
	member_count: number;
	post_count: number;
	follower_count: number;
}

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
 * Notifications (realtime badge — minimal shape for the Academy shell)
 * ------------------------------------------------------------------ */

export interface Notification {
	id: string;
	type: string;
	body?: string;
	created_at?: string;
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
	/** UI preferences persisted via PATCH /me/account, e.g. nav_collapsed. */
	ui?: { nav_collapsed?: boolean } & Record<string, unknown>;
	/** Pro subscription status — drives "Included with Pro" pricing (ADR-063 §7). */
	is_pro?: boolean;
}

export interface SpaceRef {
	id: string;
	slug: string;
	name: string;
}

/** GET /me/spaces item — a space the user belongs to. Lean shape for the nav switcher. */
export interface UserSpaceRef {
	id: string;
	slug: string;
	name: string;
}

/* ------------------------------------------------------------------ *
 * Navigation menus (ADR-054) — GET /menus/{slug}
 * ------------------------------------------------------------------ */

export type MenuPlacement = 'sidebar' | 'bottom_bar' | 'dropdown' | 'footer' | 'header';
export type MenuVisibility = 'all' | 'authenticated' | 'guest';

export interface MenuItem {
	id: string;
	label: string;
	url: string;
	/** Lucide icon name in kebab-case, e.g. "graduation-cap". */
	icon: string | null;
	sort_order: number;
	visibility: MenuVisibility;
	target: '_self' | '_blank';
	permission_node?: string | null;
	is_ad_slot: boolean;
	ad_slot_config?: Record<string, unknown> | null;
	children?: MenuItem[];
}

export interface Menu {
	id: string;
	slug: string;
	name: string;
	placement: MenuPlacement;
	items: MenuItem[];
}

/* ------------------------------------------------------------------ *
 * Academy Platform — Section 36
 *   GET /academy/tracks, GET /academy/tracks/{slug}, GET /academy/lessons/{id},
 *   POST /academy/lessons/{id}/progress, POST /academy/lessons/{id}/complete,
 *   GET /me/enrollments, GET /me/certificates, GET/POST /academy/quizzes/{id}
 * ------------------------------------------------------------------ */

/** Difficulty level is a catalog value returned by the backend. */
export type TrackLevel = string;

/** Option list item (value/label) — returned by catalog endpoints. */
export interface CatalogOption {
	value: string;
	label: string;
}

/** Track marketplace category — GET /academy/categories. */
export interface TrackCategory {
	id: string;
	slug: string;
	name: string;
	sort_order?: number;
}

/** Catalog option lists — GET /academy/catalog/options. */
export interface CatalogOptionsResponse {
	difficulty_levels?: CatalogOption[];
	social_platforms?: CatalogOption[];
	/** Catalog sort modes — values the `sort` query param accepts. */
	sort_options?: CatalogOption[];
	/** Page-size choices for the catalog/search per-page control. */
	page_sizes?: CatalogOption[];
	/** Distinct languages present in the live catalog. */
	languages?: string[];
}

/** Social / share platform — GET /share-platforms/enabled. */
export interface SharePlatform {
	key: string;
	label: string;
	icon: string;
	url_template: string;
	color: string;
}

/** Catalog list item — GET /academy/tracks. */
export interface TrackSummary {
	id: string;
	slug: string;
	title: string;
	subtitle?: string | null;
	thumbnail_url?: string | null;
	level: TrackLevel;
	/** ISO 8601 duration estimate, e.g. "PT4H30M". */
	duration_iso?: string | null;
	/** Human-readable duration, e.g. "4h 30m". */
	duration_label?: string | null;
	instructor_name?: string | null;
	instructor_id?: string | null;
	rating_avg?: number | null;
	rating_count?: number;
	enrolled_count?: number;
	/** Marketplace price in kobo (ADR-063). Null/0 = free. */
	price_kobo?: number | null;
	/** Currency code for the price. Inherits from the track's geo space. */
	currency?: string | null;
	/** Track category slug (from /academy/categories). */
	category?: string | null;
	/** ISO 639-1 language code for the track content. */
	language?: string | null;
	/** Primary geo space (country) this track is targeted at. */
	geo_space_id?: string | null;
	/** True when the course is published to the Japa Academy marketplace. */
	is_published_to_academy?: boolean;
	/** True when the course can be accessed via subscription plans. */
	subscription_eligible?: boolean;
	tags?: string[];
}

/** Full track detail — GET /academy/tracks/{slug}. */
export interface TrackDetail extends TrackSummary {
	description?: string | null;
	/** TipTap JSON for the long-form overview (rendered to .prose server-side). */
	overview_json?: unknown;
	overview_html?: string | null;
	objectives?: string[];
	prerequisites?: string[];
	instructor?: InstructorRef | null;
	modules: ModuleSummary[];
	/** True if the requesting session can preview without enrollment. */
	is_preview?: boolean;
}

export interface InstructorRef {
	id: string;
	username: string;
	displayName: string;
	avatar_url?: string | null;
	bio?: string | null;
}

export interface ModuleSummary {
	id: string;
	slug: string;
	title: string;
	summary?: string | null;
	sort_order: number;
	lessons: LessonSummary[];
}

export interface LessonSummary {
	id: string;
	slug: string;
	title: string;
	lesson_type: LessonType;
	/** ISO 8601 duration estimate. */
	duration_iso?: string | null;
	duration_label?: string | null;
	/** True if the lesson can be previewed without enrollment. */
	is_preview?: boolean;
}

export type LessonType = 'article' | 'video' | 'quiz' | 'checklist';

/** Full lesson — GET /academy/lessons/{id} (authenticated). */
export interface Lesson {
	id: string;
	slug: string;
	title: string;
	lesson_type: LessonType;
	module_id: string;
	module_title?: string;
	track_id: string;
	track_slug: string;
	track_title: string;
	/** Article: TipTap JSON rendered to .prose server-side. */
	content_json?: unknown;
	content_html?: string | null;
	/** Video: signed URL or HLS manifest (browser-guarded init). */
	video_url?: string | null;
	video_duration_secs?: number | null;
	/** Quiz: question set. */
	quiz?: Quiz | null;
	/** Checklist: TaskList items. */
	checklist?: ChecklistItem[] | null;
	/** Last saved playback position in seconds (video lessons). */
	progress_secs?: number | null;
	/** True when the lesson is marked complete for the requesting user. */
	is_completed?: boolean;
	is_preview?: boolean;
	/** Slugs for building the curriculum rail + next/prev navigation. */
	prev_lesson?: LessonNavRef | null;
	next_lesson?: LessonNavRef | null;
}

export interface LessonNavRef {
	slug: string;
	title: string;
	module_slug: string;
}

/* ------------------------------------------------------------------ *
 * Quizzes — GET/POST /academy/quizzes/{id}
 * ------------------------------------------------------------------ */

export interface Quiz {
	id: string;
	title: string;
	description?: string | null;
	pass_score_pct?: number;
	questions: QuizQuestion[];
}

export type QuizQuestionType = 'single' | 'multiple' | 'text';

export interface QuizQuestion {
	id: string;
	type: QuizQuestionType;
	prompt: string;
	/** Markdown-formatted explanation shown after submit. */
	explanation?: string | null;
	options?: QuizOption[];
	/** Accepted free-text answers (for text questions). */
	accepted_answers?: string[];
	points?: number;
}

export interface QuizOption {
	id: string;
	label: string;
	is_correct?: boolean;
}

export interface QuizSubmission {
	question_id: string;
	/** Option IDs (single/multiple) or free text. */
	answer: string[] | string;
}

export interface QuizResult {
	score_pct: number;
	passed: boolean;
	earned_points: number;
	total_points: number;
	/** Per-question feedback. */
	results: Array<{
		question_id: string;
		correct: boolean;
		selected: string[];
		correct_answers: string[];
		explanation?: string | null;
	}>;
}

/* ------------------------------------------------------------------ *
 * Checklists — TaskList lesson type
 * ------------------------------------------------------------------ */

export interface ChecklistItem {
	id: string;
	label: string;
	checked?: boolean;
}

/* ------------------------------------------------------------------ *
 * Enrollment & progress — GET /me/enrollments, POST /academy/lessons/{id}/progress
 * ------------------------------------------------------------------ */

export interface Enrollment {
	id: string;
	user_id: string;
	track_id: string;
	track_slug: string;
	track_title: string;
	thumbnail_url?: string | null;
	level: TrackLevel;
	access_type: string;
	/** 0–100 completion percentage. */
	progress_pct: number;
	/** Count of completed lessons. Omitted by the backend when zero. */
	lessons_completed?: number;
	/** Total number of lessons in the track. Omitted by the backend when zero. */
	lessons_total?: number;
	/** ISO 8601 timestamps. */
	enrolled_at: string;
	completed_at?: string | null;
	last_activity_at?: string | null;
	/** Next up lesson for the continue-learning card. */
	next_lesson?: LessonNavRef | null;
}

export interface AcademyLessonProgressUpdate {
	progress_secs?: number;
	/** Optional — set true to mark complete via progress endpoint. */
	complete?: boolean;
}

/* ------------------------------------------------------------------ *
 * Certificates — GET /me/certificates, GET /certificates/{id}
 * ------------------------------------------------------------------ */

export interface Certificate {
	id: string;
	track_id: string;
	/** Track title — backend serializes this as `title` (track_title was a drifted name). */
	title: string;
	track_slug: string;
	/** Recipient display name (denormalized for public share page). */
	recipient_name: string;
	recipient_username: string;
	issued_at: string;
	/** Public verification URL. */
	verify_url?: string | null;
	certificate_number?: string;
}

/* ------------------------------------------------------------------ *
 * Reviews — GET/POST /academy/tracks/{id}/reviews (polymorphic reviews table)
 * ------------------------------------------------------------------ */

export interface TrackReview {
	id: string;
	track_id: string;
	author_id: string;
	author_username: string;
	author_displayName?: string;
	author_avatar_url?: string | null;
	rating: number;
	title?: string | null;
	body?: string | null;
	created_at: string;
	updated_at?: string | null;
}

export interface ReviewAggregate {
	avg: number;
	count: number;
	distribution?: Record<number, number>;
}

/* ------------------------------------------------------------------ *
 * Japa Academy (ADR-063) — creator economy
 *   GET /academy/catalog, GET /academy/instructor/me,
 *   POST /academy/instructor/apply,
 *   GET /academy/my/tracks, POST /academy/my/tracks/{id}/submit,
 *   GET /academy/my/earnings, GET /academy/my/payouts
 * ------------------------------------------------------------------ */

export type InstructorStatus = 'pending' | 'approved' | 'rejected' | 'suspended';

export interface AcademyInstructorProfile {
	id: string;
	user_id: string;
	display_name: string;
	headline?: string | null;
	bio?: string | null;
	social_links?: Record<string, string>;
	tier: string;
	status: InstructorStatus;
	commission_pct_override?: number | null;
	kyc_status: string;
	application_note?: string | null;
	reviewed_at?: string | null;
	created_at: string;
	updated_at: string;
	/** Aggregate stats for the instructor dashboard. */
	total_enrolled?: number;
	total_revenue_kobo?: number;
	/** Currency for revenue totals — resolved from wallet.default_currency. */
	currency?: string | null;
	courses_count?: number;
}

/** Marketplace catalog item — GET /academy/catalog. */
export interface AcademyCatalogTrack extends TrackSummary {
	instructor_bio?: string | null;
	instructor_avatar_url?: string | null;
}

/** Instructor's own track with marketplace status — GET /academy/my/tracks. */
export interface InstructorTrack extends TrackSummary {
	description?: string | null;
	/** True when the track is live (not draft). Distinct from academy submission status. */
	is_published?: boolean;
	/** Marketplace submission status. */
	submission_status: 'draft' | 'pending' | 'approved' | 'rejected';
	submitted_at?: string | null;
	reviewed_at?: string | null;
	rejection_reason?: string | null;
	total_enrolled: number;
	total_revenue_kobo: number;
	thumbnail_url?: string | null;
}

export interface InstructorTrackStats {
	track_id: string;
	total_enrolled: number;
	completion_rate_pct: number;
	total_revenue_kobo: number;
	/** Currency for the revenue series and total. */
	currency?: string | null;
	/** Daily revenue series for LayerChart. */
	revenue_series?: Array<{ date: string; kobo: number }>;
	enrollment_series?: Array<{ date: string; count: number }>;
}

export interface EarningsSummary {
	total_earned_kobo: number;
	total_paid_out_kobo: number;
	pending_kobo: number;
	/** Currency for all amounts in this summary. */
	currency?: string | null;
	/** Daily revenue series for LayerChart. */
	revenue_series: Array<{ date: string; kobo: number }>;
}

export interface Payout {
	id: string;
	amount_kobo: number;
	/** ISO 4217 currency code for the payout. */
	currency?: string | null;
	status: 'pending' | 'processing' | 'paid' | 'failed';
	period_start: string;
	period_end: string;
	requested_at: string;
	processed_at?: string | null;
	/** Track breakdown. */
	tracks?: Array<{ track_id: string; track_title: string; amount_kobo: number }>;
}

/* ------------------------------------------------------------------ *
 * Learner analytics — My Learning dashboard (ADR-059 LayerChart)
 * ------------------------------------------------------------------ */

export interface LearnerAnalytics {
	/** Progress over time series for LineChart. */
	progress_series: Array<{ date: string; pct: number }>;
	/** Weekly activity for BarChart. */
	weekly_activity: Array<{ date: string; lessons: number; minutes: number }>;
	/** Aggregate stats. */
	total_lessons_completed: number;
	total_time_spent_secs: number;
	current_streak_days: number;
	longest_streak_days: number;
	/** Streak sparkline (last 14 days). */
	streak_series: Array<{ date: string; active: boolean }>;
}

/* ------------------------------------------------------------------ *
 * Bookmarks — GET /me/bookmarks, GET /me/bookmark-folders
 * Ref: sections/50-user-dashboard-settings.md
 * ------------------------------------------------------------------ */

/** GET /me/bookmarks — a thin bookmark record with optional folder info. */
export interface Bookmark {
	id: string;
	entity_type: string;
	entity_id: string;
	folder?: string | null;
	folder_id?: string | null;
	created_at: string;
	title?: string | null;
	slug?: string | null;
}

/** GET /me/bookmark-folders — a user's bookmark folder. */
export interface BookmarkFolder {
	id: string;
	name: string;
	position?: number;
	bookmark_count?: number;
	created_at: string;
	updated_at?: string;
}

/* ------------------------------------------------------------------ *
 * Discount codes — POST /payments/discount-codes/validate
 * Ref: internal/payments/handlers.go (ValidateDiscountCode)
 * The backend returns the full discount_codes row; the frontend
 * computes the kobo discount from discount_type + discount_value.
 * ------------------------------------------------------------------ */

export interface DiscountCode {
	id: string;
	code: string;
	discount_type: 'percent' | 'percentage' | 'fixed';
	discount_value: number | string;
	applicable_to: string;
}

/* ------------------------------------------------------------------ *
 * Payment intent — POST /payments/intent
 * Ref: internal/payments/dto.go (PaymentAPIResponse)
 * ------------------------------------------------------------------ */

export interface PaymentIntent {
	id: string;
	status: string;
	checkout_url?: string;
	amount: number;
	currency: string;
	discount_amount?: number;
	discount_code_id?: string;
}
