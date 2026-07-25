import type { ContentTypeMeta } from '$lib/types';

export type FormKind = 'standard' | 'comparison';

export interface TypeEntry {
	value: string;
	label: string;
	formKind: FormKind;
	supportsSeries: boolean;
	customFieldEntityType: 'content';
	/** Flag that must be ON for this type to appear; null = always available. */
	featureFlag: string | null;
}

export const COMPOSER_TYPES: Record<string, TypeEntry> = {
	blog_post: {
		value: 'blog_post',
		label: 'Blog post',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: 'feature.blog'
	},
	article: {
		value: 'article',
		label: 'Article',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	guide: {
		value: 'guide',
		label: 'Guide',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	comparison: {
		value: 'comparison',
		label: 'Comparison',
		formKind: 'comparison',
		supportsSeries: false,
		customFieldEntityType: 'content',
		featureFlag: 'feature.comparison'
	}
};

/** The order the selector presents composer types. */
const COMPOSER_ORDER = ['blog_post', 'article', 'guide', 'comparison'];

export function resolveType(value: string): TypeEntry {
	return COMPOSER_TYPES[value] ?? COMPOSER_TYPES.blog_post;
}

/**
 * Intersect the backend's content types with composer-supported types and the
 * tenant's feature flags. A gated type is shown only when its flag is truthy;
 * an ungated type (featureFlag === null) is always shown.
 */
export function availableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>
): TypeEntry[] {
	const offered = new Set(types.map((t) => t.value));
	return COMPOSER_ORDER.filter((v) => offered.has(v))
		.map((v) => COMPOSER_TYPES[v])
		.filter((entry) => entry.featureFlag === null || flags[entry.featureFlag] === true);
}
