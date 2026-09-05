import type { SearchHit } from '$lib/types';

const MIN_QUERY = 2;

export function searchPath(
	kind: 'users' | 'content' | 'tags' | 'coupons' | 'literals',
	query: string
): string | null {
	const q = query.trim();
	// The {{ token picker lists every active literal when the query is empty,
	// so it bypasses the 2-char minimum used by the search-backed kinds.
	if (kind === 'literals') return `/literals?q=${encodeURIComponent(q)}`;
	if (q.length < MIN_QUERY) return null;
	const enc = encodeURIComponent(q);
	if (kind === 'tags') return `/tags?q=${enc}`;
	if (kind === 'coupons') return `/coupons?q=${enc}`;
	// Backend type filter uses singular form: "user" not "users".
	const type = kind === 'users' ? 'user' : 'content';
	return `/search?type=${type}&q=${enc}`;
}

function asArray(raw: unknown): Record<string, unknown>[] {
	return Array.isArray(raw) ? (raw as Record<string, unknown>[]) : [];
}

/**
 * §54: /search returns hits as { type, item } where item holds the
 * type-specific fields (username/display_name/avatar_url for users,
 * title/url/slug for content). Older code paths passed flat hit objects —
 * fall back to the row itself when `item` is absent.
 */
function unwrapItem(r: Record<string, unknown>): Record<string, unknown> {
	const item = r.item;
	return item && typeof item === 'object' ? (item as Record<string, unknown>) : r;
}

export function mapUserHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => {
		const it = unwrapItem(r);
		const handle = it.username ? String(it.username) : (it.title ? String(it.title) : undefined);
		return {
			id: String(it.id ?? ''),
			// §54 user items expose display_name + username (title/excerpt are
			// cleared server-side); fall back through legacy field names.
			label: String(it.display_name || it.username || it.excerpt || it.title || ''),
			handle,
			url: handle ? `/u/${handle}` : undefined,
			avatar_url: (it.avatar_url as string | null | undefined) ?? null
		};
	});
}

export function mapContentHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => {
		const it = unwrapItem(r);
		const url = it.url ? String(it.url) : undefined;
		return {
			id: String(it.id ?? ''),
			label: String(it.title ?? ''),
			// §54 content items carry the full public path in `url`; keep it so
			// the interlink node can link to it at render time.
			url,
			slug: it.slug ? String(it.slug) : (url ? url.split('/').pop() : undefined)
		};
	});
}

export function mapTagHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.name ?? r.title ?? ''),
		slug: r.slug ? String(r.slug) : undefined,
		url: r.slug ? `/tags/${String(r.slug)}` : undefined
	}));
}

export function mapLiteralHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		// id carries the token key — the `{{` suggestion command inserts
		// `{{id}}` as plain text for server-side resolution at render time.
		id: String(r.key ?? r.id ?? ''),
		label: String(r.label ?? r.key ?? ''),
		slug: r.key ? String(r.key) : undefined
	}));
}

export function mapCouponHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.title ?? r.code ?? r.name ?? ''),
		slug: r.code ? String(r.code) : undefined
	}));
}
