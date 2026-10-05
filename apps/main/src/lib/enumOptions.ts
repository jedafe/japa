// Ref: japaconnect-engineering/sections/02-main-site-frontend.md (Rule 16)
// Shared option builders and label helpers for schema-defined string-union enums.
// These centralize the enum vocabulary used by <select> filters and form fields
// so the same value set is not duplicated across multiple files. Labels are
// resolved through the translator so i18n fallbacks remain in one place.
//
// NOTE: Where the backend exposes a dedicated schema endpoint (e.g.
// GET /calendar/categories), the page load should prefer that route and these
// helpers are used only as a client-side fallback for ad-hoc display.

import type {
	ScamType,
	ScamPlatform,
	MutualAidPostType,
	MutualAidCategory,
	ServiceCategory,
	JapaStage,
	JapaStageOption,
	JapaIntent,
	CalendarCategory,
	TrendingWindow,
	RoadmapStatus
} from '$lib/types';

type Translator = (key: string, fallback?: string) => string;

// --- Scam alerts ---

export function scamTypeLabel(t: Translator, value: ScamType): string {
	return t(`scam_alerts.type.${value}`, value.replace(/_/g, ' '));
}

export function scamPlatformLabel(t: Translator, value: ScamPlatform): string {
	return t(`scam_alerts.platform.${value}`, value.replace(/_/g, ' '));
}

export function scamTypeOptions(
	t: Translator,
	types: ScamType[] = []
): { value: ScamType; label: string }[] {
	return types.map((value) => ({ value, label: scamTypeLabel(t, value) }));
}

export function scamTypeFilterOptions(
	t: Translator,
	types: ScamType[] = []
): { value: '' | ScamType; label: string }[] {
	return [{ value: '', label: t('scam_alerts.filter_all_types', 'All types') }, ...scamTypeOptions(t, types)];
}

export function scamPlatformOptions(
	t: Translator,
	platforms: ScamPlatform[] = []
): { value: ScamPlatform; label: string }[] {
	return platforms.map((value) => ({ value, label: scamPlatformLabel(t, value) }));
}

// --- Mutual aid ---

export function mutualAidPostTypeLabel(t: Translator, value: MutualAidPostType): string {
	return t(`mutual_aid.type_${value}`, value.replace(/_/g, ' '));
}

export function mutualAidCategoryLabel(t: Translator, value: MutualAidCategory): string {
	return t(`mutual_aid.cat_${value}`, value.replace(/_/g, ' '));
}

export function mutualAidPostTypeOptions(
	t: Translator,
	includeAll = false,
	postTypes: MutualAidPostType[] = []
): { value: '' | MutualAidPostType; label: string }[] {
	const all: { value: '' | MutualAidPostType; label: string }[] = includeAll
		? [{ value: '', label: t('common.all_types', 'All types') }]
		: [];
	return [...all, ...postTypes.map((value) => ({ value, label: mutualAidPostTypeLabel(t, value) }))];
}

export function mutualAidCategoryOptions(
	t: Translator,
	includeAll = false,
	categories: MutualAidCategory[] = []
): { value: '' | MutualAidCategory; label: string }[] {
	const all: { value: '' | MutualAidCategory; label: string }[] = includeAll
		? [{ value: '', label: t('common.all_categories', 'All categories') }]
		: [];
	return [...all, ...categories.map((value) => ({ value, label: mutualAidCategoryLabel(t, value) }))];
}

// --- Service directory ---

export function serviceCategoryLabel(t: Translator, value: ServiceCategory): string {
	return t(`services.cat_${value}`, value.replace(/_/g, ' '));
}

export function serviceCategoryOptions(
	t: Translator,
	categories: ServiceCategory[] = []
): { value: ServiceCategory; label: string }[] {
	return categories.map((value) => ({ value, label: serviceCategoryLabel(t, value) }));
}

// --- Japa stage ---

export const japaStageColors: Record<JapaStage, string> = {
	considering: 'var(--color-muted)',
	planning: 'var(--stage-planning)',
	preparing: 'var(--stage-preparing)',
	applied: 'var(--stage-applied)',
	approved: 'var(--stage-approved)',
	relocated: 'var(--stage-relocated)',
	settled: 'var(--color-primary)',
	// ADR-122: trip-class terminals mirror the relocation terminal palette.
	traveled: 'var(--stage-relocated)',
	returned: 'var(--color-primary)'
};

export function japaStageLabel(t: Translator, value: JapaStage): string {
	return t(`japa_stage.${value}`, value.charAt(0).toUpperCase() + value.slice(1));
}

