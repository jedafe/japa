// safeHttpUrl returns the trimmed URL when it is an absolute http(s) URL,
// otherwise ''. Use at href/window.open sinks for user- or source-provided
// URLs (job company_website, application_url, source_url) so javascript:,
// data:, or scheme-relative values can never reach the DOM — even for rows
// written before the backend validation (internal/jobs sanitizeJobFieldURL).
export function safeHttpUrl(url: string | null | undefined): string {
	const v = (url ?? '').trim();
	if (!v) return '';
	try {
		const u = new URL(v);
		return u.protocol === 'http:' || u.protocol === 'https:' ? v : '';
	} catch {
		return '';
	}
}
