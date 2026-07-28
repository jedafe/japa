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

/**
 * Composer-specific metadata for each content type. The `label` field is a
 * fallback only — the actual label displayed to the user comes from the
 * backend's GET /content-types response (ContentTypeMeta.label), so the
 * selector always reflects admin-configured names (Rule 16 — no hardcoded
 * data in the frontend).
 */
export const COMPOSER_TYPES: Record<string, Omit<TypeEntry, 'label'>> = {
	blog_post: {
		value: 'blog_post',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: 'feature.blog'
	},
	article: {
		value: 'article',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	guide: {
		value: 'guide',
		formKind: 'standard',
		supportsSeries: true,
		customFieldEntityType: 'content',
		featureFlag: null
	},
	comparison: {
		value: 'comparison',
		formKind: 'comparison',
		supportsSeries: false,
		customFieldEntityType: 'content',
		featureFlag: 'feature.comparison'
	}
};

/** The order the selector presents composer types. */
const COMPOSER_ORDER = ['blog_post', 'article', 'guide', 'comparison'];

export function resolveType(value: string, backendLabel?: string): TypeEntry {
	const meta = COMPOSER_TYPES[value] ?? COMPOSER_TYPES.blog_post;
	return { ...meta, label: backendLabel ?? meta.value };
}

/**
 * Intersect the backend's content types with composer-supported types and the
 * tenant's feature flags. A gated type is shown only when its flag is truthy;
 * an ungated type (featureFlag === null) is always shown. The label comes
 * from the backend's ContentTypeMeta, not a hardcoded string (Rule 16).
 */
export function availableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>
): TypeEntry[] {
	const offered = new Map(types.map((t) => [t.value, t.label]));
	return COMPOSER_ORDER.filter((v) => offered.has(v))
		.map((v) => ({ ...COMPOSER_TYPES[v], label: offered.get(v) ?? v }))
		.filter((entry) => entry.featureFlag === null || flags[entry.featureFlag] === true);
}
