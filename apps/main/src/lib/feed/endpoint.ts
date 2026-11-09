/** Which feed a Home tab maps to. 'thread' lists all discussions via /explore. */
export type FeedTab = 'following' | 'for_you' | 'explore' | 'thread';

/** Backend `sort` values (note: chip label "Popular" → value "top"). */
export type SortValue = 'relevant' | 'latest' | 'top' | 'rising';

export const PAGE_SIZE = 20;

const TAB_PATH: Record<FeedTab, string> = {
	following: '/feed',
	for_you: '/feed/for-you',
	explore: '/explore',
	// Every contents row is a thread (unified-discussion spec) — the Thread
	// tab lists the same /explore feed the /thread index page does.
	thread: '/explore'
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
	const params: string[] = [`sort=${sort}`, `page_size=${pageSize}`, `page_offset=${pageOffset}`];
	if (geo) params.push(`geo=${encodeURIComponent(geo)}`);
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
	if (raw === 'following' || raw === 'for_you' || raw === 'thread') return raw;
	return 'for_you';
}

/** Resolve the active sort; always yields an explicit value. Most tabs
 * default to 'relevant' — the for-you feed ranks it, the following feed
 * has a real relevant arm, and explore serves its geo discovery blend for
 * it; the Thread tab defaults to 'latest' (chronological, like /thread).
 */
export function resolveSort(raw: string | null, tab?: FeedTab): SortValue {
	// The Thread tab mirrors the /thread index — Latest/Top only, so a
	// carried-over ?sort=relevant|rising coerces to the default instead of
	// leaving no chip highlighted.
	if (tab === 'thread') return raw === 'top' ? 'top' : 'latest';
	return (SORTS as string[]).includes(raw ?? '') ? (raw as SortValue) : 'relevant';
}
