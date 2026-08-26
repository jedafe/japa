/**
 * Frontmatter validation for JapaConnect publishing.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface ValidationErrors {
	[key: string]: string;
}

export const HIGH_STAKES_CATEGORIES = new Set([
	'visa',
	'remittance',
	'banking',
	'grants',
	'health_insurance',
	'admission'
]);

export function isHighStakesCategory(category: string): boolean {
	return !!category && HIGH_STAKES_CATEGORIES.has(category.toLowerCase());
}

export function validateFrontmatter(
	frontmatter: Record<string, unknown>,
	effectiveStatus?: string,
	mode?: 'operator' | 'member'
): ValidationErrors {
	const errors: ValidationErrors = {};

	const title = getString(frontmatter, 'title');
	if (!title) {
		errors.title = 'Title is required.';
	} else if (title.length < 3) {
		errors.title = 'Title must be at least 3 characters.';
	}

	const contentType = getString(frontmatter, 'content_type');
	if (!contentType) {
		errors.content_type = 'Content type is required.';
	}

	const slug = getString(frontmatter, 'slug');
	if (!slug) {
		errors.slug = 'Slug is required.';
	} else if (!/^[a-z0-9-]+$/.test(slug)) {
		errors.slug = 'Slug must be lowercase letters, numbers, and hyphens only.';
	}

	const status = effectiveStatus ?? getString(frontmatter, 'status');
	if (!status) {
		errors.status = 'Status is required.';
	} else if (!['draft', 'in_review', 'published', 'scheduled', 'archived'].includes(status)) {
		errors.status = 'Invalid status.';
	}

	const visibility = getString(frontmatter, 'visibility');
	if (!visibility) {
		errors.visibility = 'Visibility is required.';
	} else if (!['public', 'private', 'members', 'unlisted'].includes(visibility)) {
		errors.visibility = 'Invalid visibility.';
	}

	if (status !== 'draft' && status !== 'archived') {
		if (!getString(frontmatter, 'space_id')) {
			errors.space_id = 'A primary space is required for non-draft content.';
		}
		if (mode !== 'member' && !getString(frontmatter, 'persona_id')) {
			errors.persona_id = 'A persona is required for authorship attribution.';
		}
	}

	const guideCategory = getString(frontmatter, 'guide_category');
	if (guideCategory && isHighStakesCategory(guideCategory) && (status === 'published' || status === 'scheduled')) {
		errors.guide_category = `High-stakes category "${guideCategory}" must be held for review (status in_review).`;
	}

	if (status === 'scheduled') {
		const publishAt = getString(frontmatter, 'publish_at');
		if (!publishAt) {
			errors.publish_at = 'Scheduled content requires a publish date.';
		} else {
			const parsed = new Date(publishAt).getTime();
			if (Number.isNaN(parsed) || parsed <= Date.now()) {
				errors.publish_at = 'Publish at must be a valid future date and time.';
			}
		}
	}

	return errors;
}

export function validateBody(body: string, status: string): string | undefined {
	const trimmed = body.trim();
	if (status === 'draft' || status === 'archived') {
		return undefined;
	}
	if (!trimmed) return 'Body is required.';
	if (trimmed.length < 10) {
		return 'Body is too short to publish (minimum 10 characters). Most sites require at least 100-200 words for non-draft content.';
	}
	return undefined;
}

function getString(frontmatter: Record<string, unknown>, key: string): string {
	const value = frontmatter[key];
	return typeof value === 'string' ? value.trim() : '';
}

export interface HighStakesInput {
	guide_category?: string;
	tags?: unknown;
}

export function isHighStakes(frontmatter: Record<string, unknown> | HighStakesInput): boolean {
	const fm = frontmatter as Record<string, unknown>;
	const guideCategory = getString(fm, 'guide_category');
	if (isHighStakesCategory(guideCategory)) return true;
	const tags = fm.tags;
	const tagList: string[] = Array.isArray(tags)
		? tags.map((t) => String(t).toLowerCase())
		: typeof tags === 'string'
			? tags.split(',').map((t) => t.trim().toLowerCase())
			: [];
	return tagList.some((t) => t === 'immigration-advice' || t === 'financial-advice');
}
