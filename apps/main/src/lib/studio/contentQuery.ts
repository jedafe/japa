/**
 * Builds the query-string portion (no leading `?`) for the content-manager
 * view. The caller supplies the base path literal so the endpoint audit can
 * see `/me/drafts`, `/me/scheduled`, and `/me/content`.
 *
 * Ref: japaconnect-engineering/sections/50-current-user.md
 */
export function contentQueryString(opts: {
	status?: string;
	type?: string;
	q?: string;
	sort?: string;
	pageOffset?: number;
	pageSize?: number;
}): string {
	const size = opts.pageSize ?? 20;
	const params = new URLSearchParams({ page_size: String(size) });
	if (opts.type) params.set('type', opts.type);
	if (opts.q) params.set('q', opts.q);
	if (opts.sort) params.set('sort', opts.sort);
	if (opts.pageOffset) params.set('page_offset', String(opts.pageOffset));
	return params.toString();
}

/**
 * Resolve the base endpoint for a content-manager tab.
 */
export function contentBase(
	status?: string
): '/me/drafts' | '/me/scheduled' | '/me/in-review' | '/me/content' {
	if (status === 'draft') return '/me/drafts';
	if (status === 'scheduled') return '/me/scheduled';
	if (status === 'in_review') return '/me/in-review';
	return '/me/content';
}

/**
 * Builds the API path for the content-manager view. Kept for tests and callers
 * that do not need per-path literal extraction.
 */
export function buildContentQuery(opts: {
	status?: string;
	type?: string;
	q?: string;
	sort?: string;
	pageOffset?: number;
	pageSize?: number;
}): string {
	return `${contentBase(opts.status)}?${contentQueryString(opts)}`;
}
