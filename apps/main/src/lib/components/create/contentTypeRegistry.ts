import type { ContentTypeMeta, GuideCategory } from '$lib/types';

export type FormKind = 'standard' | 'comparison';

export interface TypeEntry {
	value: string;
	label: string;
	formKind: FormKind;
	supportsSeries: boolean;
	customFieldEntityType: 'content';
	/** Flag that must be ON for this type to appear; null = always available. */
	featureFlag: string | null;
	/** True when this type requires a sub-category selection (e.g. guide). */
	hasCategory?: boolean;
	/** Admin-configured sub-categories (loaded from GET /content-types). */
	categories?: GuideCategory[];
}

/**
 * Composer-specific metadata overrides for known Section 14 content types.
 * Labels, categories, and the canonical list come from GET /content-types
 * (Rule 16). This map only provides non-label metadata; unknown types fall
 * back to sensible defaults so the composer can create any content_type the
 * backend advertises.
 */
const COMPOSER_OVERRIDES: Record<string, Partial<Omit<TypeEntry, 'label'>>> = {
	blog_post: {
		formKind: 'standard',
		supportsSeries: true,
		featureFlag: 'feature.blog'
	},
	guide: {
		formKind: 'standard',
		supportsSeries: true,
		featureFlag: null,
		hasCategory: true
	},
	comparison: {
		formKind: 'comparison',
		supportsSeries: false,
		featureFlag: 'feature.comparison'
	},
	discussion: {
		formKind: 'standard',
		supportsSeries: false,
		featureFlag: 'feature.forum'
	},
	page: {
		formKind: 'standard',
		supportsSeries: false,
		featureFlag: 'feature.page'
	},
	// §79 Community Story Wall — stories use the standard composer; the
	// backend syncs story_meta from field_values + journey fields and sends
	// publishes to the moderation queue (in_review).
	story: {
		formKind: 'standard',
		supportsSeries: false,
		featureFlag: 'feature.story'
	}
};

const DEFAULT_COMPOSER_META: Omit<TypeEntry, 'label' | 'value'> = {
	formKind: 'standard',
	supportsSeries: false,
	customFieldEntityType: 'content',
	featureFlag: null,
	hasCategory: false
};

/**
 * Build a TypeEntry for a single content type, using the backend-supplied
 * label and categories and falling back to defaults for form behavior.
 */
export function resolveType(value: string, backendLabel?: string, categories?: GuideCategory[]): TypeEntry {
	const override = COMPOSER_OVERRIDES[value] ?? {};
	return {
		value,
		label: backendLabel ?? value,
		...DEFAULT_COMPOSER_META,
		...override,
		categories
	} as TypeEntry;
}

/**
 * Build the composer type selector from the backend's GET /content-types
 * response (Rule 16). The backend is the canonical catalog; the composer
 * metadata map only supplies behavior overrides (formKind, series support,
 * feature flags, category requirement). Unknown types use sensible defaults
 * so a new content_type does not require a frontend code change.
 */
export function availableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>
): TypeEntry[] {
	return types
		.map((t) => resolveType(t.value, t.label ?? t.value, t.categories))
		.filter((entry) => entry.featureFlag === null || flags[entry.featureFlag] !== false);
}

/** Content types that can be created through the /create page composer. */
export const CREATE_ALLOWED_TYPES = ['blog_post', 'comparison', 'guide', 'story'] as const;

const CREATE_TYPE_ORDER = Object.fromEntries(
	CREATE_ALLOWED_TYPES.map((value, index) => [value, index])
) as Record<string, number>;

/**
 * Build the /create type selector. Blog posts, comparisons, guides, and
 * stories (§79 — standard composer with the story_fields group; the
 * backend syncs story_meta and enforces moderation) are authored here;
 * pages are created in the admin pagebuilder and discussions are created
 * in the forum composer (ADR-058 / Section 14).
 */
export function createAvailableTypes(
	types: ContentTypeMeta[],
	flags: Record<string, boolean>
): TypeEntry[] {
	const allowed = new Set<string>(CREATE_ALLOWED_TYPES);
	return availableTypes(types, flags)
		.filter((entry) => allowed.has(entry.value))
		.sort((a, b) => (CREATE_TYPE_ORDER[a.value] ?? 0) - (CREATE_TYPE_ORDER[b.value] ?? 0));
}
