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

// safeSameOriginHref returns href when it is a site-relative path ("/…",
// never protocol-relative "//" or backslash tricks) or an absolute URL on
// the current origin — otherwise ''. Use for return_url-style values that
// become a payment provider's success_url (M5): the backend enforces the
// same same-origin rule (servicedir validReturnURL), and the client omits
// anything that would fail it instead of sending a rejected value.
export function safeSameOriginHref(href: string | null | undefined): string {
	const v = (href ?? '').trim();
	if (!v) return '';
	if (v.startsWith('/') && !v.startsWith('//') && !v.includes('\\') && !v.includes('://')) {
		return v;
	}
	try {
		const u = new URL(v, window.location.origin);
		return u.origin === window.location.origin ? u.toString() : '';
	} catch {
		return '';
	}
}
