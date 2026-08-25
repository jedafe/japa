/**
 * Frontmatter validation for JapaConnect publishing.
 * Ref: sections/105-japa-content-studio-desktop.md
 */

export interface ValidationErrors {
	[key: string]: string;
}

const HIGH_STAKES_CATEGORIES = new Set([
	'visa',
	'remittance',
	'banking',
	'grants',
	'health_insurance',
	'admission'
]);

export function validateFrontmatter(
	frontmatter: Record<string, unknown>,
	effectiveStatus?: string
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

	if ((status === 'published' || status === 'scheduled') && !getString(frontmatter, 'space_id')) {
		errors.space_id = 'A primary space is required to publish or schedule content.';
	}

	const guideCategory = getString(frontmatter, 'guide_category');
	if (
		guideCategory &&
		HIGH_STAKES_CATEGORIES.has(guideCategory.toLowerCase()) &&
		status === 'published' &&
		!frontmatter.force
	) {
		errors.guide_category = `High-stakes category "${guideCategory}" will be held for review before publishing.`;
	}

	return errors;
}

export function validateBody(body: string, status: string): string | undefined {
	const trimmed = body.trim();
	if (!trimmed) return 'Body is required.';
	if (status !== 'draft' && status !== 'archived' && trimmed.length < 10) {
		return 'Body is too short to publish (minimum 10 characters). Most sites require at least 100-200 words for non-draft content.';
	}
	return undefined;
}

function getString(frontmatter: Record<string, unknown>, key: string): string {
	const value = frontmatter[key];
	return typeof value === 'string' ? value.trim() : '';
}
