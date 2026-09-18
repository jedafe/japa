/**
 * Frontmatter validation for JapaConnect publishing.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface ValidationErrors {
	[key: string]: string;
}

export const HIGH_STAKES_CATEGORIES = new Set([
	'visa',
	'banking',
	'grants',
	'health_insurance',
	'admission',
	'import_export'
]);

export function isHighStakesCategory(category: string): boolean {
	return !!category && HIGH_STAKES_CATEGORIES.has(category.toLowerCase());
}

export function validateFrontmatter(
	frontmatter: Record<string, unknown>,
	effectiveStatus?: string,
	mode?: 'operator' | 'member',
	allowedStatuses?: string[],
	allowedVisibilities?: string[]
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

	const defaultStatuses = ['draft', 'in_review', 'published', 'scheduled', 'archived'];
	const status = effectiveStatus ?? getString(frontmatter, 'status');
	const statusList = allowedStatuses?.length ? allowedStatuses : defaultStatuses;
	if (!status) {
		errors.status = 'Status is required.';
	} else if (!statusList.includes(status)) {
		errors.status = 'Invalid status.';
	}

	const defaultVisibilities = ['public', 'private', 'members'];
	const visibility = getString(frontmatter, 'visibility');
	const visibilityList = allowedVisibilities?.length ? allowedVisibilities : defaultVisibilities;
	if (!visibility) {
		errors.visibility = 'Visibility is required.';
	} else if (!visibilityList.includes(visibility)) {
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

export function validateBody(
	body: string,
	status: string,
	minLength = 10
): string | undefined {
	const trimmed = body.trim();
	if (status === 'draft' || status === 'archived') {
		return undefined;
	}
	if (!trimmed) return 'Body is required.';
	if (trimmed.length < minLength) {
		return `Body is too short to publish (minimum ${minLength} characters). Most sites require at least 100-200 words for non-draft content.`;
	}
	return undefined;
}

function getString(frontmatter: Record<string, unknown>, key: string): string {
	const value = frontmatter[key];
	return typeof value === 'string' ? value.trim() : '';
}

export interface HighStakesInput {
	guide_category?: string;
}

export function isHighStakes(frontmatter: Record<string, unknown> | HighStakesInput): boolean {
	const guideCategory = getString(frontmatter as Record<string, unknown>, 'guide_category');
	return isHighStakesCategory(guideCategory);
}
