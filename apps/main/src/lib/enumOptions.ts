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
	JapaIntent,
	CalendarCategory,
	TrendingWindow
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
	settled: 'var(--color-primary)'
};

export function japaStageLabel(t: Translator, value: JapaStage): string {
	return t(`japa_stage.${value}`, value.charAt(0).toUpperCase() + value.slice(1));
}

export function japaStageOptions(
	t: Translator,
	stages: JapaStage[] = []
): { value: JapaStage; label: string }[] {
	return stages.map((value) => ({ value, label: japaStageLabel(t, value) }));
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
