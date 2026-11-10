/**
 * Generated OG-card URLs — the Go API renders 1200×630 PNG cards at
 * /og/{key}.png (nginx routes the path straight to the API in production;
 * the /og/ app route proxies it on other deployments). SVG is not a valid
 * og:image format anywhere, so generated PNGs are the only way to give
 * pages without uploaded media a share card.
 *
 * Keys:
 *   <uuid>        contents / wiki_pages row — title card
 *   tool:{slug}   /tools/{slug} index card (registry title)
 *   space:{slug}  /wiki/{slug} destination card (space name)
 *
 * Unknown keys render the brand fallback card, so a bad key degrades
 * gracefully instead of 404ing.
 */
export function ogCardUrl(origin: string, key: string): string {
	return `${origin}/og/${key}.png`;
}

/** OG card for any /tools/* page — the tool slug is always the second
 * path segment, so /tools/{slug} index and /tools/{slug}/[id] sub-pages
 * share the tool's card. /tools itself resolves to the brand card. */
export function toolOgCard(origin: string, pathname: string): string {
	const slug = pathname.split('/').filter(Boolean)[1];
	return ogCardUrl(origin, slug ? `tool:${slug}` : 'tools');
}
