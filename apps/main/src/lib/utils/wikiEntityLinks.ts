// Entity-bound wiki page links (JCE ↔ wiki mapping): /wiki/e/{type}/{entity}/{page}.
export function entityPageHref(typeSlug: string, entitySlug: string, pageSlug = 'overview'): string {
	return `/wiki/e/${encodeURIComponent(typeSlug)}/${encodeURIComponent(entitySlug)}/${encodeURIComponent(pageSlug)}`;
}
