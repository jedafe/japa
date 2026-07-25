import type { SearchHit } from '$lib/types';

const MIN_QUERY = 2;

export function searchPath(kind: 'users' | 'content' | 'tags', query: string): string | null {
	const q = query.trim();
	if (q.length < MIN_QUERY) return null;
	const enc = encodeURIComponent(q);
	if (kind === 'tags') return `/tags?q=${enc}`;
	return `/search?type=${kind}&q=${enc}`;
}

function asArray(raw: unknown): Record<string, unknown>[] {
	return Array.isArray(raw) ? (raw as Record<string, unknown>[]) : [];
}

export function mapUserHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.display_name ?? r.username ?? ''),
		handle: r.username ? String(r.username) : undefined,
		avatar_url: (r.avatar_url as string | null | undefined) ?? null
	}));
}

export function mapContentHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.title ?? ''),
		slug: r.slug ? String(r.slug) : undefined
	}));
}

export function mapTagHits(raw: unknown): SearchHit[] {
	return asArray(raw).map((r) => ({
		id: String(r.id ?? ''),
		label: String(r.name ?? ''),
		slug: r.slug ? String(r.slug) : undefined
	}));
}
