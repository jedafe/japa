// Client-side SEO score computation — mirrors the backend ValidateSEO
// handler (internal/seo/sitemap.go) per Section 26 ResolveMetadata.
// Used by the /create page SEO tab to show a live score as the user edits.

export interface SeoScoreInput {
	title: string;
	slug: string;
	metaTitle: string | null;
	metaDescription: string | null;
	ogImageUrl: string | null;
	canonicalUrl: string;
	seoIndexable: boolean;
	visibility: string;
	bodyHtml: string;
	contentType: string;
}

export interface SeoScoreResult {
	score: number;
	issues: string[];
}

// Minimum length thresholds for SEO fields. A field that is present but
// shorter than its minimum counts as a "short_*" issue (distinct from
// "missing_*"). Values follow common SEO best practices and match the
// backend ValidateSEO handler.
export const SEO_MIN_LENGTHS: Record<string, number> = {
	title: 10,
	slug: 3,
	meta_title: 30,
	meta_description: 70,
	og_title: 30,
	og_description: 70
};

// Maximum length thresholds for SEO fields (for display + over-limit issues).
export const SEO_MAX_LENGTHS: Record<string, number> = {
	meta_title: 60,
	meta_description: 160,
	og_title: 60,
	og_description: 160
};

// excerpt strips HTML tags and returns the first n characters of body text,
// matching the backend's excerpt() fallback for meta_description.
function excerpt(body: string, n: number): string {
	if (!body) return '';
	// Strip HTML tags (body_html is the rendered HTML from the editor).
	const text = body
		.replace(/<[^>]*>/g, ' ')
		.replace(/&[a-z]+;/gi, ' ')
		.replace(/\s+/g, ' ')
		.trim();
	return text.slice(0, n);
}

// computeSeoScore evaluates the current form state and returns a score
// (0-100) plus a list of issue codes, using the same resolution and
// scoring rules as the backend ValidateSEO handler.
export function computeSeoScore(input: SeoScoreInput): SeoScoreResult {
	const {
		title,
		slug,
		metaTitle,
		metaDescription,
		ogImageUrl,
		canonicalUrl,
		seoIndexable,
		visibility,
		bodyHtml,
		contentType
	} = input;

	// Resolve metadata per Section 26 ResolveMetadata (fallback chain).
	const resolvedMetaTitle = metaTitle?.trim() || title.trim();
	const resolvedMetaDesc = metaDescription?.trim() || excerpt(bodyHtml, 160);
	const resolvedOGTitle = title.trim(); // og_title falls back to title
	const resolvedOGDesc = resolvedMetaDesc; // og_description falls back to meta_description

	// robots_meta per Section 26.
	const isPrivate = visibility === 'private';
	const robotsMeta = isPrivate
		? 'noindex,nofollow'
		: !seoIndexable
			? 'noindex,follow'
			: 'index,follow';

	// json_ld_type fallback by content type (Section 26 schema mapping).
	const jsonLDType =
		contentType === 'blog_post'
			? 'BlogPosting'
			: contentType === 'discussion' || contentType === 'question'
				? 'QAPage'
				: 'Article';

	// Build issues list — each missing/empty resolved field is one issue,
	// and each present-but-too-short field is a separate issue.
	const issues: string[] = [];

	// --- Missing checks ---
	// Per Section 26 ResolveMetadata, canonical_url and og_image_url have
	// platform-level fallbacks (self-referencing URL and default banner
	// respectively), so an empty value is NOT a "missing" issue — the
	// platform auto-resolves them. Only fields without fallbacks are flagged.
	if (!title.trim()) issues.push('missing_title');
	if (!slug.trim()) issues.push('missing_slug');
	if (!resolvedMetaTitle) issues.push('missing_meta_title');
	if (!resolvedMetaDesc) issues.push('missing_meta_description');
	if (!resolvedOGTitle) issues.push('missing_og_title');
	if (!resolvedOGDesc) issues.push('missing_og_description');
	if (!jsonLDType) issues.push('missing_json_ld_type');
	if (robotsMeta.startsWith('noindex')) issues.push('not_indexable');

	// --- Minimum length checks (only for present, non-empty fields) ---
	if (title.trim() && title.trim().length < SEO_MIN_LENGTHS.title) {
		issues.push('short_title');
	}
	if (slug.trim() && slug.trim().length < SEO_MIN_LENGTHS.slug) {
		issues.push('short_slug');
	}
	if (resolvedMetaTitle && resolvedMetaTitle.length < SEO_MIN_LENGTHS.meta_title) {
		issues.push('short_meta_title');
	}
	if (resolvedMetaDesc && resolvedMetaDesc.length < SEO_MIN_LENGTHS.meta_description) {
		issues.push('short_meta_description');
	}
	if (resolvedOGTitle && resolvedOGTitle.length < SEO_MIN_LENGTHS.og_title) {
		issues.push('short_og_title');
	}
	if (resolvedOGDesc && resolvedOGDesc.length < SEO_MIN_LENGTHS.og_description) {
		issues.push('short_og_description');
	}

	// --- Maximum length checks (over-limit fields) ---
	if (resolvedMetaTitle && resolvedMetaTitle.length > SEO_MAX_LENGTHS.meta_title) {
		issues.push('long_meta_title');
	}
	if (resolvedMetaDesc && resolvedMetaDesc.length > SEO_MAX_LENGTHS.meta_description) {
		issues.push('long_meta_description');
	}

	// Score: 100 - 12 per issue, floored at 0.
	const score = Math.max(0, 100 - issues.length * 12);

	return { score, issues };
}
