/**
 * Builds the API path for the content-manager view. Drafts and scheduled
 * posts route to their own endpoints; everything else hits /me/content with
 * optional type/cursor filters.
 *
 * Ref: japaconnect-engineering/sections/50-current-user.md
 */
export function buildContentQuery(opts: {
	status?: string;
	type?: string;
	cursor?: string;
	pageSize?: number;
}): string {
	const size = opts.pageSize ?? 20;
	if (opts.status === 'draft') return `/me/drafts?page_size=${size}`;
	if (opts.status === 'scheduled') return `/me/scheduled?page_size=${size}`;
	const params = new URLSearchParams({ page_size: String(size) });
	if (opts.type) params.set('type', opts.type);
	if (opts.cursor) params.set('cursor', opts.cursor);
	return `/me/content?${params.toString()}`;
}
