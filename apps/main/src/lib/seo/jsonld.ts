/**
 * Merge a backend-resolved JSON-LD payload with a component's fallback
 * entity schema (Section 14 / Section 26).
 *
 * The backend merges §45 editor FAQ blocks into the resolved template as a
 * @graph sibling server-side. When NO template resolves for the content
 * type (page, event, custom CPTs) it emits a standalone FAQPage instead —
 * returning that alone would drop the generic Article/BlogPosting fallback
 * the page would otherwise emit. In that single case, keep both under a
 * shared @graph. Every other resolved payload is returned untouched.
 */
export function mergeJsonLd(resolved: unknown, fallback: unknown): unknown {
	if (!resolved) return fallback;
	if (fallback === null || fallback === undefined) return resolved;

	const type =
		typeof resolved === 'object' && !Array.isArray(resolved)
			? (resolved as Record<string, unknown>)['@type']
			: undefined;

	if (type === 'FAQPage') {
		return { '@context': 'https://schema.org', '@graph': [fallback, resolved] };
	}

	return resolved;
}
