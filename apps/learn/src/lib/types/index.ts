/**
 * Shared domain + transport types for the Learn App.
 * Shapes mirror the Go backend's JSON contract (verified against the live API
 * at internal/learning/handlers.go and internal/university/handlers.go).
 *
 * Ref: ADR-060 (Learn shell), ADR-063 (Japa University creator economy),
 *      sections/36 (Learning Platform).
 */

export type ThemeName = 'light' | 'dark';

/** Supported UI locales. Mirrors the i18n store's Locale type. */
export type Locale = 'en' | 'fr';

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
 * Notifications (realtime badge — minimal shape for the Learn shell)
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
 * Learning Platform — Section 36
 *   GET /learn/tracks, GET /learn/tracks/{slug}, GET /learn/lessons/{id},
 *   POST /learn/lessons/{id}/progress, POST /learn/lessons/{id}/complete,
 *   GET /me/enrollments, GET /me/certificates, GET/POST /learn/quizzes/{id}
 * ------------------------------------------------------------------ */

export type TrackLevel = 'beginner' | 'intermediate' | 'advanced';

/** Catalog list item — GET /learn/tracks. */
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
	/** Currency code for the price (e.g. 'NGN'). Defaults to NGN if absent. */
	currency?: string | null;
	/** True when the course is published to the Japa University marketplace. */
	is_published_to_university?: boolean;
	/** True when the course can be accessed via subscription plans. */
	subscription_eligible?: boolean;
	tags?: string[];
}

/** Full track detail — GET /learn/tracks/{slug}. */
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

/** Full lesson — GET /learn/lessons/{id} (authenticated). */
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
 * Quizzes — GET/POST /learn/quizzes/{id}
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
 * Enrollment & progress — GET /me/enrollments, POST /learn/lessons/{id}/progress
 * ------------------------------------------------------------------ */

export interface Enrollment {
	id: string;
	track_id: string;
	track_slug: string;
	track_title: string;
	thumbnail_url?: string | null;
	level: TrackLevel;
	/** 0–100 completion percentage. */
	progress_pct: number;
	/** Count of completed lessons. */
	lessons_completed: number;
	lessons_total: number;
	/** ISO 8601 timestamps. */
	enrolled_at: string;
	last_activity_at?: string | null;
	/** Next up lesson for the continue-learning card. */
	next_lesson?: LessonNavRef | null;
}

export interface LessonProgressUpdate {
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
	track_title: string;
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
 * Reviews — GET/POST /learn/tracks/{id}/reviews (polymorphic reviews table)
 * ------------------------------------------------------------------ */

export interface TrackReview {
	id: string;
	track_id: string;
	author_id: string;
	author_username: string;
	author_displayName?: string;
	author_avatar_url?: string | null;
	rating: number;
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
 * Japa University (ADR-063) — creator economy
 *   GET /learn/university/catalog, GET /learn/university/instructor/me,
 *   POST /learn/university/instructor/apply,
 *   GET /learn/university/my/tracks, POST /learn/university/my/tracks/{id}/submit,
 *   GET /learn/university/my/earnings, GET /learn/university/my/payouts
 * ------------------------------------------------------------------ */

export type InstructorStatus = 'pending' | 'approved' | 'rejected' | 'suspended';

export interface InstructorProfile {
	id: string;
	tenant_id: string;
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
	courses_count?: number;
}

/** Marketplace catalog item — GET /learn/university/catalog. */
export interface UniversityCatalogTrack extends TrackSummary {
	instructor_bio?: string | null;
	instructor_avatar_url?: string | null;
}

/** Instructor's own track with marketplace status — GET /learn/university/my/tracks. */
export interface InstructorTrack extends TrackSummary {
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
	/** Daily revenue series for LayerChart. */
	revenue_series?: Array<{ date: string; kobo: number }>;
	enrollment_series?: Array<{ date: string; count: number }>;
}

export interface EarningsSummary {
	total_earned_kobo: number;
	total_paid_out_kobo: number;
	pending_kobo: number;
	/** Daily revenue series for LayerChart. */
	revenue_series: Array<{ date: string; kobo: number }>;
}

export interface Payout {
	id: string;
	amount_kobo: number;
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
	bookmark_count?: number;
	created_at: string;
	updated_at?: string;
}
