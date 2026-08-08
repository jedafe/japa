/**
 * Builds the API path for the content-manager view. Drafts and scheduled
 * posts route to their own endpoints; everything else hits /me/content with
 * optional type/q/pageOffset filters.
 *
 * Ref: japaconnect-engineering/sections/50-current-user.md
 */
export function buildContentQuery(opts: {
	status?: string;
	type?: string;
	q?: string;
	pageOffset?: number;
	pageSize?: number;
}): string {
	const size = opts.pageSize ?? 20;
	const params = new URLSearchParams({ page_size: String(size) });
	if (opts.status === 'draft') {
		if (opts.q) params.set('q', opts.q);
		if (opts.pageOffset) params.set('page_offset', String(opts.pageOffset));
		return `/me/drafts?${params.toString()}`;
	}
	if (opts.status === 'scheduled') {
		if (opts.q) params.set('q', opts.q);
		if (opts.pageOffset) params.set('page_offset', String(opts.pageOffset));
		return `/me/scheduled?${params.toString()}`;
	}
	if (opts.type) params.set('type', opts.type);
	if (opts.q) params.set('q', opts.q);
	if (opts.pageOffset) params.set('page_offset', String(opts.pageOffset));
	return `/me/content?${params.toString()}`;
}