export function japaStageOptions(
	t: Translator,
	stages: JapaStageOption[] = []
): { value: JapaStage; label: string }[] {
	return stages.map((s) => ({ value: s.value, label: japaStageLabel(t, s.value) }));
}

/** Stages applicable to a journey class (ADR-122 class-scoped terminals). */
export function japaStagesForIntent(
	stages: JapaStageOption[],
	intent: JapaIntent | ''
): JapaStageOption[] {
	if (!intent) return stages;
	return stages.filter((s) => !s.intents || s.intents.includes(intent));
}

// --- Japa intent (ADR-122) ---

export function japaIntentLabel(t: Translator, value: JapaIntent): string {
	return t(`japa_intent.${value}`, value.charAt(0).toUpperCase() + value.slice(1));
}

export function japaIntentOptions(
	t: Translator,
	intents: JapaIntent[] = []
): { value: JapaIntent; label: string }[] {
	return intents.map((value) => ({ value, label: japaIntentLabel(t, value) }));
}

// --- Calendar ---

export function calendarCategoryLabel(t: Translator, value: CalendarCategory): string {
	return t(`calendar.cat_${value}`, value.replace(/_/g, ' '));
}

// --- Trending ---

export const trendingWindows: TrendingWindow[] = ['24h', '7d', '30d'];

export function trendingWindowOptions(
	t: Translator
): { value: TrendingWindow; label: string }[] {
	return trendingWindows.map((value) => ({
		value,
		label: t(`trending.window_${value}`, value)
	}));
}

export function trendingTabs(t: Translator): { id: 'content' | 'tags' | 'spaces' | 'users'; label: string }[] {
	return [
		{ id: 'content', label: t('trending.tab_content', 'Content') },
		{ id: 'tags', label: t('trending.tab_tags', 'Tags') },
		{ id: 'spaces', label: t('trending.tab_spaces', 'Spaces') },
		{ id: 'users', label: t('trending.tab_users', 'People') }
	];
}

// --- Public product roadmap (§113) ---

/** Statuses that accept votes — voting is rejected for the rest (§113). */
export const roadmapVotableStatuses: RoadmapStatus[] = [
	'under_review',
	'planned',
	'in_progress'
];

/** Statuses rendered on the public board / reachable via /roadmap/[slug]. */
export const roadmapPublicStatuses: RoadmapStatus[] = [
	'under_review',
	'planned',
	'in_progress',
	'shipped'
];

export function roadmapStatusLabel(t: Translator, value: RoadmapStatus | string): string {
	return t(`roadmap.status.${value}`, value.replace(/_/g, ' '));
}

/**
 * Allowed lifecycle transitions, mirroring roadmap.IsValidTransition
 * (§113 — Status Lifecycle). The backend re-validates on PATCH; this map
 * only constrains which options the moderator <select> offers so illegal
 * jumps are never presented in the UI.
 */
const roadmapTransitions: Record<RoadmapStatus, RoadmapStatus[]> = {
	pending: ['under_review', 'declined'],
	under_review: ['planned', 'declined', 'archived'],
	planned: ['in_progress', 'under_review', 'declined', 'archived'],
	in_progress: ['shipped', 'planned', 'archived'],
	shipped: ['archived'],
	declined: ['under_review', 'archived'],
	archived: []
};

/** Options for the moderator status select: current status + valid targets. */
export function roadmapStatusOptions(
	t: Translator,
	current: RoadmapStatus
): { value: RoadmapStatus; label: string }[] {
	return [current, ...(roadmapTransitions[current] ?? [])].map((value) => ({
		value,
		label: roadmapStatusLabel(t, value)
	}));
}

/**
 * Maps a roadmap API error code (§113 — the codes emitted by
 * internal/roadmap handlers plus the shared errpkg/service sentinels) to a
 * `roadmap.errors.*` i18n key. Unknown/unmapped codes fall back to the
 * caller-provided generic key.
 */
export function roadmapErrorKey(
	code: string | undefined,
	fallback = 'roadmap.errors.submit_failed'
): string {
	switch (code) {
		case 'missing_title':
			return 'roadmap.errors.title_required';
		case 'invalid_category_id':
			return 'roadmap.errors.invalid_category_id';
		case 'already_voted':
			return 'roadmap.errors.already_voted';
		case 'not_voted':
			return 'roadmap.errors.not_voted';
		case 'invalid_status_transition':
			return 'roadmap.errors.invalid_status_transition';
		case 'feature_disabled':
			return 'roadmap.errors.feature_disabled';
		case 'rate_limit_exceeded':
			return 'roadmap.errors.rate_limit_exceeded';
		case 'unauthorized':
			return 'roadmap.errors.unauthorized';
		case 'not_found':
			return 'roadmap.errors.not_found';
		default:
			return fallback;
	}
}
