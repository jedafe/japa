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

/**
 * Resolve an API media URL (header_url / logo_url / TipTap image src) into an
 * absolute, crawler-usable og:image URL, or '' when the candidate is unusable
 * and the caller should fall through to the next one.
 *
 * API media paths are origin-relative (/api/v1/media/{id}) and nginx serves
 * them on the public origin, so origin + path is resolvable. SVG, data: and
 * blob: URLs are rejected — social crawlers silently ignore them.
 */
export function ogMediaUrl(origin: string, url: string | null | undefined): string {
	if (!url) return '';
	const u = url.trim();
	if (!u || /\.svg(\?.*)?$/i.test(u) || /^(data|blob):/i.test(u)) return '';
	if (/^https?:\/\//i.test(u) || u.startsWith('//')) return u;
	return u.startsWith('/') ? `${origin}${u}` : '';
}

/** OG card for any /tools/* page — the tool slug follows the literal
 * 'tools' path segment (locale-prefixed paths like /fr/tools/{slug} still
 * resolve correctly), so index and /tools/{slug}/[id] sub-pages share the
 * tool's card. /tools itself resolves to the brand card. */
export function toolOgCard(origin: string, pathname: string): string {
	const segs = pathname.split('/').filter(Boolean);
	const i = segs.indexOf('tools');
	const slug = i >= 0 ? segs[i + 1] : undefined;
	return ogCardUrl(origin, slug ? `tool:${slug}` : 'tools');
}
