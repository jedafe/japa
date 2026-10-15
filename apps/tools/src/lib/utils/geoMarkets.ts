/**
 * Geo market lists (ADR-064). The backend archetype resolver classifies
 * visitors against the geo_personalization.origin_markets and
 * geo_personalization.destination_markets system settings; both are public
 * (served by GET /site/settings/public → root layout `data.siteSettings`)
 * so pickers can pin the platform's curated markets first while still
 * allowing every seeded country.
 *
 * All helpers are pure — pass the siteSettings record in so they work in
 * SSR, server loads, and components alike.
 */

export type SiteSettingsMap = Record<string, unknown>;

/** Parse a CSV of ISO 3166-1 alpha-2 codes ("NG, gh") into ["NG", "GH"]. */
export function parseCountryCsv(raw: unknown): string[] {
	if (typeof raw !== 'string') return [];
	return raw
		.split(',')
		.map((s) => s.trim().toUpperCase())
		.filter((c) => /^[A-Z]{2}$/.test(c));
}

/** Configured origin markets — visitors from these are origin_visitor. */
export function originMarkets(settings: SiteSettingsMap | undefined): string[] {
	return parseCountryCsv(settings?.['geo_personalization.origin_markets']);
}

/** Configured destination markets — visitors there (or settled) are diaspora. */
export function destinationMarkets(settings: SiteSettingsMap | undefined): string[] {
	return parseCountryCsv(settings?.['geo_personalization.destination_markets']);
}

/** Union of origin + destination markets — for content/geo-filter pickers. */
export function servedMarkets(settings: SiteSettingsMap | undefined): string[] {
	return [...new Set([...originMarkets(settings), ...destinationMarkets(settings)])];
}

/**
 * Stable reorder: items whose code is in `markets` come first (in market
 * order); everything else keeps its original order. Every country remains
 * selectable — the curated list is pinned, not enforced.
 */
export function pinMarketsFirst<T>(items: T[], markets: string[], getCode: (item: T) => string): T[] {
	if (markets.length === 0 || items.length === 0) return items;
	const rank = new Map(markets.map((c, i) => [c, i]));
	return [...items].sort((a, b) => {
		const ra = rank.get(getCode(a).toUpperCase());
		const rb = rank.get(getCode(b).toUpperCase());
		if (ra === undefined) return rb === undefined ? 0 : 1;
		if (rb === undefined) return -1;
		return ra - rb;
	});
}
