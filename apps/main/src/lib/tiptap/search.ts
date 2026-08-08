import type { SearchHit } from '$lib/types';

const MIN_QUERY = 2;

export function searchPath(kind: 'users' | 'content' | 'tags' | 'coupons', query: string): string | null {
	const q = query.trim();
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

export function mapUserHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		// /search returns { title: username, excerpt: display_name }.
		label: String(r.excerpt || r.display_name || r.title || (r.username ?? '')),
		handle: r.title ? String(r.title) : (r.username ? String(r.username) : undefined),
		avatar_url: (r.avatar_url as string | null | undefined) ?? null
	}));
}

export function mapContentHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		// /search returns { title, url }; legacy posts return { title, slug }.
		label: String(r.title ?? ''),
		slug: r.slug ? String(r.slug) : (r.url ? String(r.url).split('/').pop() : undefined)
	}));
}

export function mapTagHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.name ?? r.title ?? ''),
		slug: r.slug ? String(r.slug) : undefined
	}));
}

export function mapCouponHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.title ?? r.code ?? r.name ?? ''),
		slug: r.code ? String(r.code) : undefined
	}));
}
