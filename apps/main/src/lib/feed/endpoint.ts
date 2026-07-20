/** Which feed a Home tab maps to. */
export type FeedTab = 'following' | 'for_you' | 'explore';

/** Backend `sort` values (note: chip label "Popular" → value "top"). */
export type SortValue = 'relevant' | 'latest' | 'top' | 'rising';

export const PAGE_SIZE = 20;

const TAB_PATH: Record<FeedTab, string> = {
	following: '/feed',
	for_you: '/feed/for-you',
	explore: '/explore'
};

const SORTS: SortValue[] = ['relevant', 'latest', 'top', 'rising'];

/**
 * Build the bare backend path for a feed request. Returns a path WITHOUT the
 * /api/v1 prefix — the server client (apiList) and browser client (api.get)
 * each prepend their own base, so we must not double-prefix.
 */
export function feedPath(tab: FeedTab, sort: SortValue, pageSize: number, pageOffset: number): string {
	return `${TAB_PATH[tab]}?sort=${sort}&page_size=${pageSize}&page_offset=${pageOffset}`;
}

/** Resolve the active tab. Guests are always explore; authed users default to for_you. */
export function resolveTab(raw: string | null, authed: boolean): FeedTab {
	if (!authed) return 'explore';
	if (raw === 'following' || raw === 'for_you') return raw;
	return 'for_you';
}

/** Resolve the active sort; always yields an explicit value (default relevant). */
export function resolveSort(raw: string | null): SortValue {
	return (SORTS as string[]).includes(raw ?? '') ? (raw as SortValue) : 'relevant';
}
