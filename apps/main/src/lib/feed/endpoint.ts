/** Which feed a Home tab maps to. */
export type FeedTab = 'following' | 'for_you' | 'explore' | 'roadmap';

/** Backend `sort` values (note: chip label "Popular" → value "top"). */
export type SortValue = 'relevant' | 'latest' | 'top' | 'rising';

export const PAGE_SIZE = 20;

const TAB_PATH: Record<FeedTab, string> = {
	following: '/feed',
	for_you: '/feed/for-you',
	explore: '/explore',
	roadmap: '/feed/roadmap'
};

const SORTS: SortValue[] = ['relevant', 'latest', 'top', 'rising'];

/** Build the query-string portion of a feed request (without the leading `?`). */
export function feedQueryString(
	tab: FeedTab,
	sort: SortValue,
	pageSize: number,
	pageOffset: number,
	geo?: string
): string {
	// The roadmap feed is returned in assessment roadmap order; it does not
	// accept a sort parameter or geo override (ADR-064, §71).
	const params: string[] = [];
	if (tab !== 'roadmap') {
		params.push(`sort=${sort}`);
	}
	params.push(`page_size=${pageSize}`, `page_offset=${pageOffset}`);
	if (geo && tab !== 'roadmap') params.push(`geo=${encodeURIComponent(geo)}`);
	return params.join('&');
}

/**
 * Build the bare backend path for a feed request. Returns a path WITHOUT the
 * /api/v1 prefix — the server client (apiList) and browser client (api.get)
 * each prepend their own base, so we must not double-prefix.
 *
 * When geo is provided (a non-empty country code from the user's
 * content_country_preference or the ?geo= query param), it is forwarded as a
 * re-weight parameter — never a hard filter (ADR-064). The backend applies it
 * as a boost, not a WHERE clause, so content from other countries still
 * appears but is de-prioritized.
 */
export function feedPath(
	tab: FeedTab,
	sort: SortValue,
	pageSize: number,
	pageOffset: number,
	geo?: string
): string {
	return `${TAB_PATH[tab]}?${feedQueryString(tab, sort, pageSize, pageOffset, geo)}`;
}

/** Resolve the active tab. Guests are always explore; authed users default to for_you. */
export function resolveTab(raw: string | null, authed: boolean): FeedTab {
	if (!authed) return 'explore';
	if (raw === 'following' || raw === 'for_you' || raw === 'roadmap') return raw;
	return 'for_you';
}

/** Resolve the active sort; always yields an explicit value (default relevant). */
export function resolveSort(raw: string | null): SortValue {
	return (SORTS as string[]).includes(raw ?? '') ? (raw as SortValue) : 'relevant';
}
